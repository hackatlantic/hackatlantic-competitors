//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"testing"
	"time"
)

func TestScannerEmailLookupAndAuditedAccessLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	profiles := profileSource{profiles: map[string]users.Profile{
		"lookup-admin":     {ClerkUserID: "lookup-admin", Email: "lookup-admin@example.test"},
		"lookup-volunteer": {ClerkUserID: "lookup-volunteer", Email: "Volunteer@example.test"},
	}}
	service := users.NewService(pool, profiles, 5*time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO ats.admin_email_allowlist(normalized_email) VALUES ('lookup-admin@example.test')`); err != nil {
		t.Fatal(err)
	}
	admin, err := service.Resolve(ctx, "lookup-admin")
	if err != nil {
		t.Fatal(err)
	}
	volunteer, err := service.Resolve(ctx, "lookup-volunteer")
	if err != nil {
		t.Fatal(err)
	}
	lookup := func() users.ScannerAccessUser {
		t.Helper()
		found, err := service.LookupScannerUser(ctx, admin, "  VOLUNTEER@example.test  ")
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	found := lookup()
	if found.ID != volunteer.ID || found.ScannerAccess || !found.CanManage {
		t.Fatalf("unexpected initial access: %+v", found)
	}
	for range 2 {
		if err := service.GrantScannerRole(ctx, admin, found.ID); err != nil {
			t.Fatal(err)
		}
	}
	if !lookup().ScannerAccess {
		t.Fatal("grant not reflected by lookup")
	}
	for range 2 {
		if err := service.RevokeScannerRole(ctx, admin, found.ID); err != nil {
			t.Fatal(err)
		}
	}
	if lookup().ScannerAccess {
		t.Fatal("revoke not reflected by lookup")
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE subject_id=$1 AND event_type IN ('scanner_role_assigned','scanner_role_revoked')`, volunteer.ID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("expected exactly two audited changes, got %d: %v", audits, err)
	}
	if _, err := service.LookupScannerUser(ctx, volunteer, "lookup-admin@example.test"); !errors.Is(err, users.ErrForbidden) {
		t.Fatal("non-admin lookup allowed")
	}
	self, err := service.LookupScannerUser(ctx, admin, admin.Email)
	if err != nil || !self.ScannerAccess || self.CanManage {
		t.Fatal("admin access must be inherited and read-only")
	}
	if _, err := service.LookupScannerUser(ctx, admin, "missing@example.test"); !errors.Is(err, users.ErrNotFound) {
		t.Fatal("missing account not rejected")
	}
	profiles.profiles["lookup-volunteer"] = users.Profile{ClerkUserID: "lookup-volunteer", Email: "changed@example.test"}
	if _, err := service.LookupScannerUser(ctx, admin, volunteer.Email); !errors.Is(err, users.ErrNotFound) {
		t.Fatal("stale verified email not rejected")
	}
	delete(profiles.profiles, "lookup-volunteer")
	if _, err := service.LookupScannerUser(ctx, admin, volunteer.Email); !errors.Is(err, users.ErrProfileUnavailable) {
		t.Fatal("identity failure not rejected")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.users(clerk_user_id,primary_email) VALUES ('duplicate-lookup','volunteer@example.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.LookupScannerUser(ctx, admin, volunteer.Email); !errors.Is(err, users.ErrAmbiguousEmail) {
		t.Fatal("ambiguous email not rejected")
	}
}
