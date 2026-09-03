//go:build integration

package migrations_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lifecyclePassResponse struct {
	ID          string     `json:"id"`
	AttendeeID  string     `json:"attendeeId"`
	DisplayName string     `json:"displayName"`
	Status      string     `json:"status"`
	IssuedAt    time.Time  `json:"issuedAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
	QRToken     string     `json:"qrToken"`
	ClaimToken  string     `json:"claimToken"`
	ClaimURL    string     `json:"claimUrl"`
}

func TestPassLifecycleCredentialsAndAccessBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	organizerID := createUser(t, ctx, pool, "clerk-pass-organizer")
	applicantID := createUser(t, ctx, pool, "clerk-pass-applicant")
	otherApplicantID := createUser(t, ctx, pool, "clerk-pass-other")
	concurrentApplicantID := createUser(t, ctx, pool, "clerk-pass-concurrent")
	staleApplicantID := createUser(t, ctx, pool, "clerk-pass-stale")
	rejectedApplicantID := createUser(t, ctx, pool, "clerk-pass-rejected")
	if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, 'organizer')`, organizerID); err != nil {
		t.Fatalf("grant organizer: %v", err)
	}

	cycleID := insertCycle(t, ctx, pool, "pass-lifecycle", true)
	formID := insertWorkflowForm(t, ctx, pool, cycleID, organizerID)
	applicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, applicantID, "Pass candidate")
	concurrentApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, concurrentApplicantID, "Concurrent candidate")
	staleApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, staleApplicantID, "Stale candidate")
	rejectedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, rejectedApplicantID, "Rejected candidate")

	passService, err := passes.NewService(pool, 5*time.Second, 15*time.Second, passes.Config{
		QRTokenPepper: encodedTestPepper(0x31), ClaimTokenPepper: encodedTestPepper(0x32), AppBaseURL: "https://app.example",
	})
	if err != nil {
		t.Fatalf("create pass service: %v", err)
	}
	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool, Verifier: intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-pass-organizer":  decisionTestUser(organizerID, "clerk-pass-organizer", users.RoleOrganizer),
			"clerk-pass-applicant":  decisionTestUser(applicantID, "clerk-pass-applicant", users.RoleApplicant),
			"clerk-pass-other":      decisionTestUser(otherApplicantID, "clerk-pass-other", users.RoleApplicant),
			"clerk-pass-concurrent": decisionTestUser(concurrentApplicantID, "clerk-pass-concurrent", users.RoleApplicant),
			"clerk-pass-stale":      decisionTestUser(staleApplicantID, "clerk-pass-stale", users.RoleApplicant),
		}},
		Applications: applications.NewService(pool, 5*time.Second, 15*time.Second),
		Reviews:      reviews.NewService(pool, 5*time.Second, 15*time.Second),
		Decisions:    decisions.NewService(pool, 5*time.Second, 15*time.Second),
		Passes:       passService,
	}))
	defer server.Close()

	accepted := recordLifecycleDecision(t, server.URL, applicationID, "clerk-pass-organizer", "accepted", "accepted for pass")
	attendeeID := attendeeForApplication(t, ctx, pool, applicationID)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/attendees/"+attendeeID+"/passes", "clerk-pass-organizer", nil), http.StatusNotFound)
	releaseLifecycleDecision(t, server.URL, accepted.ID, "clerk-pass-organizer")
	confirmRSVPFixture(t, ctx, pool, accepted.ID)

	issued := issueLifecyclePass(t, server.URL, attendeeID)
	if issued.QRToken == "" || issued.ClaimToken == "" || issued.QRToken == issued.ClaimToken || issued.ClaimURL == "" {
		t.Fatal("issuance omitted distinct credential fields")
	}
	if !strings.Contains(issued.ClaimURL, issued.ClaimToken) || issued.Status != "active" || issued.IssuedAt.IsZero() {
		t.Fatal("issuance did not contain safe active pass metadata and claim URL")
	}
	assertPassSecretPersistence(t, ctx, pool, issued)
	organizerDetail := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications/"+applicationID, "clerk-pass-organizer", nil)
	if organizerDetail.StatusCode != http.StatusOK {
		organizerDetail.Body.Close()
		t.Fatalf("get organizer pass summary: got %d", organizerDetail.StatusCode)
	}
	var organizerPayload struct {
		AttendeePass *struct {
			AttendeeID string         `json:"attendeeId"`
			Pass       map[string]any `json:"pass"`
		} `json:"attendeePass"`
	}
	decodeIntakeResponse(t, organizerDetail, &organizerPayload)
	if organizerPayload.AttendeePass == nil || organizerPayload.AttendeePass.AttendeeID != attendeeID || organizerPayload.AttendeePass.Pass["id"] != issued.ID {
		t.Fatal("organizer pass summary did not provide the safe issue target and active pass")
	}
	assertNoCredentialFields(t, organizerPayload.AttendeePass.Pass)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/attendees/"+attendeeID+"/passes", "clerk-pass-organizer", nil), http.StatusConflict)

	web := intakeRequest(t, server.URL, http.MethodGet, "/v1/attendee/pass", "clerk-pass-applicant", nil)
	if web.StatusCode != http.StatusOK {
		web.Body.Close()
		t.Fatalf("get authenticated web pass: got %d", web.StatusCode)
	}
	var webPass map[string]any
	decodeIntakeResponse(t, web, &webPass)
	assertNoClaimCredentialFields(t, webPass)
	if webPass["id"] != issued.ID || webPass["attendeeId"] != attendeeID || webPass["qrToken"] != issued.QRToken {
		t.Fatal("authenticated web pass metadata did not identify the issued pass and derived QR")
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/attendee/pass", "clerk-pass-other", nil), http.StatusNotFound)
	claim := intakeRequest(t, server.URL, http.MethodGet, "/v1/claim/"+issued.ClaimToken, "", nil)
	if claim.StatusCode != http.StatusOK {
		claim.Body.Close()
		t.Fatalf("resolve public claim: got %d", claim.StatusCode)
	}
	var claimPass map[string]any
	decodeIntakeResponse(t, claim, &claimPass)
	for _, field := range []string{"attendeeId", "email", "qrToken", "claimToken", "claimUrl", "qrTokenHash", "claimTokenHash"} {
		if _, ok := claimPass[field]; ok {
			t.Fatalf("public claim leaked %s", field)
		}
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/claim/"+issued.QRToken, "", nil), http.StatusNotFound)

	reissued := reissueLifecyclePass(t, server.URL, issued.ID)
	if reissued.ID == issued.ID || reissued.QRToken == issued.QRToken || reissued.ClaimToken == issued.ClaimToken {
		t.Fatalf("reissue did not replace every credential for old pass %s", issued.ID)
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/claim/"+issued.ClaimToken, "", nil), http.StatusNotFound)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/claim/"+reissued.ClaimToken, "", nil), http.StatusOK)
	assertPassReissuePersistence(t, ctx, pool, issued.ID, reissued.ID)
	assertPassSecretPersistence(t, ctx, pool, reissued)
	reissuedWeb := intakeRequest(t, server.URL, http.MethodGet, "/v1/attendee/pass", "clerk-pass-applicant", nil)
	if reissuedWeb.StatusCode != http.StatusOK {
		reissuedWeb.Body.Close()
		t.Fatalf("get reissued attendee web pass: got %d", reissuedWeb.StatusCode)
	}
	var reissuedWebPass map[string]any
	decodeIntakeResponse(t, reissuedWeb, &reissuedWebPass)
	assertNoClaimCredentialFields(t, reissuedWebPass)
	if reissuedWebPass["id"] != reissued.ID || reissuedWebPass["qrToken"] != reissued.QRToken {
		t.Fatal("reissued attendee web pass did not derive the active QR credential")
	}

	revoked := revokeLifecyclePass(t, server.URL, reissued.ID)
	if revoked.Status != "revoked" || revoked.RevokedAt == nil {
		t.Fatalf("revocation did not transition pass %s", reissued.ID)
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/claim/"+reissued.ClaimToken, "", nil), http.StatusNotFound)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/attendee/pass", "clerk-pass-applicant", nil), http.StatusNotFound)
	assertPassAuditAndOutbox(t, ctx, pool, attendeeID, issued, reissued)
	rateLimitedServer := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool, Passes: passService, ClaimRateLimiter: httpapi.NewClaimRateLimiter(1, time.Minute, 4),
	}))
	defer rateLimitedServer.Close()
	assertPassStatus(t, intakeRequest(t, rateLimitedServer.URL, http.MethodGet, "/v1/claim/not-a-credential", "", nil), http.StatusNotFound)
	assertPassStatus(t, intakeRequest(t, rateLimitedServer.URL, http.MethodGet, "/v1/claim/another-not-a-credential", "", nil), http.StatusTooManyRequests)

	concurrentDecision := recordLifecycleDecision(t, server.URL, concurrentApplicationID, "clerk-pass-organizer", "accepted", "accepted concurrent")
	releaseLifecycleDecision(t, server.URL, concurrentDecision.ID, "clerk-pass-organizer")
	confirmRSVPFixture(t, ctx, pool, concurrentDecision.ID)
	concurrentAttendeeID := attendeeForApplication(t, ctx, pool, concurrentApplicationID)
	statuses := concurrentlyIssuePasses(t, server.URL, concurrentAttendeeID)
	if !(statuses[0] == http.StatusCreated && statuses[1] == http.StatusConflict || statuses[0] == http.StatusConflict && statuses[1] == http.StatusCreated) {
		t.Fatalf("concurrent issuance did not serialize to one success and one conflict: %v", statuses)
	}
	var activeCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.passes WHERE attendee_id = $1 AND status = 'active'`, concurrentAttendeeID).Scan(&activeCount); err != nil {
		t.Fatalf("count concurrent active passes: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one active pass after concurrent issue, got %d", activeCount)
	}

	staleDecision := recordLifecycleDecision(t, server.URL, staleApplicationID, "clerk-pass-organizer", "accepted", "initial acceptance")
	releaseLifecycleDecision(t, server.URL, staleDecision.ID, "clerk-pass-organizer")
	confirmRSVPFixture(t, ctx, pool, staleDecision.ID)
	staleAttendeeID := attendeeForApplication(t, ctx, pool, staleApplicationID)
	assertPassDatabaseConstraints(t, ctx, pool, concurrentAttendeeID, staleAttendeeID)
	recordLifecycleDecision(t, server.URL, staleApplicationID, "clerk-pass-organizer", "waitlisted", "later status change")
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/attendees/"+staleAttendeeID+"/passes", "clerk-pass-organizer", nil), http.StatusNotFound)
	rejectedDecision := recordLifecycleDecision(t, server.URL, rejectedApplicationID, "clerk-pass-organizer", "accepted", "initial acceptance")
	releaseLifecycleDecision(t, server.URL, rejectedDecision.ID, "clerk-pass-organizer")
	confirmRSVPFixture(t, ctx, pool, rejectedDecision.ID)
	rejectedAttendeeID := attendeeForApplication(t, ctx, pool, rejectedApplicationID)
	recordLifecycleDecision(t, server.URL, rejectedApplicationID, "clerk-pass-organizer", "rejected", "later rejection")
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/attendees/"+rejectedAttendeeID+"/passes", "clerk-pass-organizer", nil), http.StatusNotFound)
}

func issueLifecyclePass(t *testing.T, baseURL, attendeeID string) lifecyclePassResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/attendees/"+attendeeID+"/passes", "clerk-pass-organizer", nil)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("issue pass: expected %d, got %d", http.StatusCreated, response.StatusCode)
	}
	var pass lifecyclePassResponse
	decodeIntakeResponse(t, response, &pass)
	return pass
}

func reissueLifecyclePass(t *testing.T, baseURL, passID string) lifecyclePassResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/passes/"+passID+"/reissue", "clerk-pass-organizer", nil)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("reissue pass: expected %d, got %d", http.StatusCreated, response.StatusCode)
	}
	var pass lifecyclePassResponse
	decodeIntakeResponse(t, response, &pass)
	return pass
}

func revokeLifecyclePass(t *testing.T, baseURL, passID string) lifecyclePassResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/passes/"+passID+"/revoke", "clerk-pass-organizer", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("revoke pass: expected %d, got %d", http.StatusOK, response.StatusCode)
	}
	var pass lifecyclePassResponse
	decodeIntakeResponse(t, response, &pass)
	return pass
}

func attendeeForApplication(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, applicationID string) string {
	t.Helper()
	var attendeeID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM ats.attendees WHERE application_id = $1`, applicationID).Scan(&attendeeID); err != nil {
		t.Fatalf("load attendee for application: %v", err)
	}
	return attendeeID
}

func assertPassStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expected {
		t.Fatalf("expected pass endpoint status %d, got %d", expected, response.StatusCode)
	}
}

func assertNoCredentialFields(t *testing.T, body map[string]any) {
	t.Helper()
	for _, key := range []string{"qrToken", "claimToken", "claimUrl", "qrTokenHash", "claimTokenHash"} {
		if _, ok := body[key]; ok {
			t.Fatalf("safe response leaked %s", key)
		}
	}
}

func assertNoClaimCredentialFields(t *testing.T, body map[string]any) {
	t.Helper()
	for _, key := range []string{"claimToken", "claimUrl", "qrTokenHash", "claimTokenHash"} {
		if _, ok := body[key]; ok {
			t.Fatalf("web pass leaked %s", key)
		}
	}
}

func assertPassSecretPersistence(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, issued lifecyclePassResponse) {
	t.Helper()
	var qrHash, claimHash, auditData, outboxData string
	if err := pool.QueryRow(ctx, `SELECT encode(passes.qr_token_hash, 'hex'), encode(passes.claim_token_hash, 'hex'), COALESCE((SELECT string_agg(metadata_json::text, ' ') FROM ats.audit_events WHERE metadata_json ->> 'attendeeId' = passes.attendee_id::text), ''), COALESCE((SELECT string_agg(template_data_json::text, ' ') FROM ats.email_outbox WHERE dedupe_key = 'pass_link:' || passes.id::text), '') FROM ats.passes AS passes WHERE passes.id = $1`, issued.ID).Scan(&qrHash, &claimHash, &auditData, &outboxData); err != nil {
		t.Fatalf("load credential persistence: %v", err)
	}
	if len(qrHash) != 64 || len(claimHash) != 64 || qrHash == claimHash {
		t.Fatalf("credentials were not stored as distinct 256-bit hashes: qrLength=%d claimLength=%d equal=%t", len(qrHash), len(claimHash), qrHash == claimHash)
	}
	if !strings.Contains(outboxData, "webPassUrl") || !strings.Contains(outboxData, "/attendee/pass") {
		t.Fatal("pass-link outbox event omitted the sanitized web-pass link")
	}
	for _, material := range []struct {
		name  string
		value string
	}{
		{name: "QR credential", value: issued.QRToken},
		{name: "claim credential", value: issued.ClaimToken},
		{name: "QR hash", value: qrHash},
		{name: "claim hash", value: claimHash},
	} {
		if strings.Contains(auditData, material.value) || strings.Contains(outboxData, material.value) {
			t.Fatalf("audit or outbox leaked %s", material.name)
		}
	}
}

func assertPassReissuePersistence(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, oldID, replacementID string) {
	t.Helper()
	var status, replacedBy string
	if err := pool.QueryRow(ctx, `SELECT status, replaced_by_pass_id::text FROM ats.passes WHERE id = $1`, oldID).Scan(&status, &replacedBy); err != nil {
		t.Fatalf("load replaced pass: %v", err)
	}
	if status != "replaced" || replacedBy != replacementID {
		t.Fatalf("old pass was not atomically superseded: status=%s replacement=%s", status, replacedBy)
	}
}

func assertPassAuditAndOutbox(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, attendeeID string, issued, reissued lifecyclePassResponse) {
	t.Helper()
	var issuedAudit, reissuedAudit, revokedAudit, outboxCount int
	if err := pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM ats.audit_events WHERE event_type = 'pass_issued' AND metadata_json ->> 'attendeeId' = $1),
        (SELECT count(*) FROM ats.audit_events WHERE event_type = 'pass_reissued' AND metadata_json ->> 'attendeeId' = $1),
        (SELECT count(*) FROM ats.audit_events WHERE event_type = 'pass_revoked' AND metadata_json ->> 'attendeeId' = $1),
        (SELECT count(*) FROM ats.email_outbox WHERE dedupe_key IN ($2, $3))`,
		attendeeID, "pass_link:"+issued.ID, "pass_link:"+reissued.ID,
	).Scan(&issuedAudit, &reissuedAudit, &revokedAudit, &outboxCount); err != nil {
		t.Fatalf("count pass audit/outbox: %v", err)
	}
	if issuedAudit != 1 || reissuedAudit != 1 || revokedAudit != 1 || outboxCount != 2 {
		t.Fatalf("unexpected pass audit/outbox counts: issued=%d reissued=%d revoked=%d outbox=%d", issuedAudit, reissuedAudit, revokedAudit, outboxCount)
	}
}

func assertPassDatabaseConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool, activeAttendeeID, constraintAttendeeID string) {
	t.Helper()
	attempts := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "second active pass",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash)
                VALUES ($1, decode(repeat('01', 32), 'hex'), decode(repeat('02', 32), 'hex'))`,
			args: []any{activeAttendeeID},
		},
		{
			name: "short QR hash",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash)
                VALUES ($1, decode(repeat('03', 31), 'hex'), decode(repeat('04', 32), 'hex'))`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "long claim hash",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash)
                VALUES ($1, decode(repeat('05', 32), 'hex'), decode(repeat('06', 33), 'hex'))`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "revoked without timestamp",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash, status)
                VALUES ($1, decode(repeat('07', 32), 'hex'), decode(repeat('08', 32), 'hex'), 'revoked')`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "active with revocation timestamp",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash, revoked_at)
                VALUES ($1, decode(repeat('09', 32), 'hex'), decode(repeat('0a', 32), 'hex'), CURRENT_TIMESTAMP)`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "replaced without successor",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash, status, revoked_at)
                VALUES ($1, decode(repeat('0b', 32), 'hex'), decode(repeat('0c', 32), 'hex'), 'replaced', CURRENT_TIMESTAMP)`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "self replacement",
			sql: `WITH generated AS (SELECT gen_random_uuid() AS id)
                INSERT INTO ats.passes (id, attendee_id, qr_token_hash, claim_token_hash, status, revoked_at, replaced_by_pass_id)
                SELECT id, $1, decode(repeat('0d', 32), 'hex'), decode(repeat('0e', 32), 'hex'), 'replaced', CURRENT_TIMESTAMP, id
                FROM generated`,
			args: []any{constraintAttendeeID},
		},
		{
			name: "cross-attendee replacement",
			sql: `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash, status, revoked_at, replaced_by_pass_id)
                VALUES ($1, decode(repeat('0f', 32), 'hex'), decode(repeat('10', 32), 'hex'), 'replaced', CURRENT_TIMESTAMP,
                    (SELECT id FROM ats.passes WHERE attendee_id = $2 AND status = 'active'))`,
			args: []any{constraintAttendeeID, activeAttendeeID},
		},
	}
	for _, attempt := range attempts {
		if _, err := pool.Exec(ctx, attempt.sql, attempt.args...); err == nil {
			t.Fatalf("database accepted invalid pass state: %s", attempt.name)
		}
	}
}

func concurrentlyIssuePasses(t *testing.T, baseURL, attendeeID string) [2]int {
	t.Helper()
	var statuses [2]int
	var wait sync.WaitGroup
	wait.Add(len(statuses))
	for index := range statuses {
		go func(index int) {
			defer wait.Done()
			request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/admin/attendees/"+attendeeID+"/passes", nil)
			if err != nil {
				t.Errorf("create concurrent issue request: %v", err)
				return
			}
			request.Header.Set("Authorization", "Bearer clerk-pass-organizer")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Errorf("perform concurrent issue request: %v", err)
				return
			}
			defer response.Body.Close()
			statuses[index] = response.StatusCode
		}(index)
	}
	wait.Wait()
	return statuses
}
