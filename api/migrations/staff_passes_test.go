//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/redemptions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAutomaticStaffPasses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	adminID := createUser(t, ctx, pool, "staff-admin")
	volunteerID := createUser(t, ctx, pool, "staff-volunteer")
	otherID := createUser(t, ctx, pool, "staff-other")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO ats.admin_email_allowlist(normalized_email) SELECT lower(primary_email) FROM ats.users WHERE id=$1`, adminID)
	exec(`UPDATE ats.users SET display_name='Test Organizer' WHERE id=$1`, adminID)
	config := pool.Config().Copy()
	config.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE hackatlantic_app")
		return err
	}
	runtime, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	service, err := passes.NewService(runtime, 5*time.Second, 15*time.Second, passes.Config{
		QRTokenPepper: encodedTestPepper(1), ClaimTokenPepper: encodedTestPepper(2), StaffPassExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately stale/untrusted role claims must not grant a credential.
	other := users.User{ID: otherID, Roles: map[users.Role]struct{}{users.RoleAdmin: {}}}
	if _, err = service.EnsureStaffPass(ctx, other); !errors.Is(err, passes.ErrNotFound) {
		t.Fatalf("unapproved user: %v", err)
	}
	volunteer := users.User{ID: volunteerID}
	exec(`INSERT INTO ats.volunteer_requests(user_id,real_name) VALUES($1,'Test Volunteer')`, volunteerID)
	exec(`INSERT INTO ats.user_roles(user_id,role) VALUES($1,'scanner')`, volunteerID)
	if _, err = service.EnsureStaffPass(ctx, volunteer); !errors.Is(err, passes.ErrNotFound) {
		t.Fatalf("pending volunteer: %v", err)
	}
	exec(`UPDATE ats.volunteer_requests SET status='approved',reviewed_by=$2,reviewed_at=now() WHERE user_id=$1`, volunteerID, adminID)
	// Concurrent loads issue exactly one pass and one audit event.
	results := make(chan passes.StaffPass, 10)
	errs := make(chan error, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); p, e := service.EnsureStaffPass(ctx, volunteer); results <- p; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var credential passes.StaffPass
	for p := range results {
		if credential.ID != "" && (p.ID != credential.ID || p.QRToken != credential.QRToken) {
			t.Fatal("duplicate staff credentials")
		}
		credential = p
	}
	if credential.Kind != "volunteer" || credential.DisplayName != "Test Volunteer" {
		t.Fatalf("wrong staff projection: %s %s", credential.Kind, credential.DisplayName)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type='staff_pass_issued'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("issuance audit count %d: %v", count, err)
	}
	adminPass, err := service.EnsureStaffPass(ctx, users.User{ID: adminID})
	if err != nil || adminPass.Kind != "organizer" {
		t.Fatalf("admin pass: %v", err)
	}
	scanner := users.User{ID: adminID, Roles: map[users.Role]struct{}{users.RoleAdmin: {}}}
	scan := redemptions.NewService(runtime, 5*time.Second, service)
	for range 3 {
		result, e := scan.Lookup(ctx, scanner, credential.QRToken)
		if e != nil || result.Pass.Kind != "volunteer" {
			t.Fatalf("repeat verification: %v", e)
		}
	}
	if _, err = scan.Lookup(ctx, volunteer, credential.QRToken); !errors.Is(err, redemptions.ErrForbidden) {
		t.Fatalf("unauthorized lookup: %v", err)
	}
	if _, err = service.ResolveClaim(ctx, credential.QRToken); !errors.Is(err, passes.ErrNotFound) && !errors.Is(err, passes.ErrInvalidCred) {
		t.Fatalf("staff QR accepted as claim: %v", err)
	}
	// Neither role removal nor approval revocation may leave a saved QR valid.
	exec(`DELETE FROM ats.user_roles WHERE user_id=$1 AND role='scanner'`, volunteerID)
	if _, err = scan.Lookup(ctx, scanner, credential.QRToken); !errors.Is(err, redemptions.ErrNotFound) {
		t.Fatalf("removed scanner remains valid: %v", err)
	}
	exec(`INSERT INTO ats.user_roles(user_id,role) VALUES($1,'scanner')`, volunteerID)
	exec(`UPDATE ats.volunteer_requests SET status='revoked' WHERE user_id=$1`, volunteerID)
	if _, err = service.EnsureStaffPass(ctx, volunteer); !errors.Is(err, passes.ErrNotFound) {
		t.Fatalf("revoked owner access: %v", err)
	}
	if _, err = scan.Lookup(ctx, scanner, credential.QRToken); !errors.Is(err, redemptions.ErrNotFound) {
		t.Fatalf("revoked approval remains valid: %v", err)
	}
	exec(`DELETE FROM ats.admin_email_allowlist WHERE normalized_email IN (SELECT lower(primary_email) FROM ats.users WHERE id=$1)`, adminID)
	if _, err = scan.Lookup(ctx, scanner, adminPass.QRToken); !errors.Is(err, redemptions.ErrNotFound) {
		t.Fatalf("removed admin remains valid: %v", err)
	}
	exec(`INSERT INTO ats.admin_email_allowlist(normalized_email) SELECT lower(primary_email) FROM ats.users WHERE id=$1`, adminID)
	exec(`UPDATE ats.staff_passes SET expires_at=now()-interval '1 minute' WHERE user_id=$1`, adminID)
	if _, err = service.EnsureStaffPass(ctx, users.User{ID: adminID}); !errors.Is(err, passes.ErrNotFound) {
		t.Fatalf("expired pass renewed: %v", err)
	}
	if _, err = scan.Lookup(ctx, scanner, adminPass.QRToken); !errors.Is(err, redemptions.ErrNotFound) {
		t.Fatalf("expired scan: %v", err)
	}
	for _, table := range []string{"applications", "attendance_responses", "attendees", "passes", "redemptions", "email_outbox"} {
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM ats.`+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("staff flow changed %s: %d %v", table, count, err)
		}
	}
	for _, role := range []string{"anon", "authenticated", "service_role"} {
		var allowed bool
		if err = pool.QueryRow(ctx, `SELECT has_table_privilege($1,'ats.staff_passes','SELECT')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatalf("staff table exposed to %s: %v", role, err)
		}
	}
}
