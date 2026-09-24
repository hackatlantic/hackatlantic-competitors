//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

func TestVolunteerApprovalLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	profiles := profileSource{profiles: map[string]users.Profile{
		"vol-admin": {ClerkUserID: "vol-admin", Email: "vol-admin@example.test"},
		"vol-one":   {ClerkUserID: "vol-one", Email: "vol-one@example.test"},
		"vol-two":   {ClerkUserID: "vol-two", Email: "vol-two@example.test"},
	}}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.admin_email_allowlist(normalized_email) VALUES ('vol-admin@example.test')`); err != nil {
		t.Fatal(err)
	}
	s := users.NewService(pool, profiles, 5*time.Second)
	admin, err := s.Resolve(ctx, "vol-admin")
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.Resolve(ctx, "vol-one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Resolve(ctx, "vol-two")
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the new service with the actual restricted API database role.
	runtimeConfig := pool.Config().Copy()
	runtimeConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE hackatlantic_app")
		return err
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()
	s = users.NewService(runtimePool, profiles, 5*time.Second)
	// No application row is created or required, even with the intake closed.
	if _, err = pool.Exec(ctx, `UPDATE ats.application_cycles SET applications_close_at=now()-interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	r, err := s.RequestVolunteerAccess(ctx, one, "Alex Morgan")
	if err != nil || r.Status != "pending" || r.ScannerAccess {
		t.Fatalf("request: %+v %v", r, err)
	}
	r, err = s.RequestVolunteerAccess(ctx, one, "Changed Name")
	if err != nil || r.RealName != "Alex Morgan" {
		t.Fatal("retry changed identity")
	}
	if r, err = s.GetVolunteerRequest(ctx, two); err != nil || r != nil {
		t.Fatal("other volunteer can see request")
	}
	if err = s.ReviewVolunteerRequest(ctx, one, one.ID, "approved", "pending"); !errors.Is(err, users.ErrForbidden) {
		t.Fatal("self promotion allowed")
	}
	// Duplicate real names are not identity verification or an automatic grant.
	if _, err = s.RequestVolunteerAccess(ctx, two, "Alex Morgan"); err != nil {
		t.Fatal(err)
	}
	queue, err := s.ListVolunteerRequests(ctx, admin, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue: %v %v", queue, err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.ReviewVolunteerRequest(ctx, admin, one.ID, "approved", "pending")
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, users.ErrVolunteerConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("concurrent approval not serialized")
	}
	one, err = s.Resolve(ctx, "vol-one")
	if err != nil || !one.HasRole(users.RoleScanner) || one.HasRole(users.RoleAdmin) || one.HasRole(users.RoleOrganizer) {
		t.Fatal("approval granted wrong roles")
	}
	if err = s.ReviewVolunteerRequest(ctx, admin, one.ID, "revoked", "approved"); err != nil {
		t.Fatal(err)
	}
	if err = s.ReviewVolunteerRequest(ctx, admin, one.ID, "approved", "pending"); !errors.Is(err, users.ErrVolunteerConflict) {
		t.Fatal("replayed approval restored revoked access")
	}
	one, err = s.Resolve(ctx, "vol-one")
	if err != nil || one.HasRole(users.RoleScanner) {
		t.Fatal("revocation failed")
	}
	r, err = s.RequestVolunteerAccess(ctx, one, "Alex Morgan")
	if err != nil || r.Status != "revoked" {
		t.Fatal("resubmission reset review")
	}
	if err = s.ReviewVolunteerRequest(ctx, admin, two.ID, "rejected", "pending"); err != nil {
		t.Fatal(err)
	}
	two, err = s.Resolve(ctx, "vol-two")
	if err != nil || two.HasRole(users.RoleScanner) {
		t.Fatal("declined account gained access")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE subject_id=$1 AND event_type IN ('scanner_role_assigned','scanner_role_revoked')`, one.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("role audits %d %v", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM ats.applications WHERE applicant_user_id IN ($1,$2)`, one.ID, two.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("volunteers became applicants")
	}
	for _, role := range []string{"anon", "authenticated", "service_role"} {
		var access bool
		if err = pool.QueryRow(ctx, `SELECT has_table_privilege($1,'ats.volunteer_requests','SELECT,INSERT,UPDATE,DELETE')`, role).Scan(&access); err != nil || access {
			t.Fatalf("exposed request table to %s", role)
		}
	}
}
