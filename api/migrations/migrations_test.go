//go:build integration

package migrations_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func encodedTestPepper(value byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

const integrationTimeout = 30 * time.Second

type profileSource struct {
	profiles map[string]users.Profile
}

func (source profileSource) Profile(_ context.Context, clerkUserID string) (users.Profile, error) {
	profile, ok := source.profiles[clerkUserID]
	if !ok {
		return users.Profile{}, fmt.Errorf("unknown Clerk user %q", clerkUserID)
	}
	return profile, nil
}

func TestMilestoneOneFoundation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()

	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply clean migrations: %v", err)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	assertSchemaBoundary(t, ctx, pool)
	assertRuntimeDatabaseRole(t, ctx, pool)
	creatorID := createUser(t, ctx, pool, "clerk-creator")
	cycleID, _ := assertCycleAndFormInvariants(t, ctx, pool, creatorID)
	assertAnswerAndPublishedFormImmutability(t, ctx, pool, creatorID, cycleID)
	assertRoleConstraints(t, ctx, pool, creatorID)
	assertUserReconciliation(t, ctx, pool)
	assertOrganizerBootstrap(t, ctx, pool)
	if _, err := pool.Exec(ctx, `UPDATE ats.schema_migrations SET checksum = 'tampered' WHERE version = '000002_secure_ats_schema.sql'`); err != nil {
		t.Fatalf("tamper migration checksum fixture: %v", err)
	}
	if err := migrations.Apply(ctx, pool); err == nil {
		t.Fatal("expected tampered migration checksum to be rejected")
	}
}

func disposableDatabase(t *testing.T, ctx context.Context) (*pgxpool.Pool, func()) {
	t.Helper()
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required; run the disposable PostgreSQL integration service")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	databaseName := fmt.Sprintf("hackatlantic_m1_%d", time.Now().UnixNano())
	parsed.Path = "/postgres"
	adminURL := parsed.String()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect integration PostgreSQL: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+databaseName); err != nil {
		admin.Close(ctx)
		t.Fatalf("create disposable database: %v", err)
	}
	createdRoles := make([]string, 0, 3)
	for _, role := range []string{"anon", "authenticated", "service_role"} {
		var exists bool
		if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
			admin.Close(ctx)
			t.Fatalf("inspect Supabase role %s: %v", role, err)
		}
		if !exists {
			if _, err := admin.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN"); err != nil {
				admin.Close(ctx)
				t.Fatalf("create Supabase role fixture %s: %v", role, err)
			}
			createdRoles = append(createdRoles, role)
		}
	}
	var runtimeRoleExists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'hackatlantic_app')`).Scan(&runtimeRoleExists); err != nil {
		admin.Close(ctx)
		t.Fatalf("inspect runtime database role fixture: %v", err)
	}
	if !runtimeRoleExists {
		// The migration creates this cluster-scoped role. Record ownership here
		// so cleanup never removes a role that predates the disposable database.
		createdRoles = append(createdRoles, "hackatlantic_app")
	}
	parsed.Path = "/" + databaseName
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		admin.Close(ctx)
		t.Fatalf("open disposable database: %v", err)
	}
	cleanup := func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, databaseName)
		_, _ = admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+databaseName)
		for _, role := range createdRoles {
			_, _ = admin.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+role)
		}
		admin.Close(cleanupCtx)
	}
	return pool, cleanup
}

func assertSchemaBoundary(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{"users", "user_roles", "admin_email_allowlist", "application_cycles", "application_forms", "applications", "application_answers", "application_resumes", "review_assignments", "reviews", "decisions", "attendees", "attendee_roles", "passes", "activities", "checkpoints", "attendee_entitlements", "redemption_requests", "redemptions", "audit_events", "email_outbox"}
	var atsCount, publicCount, ledgerCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'ats' AND table_name = ANY($1)`, tables).Scan(&atsCount); err != nil {
		t.Fatalf("count ATS tables: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ANY($1)`, tables).Scan(&publicCount); err != nil {
		t.Fatalf("count public ATS tables: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.schema_migrations WHERE checksum IS NOT NULL`).Scan(&ledgerCount); err != nil {
		t.Fatalf("inspect migration ledger: %v", err)
	}
	if atsCount != len(tables) || publicCount != 0 || ledgerCount != 12 {
		t.Fatalf("unexpected schema boundary: ats=%d public=%d checksummed=%d", atsCount, publicCount, ledgerCount)
	}

	assertFalseQuery(t, ctx, pool, `SELECT EXISTS (
        SELECT 1
        FROM pg_namespace namespace
        CROSS JOIN LATERAL aclexplode(COALESCE(namespace.nspacl, acldefault('n', namespace.nspowner))) privilege
        WHERE namespace.nspname = 'ats' AND privilege.grantee = 0
    )`, "PUBLIC schema privilege")
	assertFalseQuery(t, ctx, pool, `SELECT EXISTS (
        SELECT 1
        FROM pg_class relation
        CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl, acldefault('r', relation.relowner))) privilege
        WHERE relation.oid = 'ats.users'::regclass AND privilege.grantee = 0
    )`, "PUBLIC table privilege")
	assertFalseQuery(t, ctx, pool, `SELECT EXISTS (
        SELECT 1
        FROM pg_class relation
        CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl, acldefault('r', relation.relowner))) privilege
        WHERE relation.oid = 'ats.activities'::regclass AND privilege.grantee = 0
    )`, "PUBLIC activity table privilege")
	assertFalseQuery(t, ctx, pool, `SELECT EXISTS (
        SELECT 1
        FROM pg_proc procedure
        CROSS JOIN LATERAL aclexplode(COALESCE(procedure.proacl, acldefault('f', procedure.proowner))) privilege
        WHERE procedure.oid = 'ats.prevent_non_draft_application_answers()'::regprocedure
          AND privilege.grantee = 0
          AND privilege.privilege_type = 'EXECUTE'
    )`, "PUBLIC function execution")

	rows, err := pool.Query(ctx, `SELECT rolname FROM pg_catalog.pg_roles WHERE rolname = ANY($1)`, []string{"anon", "authenticated", "service_role"})
	if err != nil {
		t.Fatalf("list Supabase roles: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatalf("scan Supabase role: %v", err)
		}
		var schemaUsage, userTableSelect, passTableSelect, activityTableSelect, functionExecute bool
		if err := pool.QueryRow(ctx, `SELECT has_schema_privilege($1, 'ats', 'USAGE'), has_table_privilege($1, 'ats.users', 'SELECT'), has_table_privilege($1, 'ats.passes', 'SELECT'), has_table_privilege($1, 'ats.activities', 'SELECT'), has_function_privilege($1, 'ats.prevent_non_draft_application_answers()', 'EXECUTE')`, role).Scan(&schemaUsage, &userTableSelect, &passTableSelect, &activityTableSelect, &functionExecute); err != nil {
			t.Fatalf("check %s privileges: %v", role, err)
		}
		if schemaUsage || userTableSelect || passTableSelect || activityTableSelect || functionExecute {
			t.Fatalf("Supabase Data API role %s retains ATS privileges", role)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Supabase roles: %v", err)
	}
}

func assertRuntimeDatabaseRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var exists, canLogin bool
	if err := pool.QueryRow(ctx, `
        SELECT true, rolcanlogin
        FROM pg_catalog.pg_roles
        WHERE rolname = 'hackatlantic_app'
    `).Scan(&exists, &canLogin); err != nil {
		t.Fatalf("inspect runtime database role: %v", err)
	}
	if !exists || canLogin {
		t.Fatalf("unexpected runtime database role: exists=%t can_login=%t", exists, canLogin)
	}

	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire runtime role test connection: %v", err)
	}
	defer connection.Release()
	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatalf("begin runtime role test transaction: %v", err)
	}
	defer transaction.Rollback(ctx)
	if _, err := transaction.Exec(ctx, `SET LOCAL ROLE hackatlantic_app`); err != nil {
		t.Fatalf("assume runtime database role: %v", err)
	}
	var currentUser string
	if err := transaction.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		t.Fatalf("read assumed runtime database role: %v", err)
	}
	if currentUser != "hackatlantic_app" {
		t.Fatalf("unexpected assumed runtime database role %q", currentUser)
	}
	var userCount int
	if err := transaction.QueryRow(ctx, `SELECT count(*) FROM ats.users`).Scan(&userCount); err != nil {
		t.Fatalf("runtime role cannot read ATS tables: %v", err)
	}
}

func assertCycleAndFormInvariants(t *testing.T, ctx context.Context, pool *pgxpool.Pool, creatorID string) (string, string) {
	t.Helper()
	cycleID := insertCycle(t, ctx, pool, "cycle-primary", true)
	mustFail(t, "second active cycle", func() error {
		_, err := pool.Exec(ctx, `INSERT INTO ats.application_cycles (slug, name, applications_open_at, applications_close_at, active) VALUES ('cycle-second-active', 'Second', now(), now() + interval '1 day', true)`)
		return err
	})
	otherCycleID := insertCycle(t, ctx, pool, "cycle-secondary", false)
	formID := insertForm(t, ctx, pool, cycleID, 1, creatorID)
	mustFail(t, "application form cycle mismatch", func() error {
		_, err := pool.Exec(ctx, `INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id) VALUES ($1, $2, $3)`, otherCycleID, formID, creatorID)
		return err
	})
	return cycleID, otherCycleID
}

func assertAnswerAndPublishedFormImmutability(t *testing.T, ctx context.Context, pool *pgxpool.Pool, creatorID, cycleID string) {
	t.Helper()
	formID := insertForm(t, ctx, pool, cycleID, 2, creatorID)
	var applicationID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id) VALUES ($1, $2, $3) RETURNING id::text`, cycleID, formID, creatorID).Scan(&applicationID); err != nil {
		t.Fatalf("create draft application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.application_answers (application_id, question_key, value_json) VALUES ($1, 'name', '"Ada"'::jsonb)`, applicationID); err != nil {
		t.Fatalf("create draft answer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ats.applications SET status = 'submitted', submitted_at = now() WHERE id = $1`, applicationID); err != nil {
		t.Fatalf("submit application fixture: %v", err)
	}
	mustFail(t, "answer update after submission", func() error {
		_, err := pool.Exec(ctx, `UPDATE ats.application_answers SET value_json = '"Grace"'::jsonb WHERE application_id = $1 AND question_key = 'name'`, applicationID)
		return err
	})

	publishedFormID := insertForm(t, ctx, pool, cycleID, 3, creatorID)
	if _, err := pool.Exec(ctx, `UPDATE ats.application_forms SET published_at = now() WHERE id = $1`, publishedFormID); err != nil {
		t.Fatalf("publish form fixture: %v", err)
	}
	mustFail(t, "unpublish form", func() error {
		_, err := pool.Exec(ctx, `UPDATE ats.application_forms SET published_at = NULL WHERE id = $1`, publishedFormID)
		return err
	})
	mustFail(t, "edit published form", func() error {
		_, err := pool.Exec(ctx, `UPDATE ats.application_forms SET schema_json = '{"changed":true}'::jsonb WHERE id = $1`, publishedFormID)
		return err
	})
}

func assertRoleConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) {
	t.Helper()
	mustFail(t, "invalid role", func() error {
		_, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, 'root')`, userID)
		return err
	})
}

func assertUserReconciliation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	displayName := "Applicant"
	if _, err := pool.Exec(ctx, `INSERT INTO ats.admin_email_allowlist (normalized_email) VALUES ('staff@example.test')`); err != nil {
		t.Fatalf("seed database admin allowlist: %v", err)
	}
	service := users.NewService(pool, profileSource{profiles: map[string]users.Profile{
		"clerk-new":   {ClerkUserID: "clerk-new", Email: "new@example.test", DisplayName: &displayName},
		"clerk-staff": {ClerkUserID: "clerk-staff", Email: "staff@example.test", DisplayName: &displayName},
	}}, integrationTimeout)
	newUser, err := service.Resolve(ctx, "clerk-new")
	if err != nil {
		t.Fatalf("reconcile new user: %v", err)
	}
	if len(newUser.Roles) != 1 || !newUser.HasRole(users.RoleApplicant) {
		t.Fatalf("new user received unexpected roles: %+v", newUser.Roles)
	}
	staffUser, err := service.Resolve(ctx, "clerk-staff")
	if err != nil {
		t.Fatalf("reconcile staff fixture: %v", err)
	}
	if !staffUser.HasRole(users.RoleAdmin) || !staffUser.HasRole(users.RoleOrganizer) || !staffUser.HasRole(users.RoleReviewer) || !staffUser.HasRole(users.RoleScanner) {
		t.Fatalf("allowlisted admin did not receive all staff capabilities: %+v", staffUser.Roles)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, 'reviewer')`, staffUser.ID); err != nil {
		t.Fatalf("grant staff fixture role: %v", err)
	}
	staffUser, err = service.Resolve(ctx, "clerk-staff")
	if err != nil {
		t.Fatalf("reconcile existing staff user: %v", err)
	}
	if !staffUser.HasRole(users.RoleApplicant) || !staffUser.HasRole(users.RoleReviewer) {
		t.Fatalf("reconciliation lost privileged role: %+v", staffUser.Roles)
	}
}

func assertOrganizerBootstrap(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	createUser(t, ctx, pool, "clerk-bootstrap-first")
	createUser(t, ctx, pool, "clerk-bootstrap-other")
	created, err := users.BootstrapFirstOrganizer(ctx, pool, integrationTimeout, "clerk-bootstrap-first")
	if err != nil || !created {
		t.Fatalf("bootstrap initial organizer: created=%t err=%v", created, err)
	}
	created, err = users.BootstrapFirstOrganizer(ctx, pool, integrationTimeout, "clerk-bootstrap-first")
	if err != nil || created {
		t.Fatalf("idempotent bootstrap: created=%t err=%v", created, err)
	}
	if _, err := users.BootstrapFirstOrganizer(ctx, pool, integrationTimeout, "clerk-bootstrap-other"); !errors.Is(err, users.ErrOrganizerAlreadyExists) {
		t.Fatalf("expected existing organizer rejection, got %v", err)
	}
	var organizerCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.user_roles WHERE role = 'organizer'`).Scan(&organizerCount); err != nil {
		t.Fatalf("count organizers: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'organizer_bootstrapped'`).Scan(&auditCount); err != nil {
		t.Fatalf("count bootstrap audit events: %v", err)
	}
	if organizerCount != 1 || auditCount != 1 {
		t.Fatalf("unexpected bootstrap persistence: organizers=%d audits=%d", organizerCount, auditCount)
	}
}

func createUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, clerkID string) string {
	t.Helper()
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.users (clerk_user_id, primary_email) VALUES ($1, $2) RETURNING id::text`, clerkID, clerkID+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create user %s: %v", clerkID, err)
	}
	return userID
}

func insertCycle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, slug string, active bool) string {
	t.Helper()
	var cycleID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.application_cycles (slug, name, applications_open_at, applications_close_at, active) VALUES ($1, $2, now(), now() + interval '1 day', $3) RETURNING id::text`, slug, strings.ToUpper(slug), active).Scan(&cycleID); err != nil {
		t.Fatalf("create cycle %s: %v", slug, err)
	}
	return cycleID
}

func insertForm(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID string, version int, creatorID string) string {
	t.Helper()
	var formID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.application_forms (cycle_id, version, schema_json, created_by) VALUES ($1, $2, '{}'::jsonb, $3) RETURNING id::text`, cycleID, version, creatorID).Scan(&formID); err != nil {
		t.Fatalf("create form version %d: %v", version, err)
	}
	return formID
}

func assertFalseQuery(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, name string) {
	t.Helper()
	var result bool
	if err := pool.QueryRow(ctx, query).Scan(&result); err != nil {
		t.Fatalf("check %s: %v", name, err)
	}
	if result {
		t.Fatalf("%s unexpectedly exists", name)
	}
}

func mustFail(t *testing.T, name string, operation func() error) {
	t.Helper()
	if err := operation(); err == nil {
		t.Fatalf("expected %s to fail", name)
	}
}
