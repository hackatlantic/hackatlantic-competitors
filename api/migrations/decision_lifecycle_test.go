//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lifecycleDecisionResponse struct {
	ID             string     `json:"id"`
	ApplicationID  string     `json:"applicationId"`
	Outcome        string     `json:"outcome"`
	InternalReason *string    `json:"internalReason"`
	SupersedesID   *string    `json:"supersedesId"`
	ReleasedAt     *time.Time `json:"releasedAt"`
}

type lifecycleApplicantDecisionResponse struct {
	ApplicationID string    `json:"applicationId"`
	Outcome       string    `json:"outcome"`
	ReleasedAt    time.Time `json:"releasedAt"`
}

func TestDecisionLifecyclePrivacyHistoryConversionAndRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	organizerID := createUser(t, ctx, pool, "clerk-decision-organizer")
	applicantID := createUser(t, ctx, pool, "clerk-decision-applicant")
	otherApplicantID := createUser(t, ctx, pool, "clerk-decision-other-applicant")
	waitlistedApplicantID := createUser(t, ctx, pool, "clerk-decision-waitlisted")
	rejectedApplicantID := createUser(t, ctx, pool, "clerk-decision-rejected")
	draftApplicantID := createUser(t, ctx, pool, "clerk-decision-draft")
	scannerID := createUser(t, ctx, pool, "clerk-decision-scanner")
	for _, fixture := range []struct {
		userID string
		role   string
	}{
		{organizerID, "organizer"},
		{scannerID, "scanner"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, $2)`, fixture.userID, fixture.role); err != nil {
			t.Fatalf("grant fixture role %s: %v", fixture.role, err)
		}
	}

	cycleID := insertCycle(t, ctx, pool, "decision-lifecycle", true)
	formID := insertWorkflowForm(t, ctx, pool, cycleID, organizerID)
	acceptedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, applicantID, "Accepted candidate")
	waitlistedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, waitlistedApplicantID, "Waitlisted candidate")
	rejectedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, rejectedApplicantID, "Rejected candidate")
	var draftApplicationID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id) VALUES ($1, $2, $3) RETURNING id::text`, cycleID, formID, draftApplicantID).Scan(&draftApplicationID); err != nil {
		t.Fatalf("create draft decision fixture: %v", err)
	}

	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool,
		Verifier:  intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-decision-organizer":       decisionTestUser(organizerID, "clerk-decision-organizer", users.RoleOrganizer),
			"clerk-decision-applicant":       decisionTestUser(applicantID, "clerk-decision-applicant", users.RoleApplicant),
			"clerk-decision-other-applicant": decisionTestUser(otherApplicantID, "clerk-decision-other-applicant", users.RoleApplicant),
			"clerk-decision-waitlisted":      decisionTestUser(waitlistedApplicantID, "clerk-decision-waitlisted", users.RoleApplicant),
			"clerk-decision-rejected":        decisionTestUser(rejectedApplicantID, "clerk-decision-rejected", users.RoleApplicant),
			"clerk-decision-draft":           decisionTestUser(draftApplicantID, "clerk-decision-draft", users.RoleApplicant),
			"clerk-decision-scanner":         decisionTestUser(scannerID, "clerk-decision-scanner", users.RoleScanner),
		}},
		Applications: applications.NewService(pool, 5*time.Second, 15*time.Second),
		Reviews:      reviews.NewService(pool, 5*time.Second, 15*time.Second),
		Decisions:    decisions.NewService(pool, 5*time.Second, 15*time.Second),
	}))
	defer server.Close()

	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+acceptedApplicationID+"/decisions", "clerk-decision-applicant", map[string]any{"outcome": "accepted"}), http.StatusForbidden)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+acceptedApplicationID+"/decisions", "clerk-decision-scanner", map[string]any{"outcome": "accepted"}), http.StatusForbidden)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+draftApplicationID+"/decisions", "clerk-decision-organizer", map[string]any{"outcome": "accepted"}), http.StatusConflict)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+acceptedApplicationID+"/decisions", "clerk-decision-organizer", map[string]any{"outcome": "not-an-outcome"}), http.StatusUnprocessableEntity)

	firstAccepted := recordLifecycleDecision(t, server.URL, acceptedApplicationID, "clerk-decision-organizer", "accepted", "Only organizers can see this reason")
	if firstAccepted.InternalReason == nil || *firstAccepted.InternalReason != "Only organizers can see this reason" || firstAccepted.ReleasedAt != nil {
		t.Fatalf("unexpected internal unreleased decision: %+v", firstAccepted)
	}
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/"+acceptedApplicationID+"/decision", "clerk-decision-applicant", nil), http.StatusNotFound)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/"+acceptedApplicationID+"/decision", "clerk-decision-other-applicant", nil), http.StatusNotFound)
	assertApplicantApplicationPrivacy(t, server.URL, acceptedApplicationID)
	assertAttendeePersistence(t, ctx, pool, acceptedApplicationID, 1, 1)

	secondAccepted := recordLifecycleDecision(t, server.URL, acceptedApplicationID, "clerk-decision-organizer", "accepted", "Replacement internal reason")
	if secondAccepted.SupersedesID == nil || *secondAccepted.SupersedesID != firstAccepted.ID {
		t.Fatalf("replacement decision did not supersede the previous decision: first=%+v second=%+v", firstAccepted, secondAccepted)
	}
	assertAttendeePersistence(t, ctx, pool, acceptedApplicationID, 1, 1)
	var historyCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.decisions WHERE application_id = $1`, acceptedApplicationID).Scan(&historyCount); err != nil {
		t.Fatalf("count accepted decision history: %v", err)
	}
	if historyCount != 2 {
		t.Fatalf("expected two append-only decisions, got %d", historyCount)
	}
	if _, err := pool.Exec(ctx, `UPDATE ats.decisions SET outcome = 'rejected' WHERE id = $1`, firstAccepted.ID); err == nil {
		t.Fatal("expected append-only decision update to be rejected")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ats.decisions WHERE id = $1`, firstAccepted.ID); err == nil {
		t.Fatal("expected append-only decision delete to be rejected")
	}
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/decisions/"+firstAccepted.ID+"/release", "clerk-decision-organizer", nil), http.StatusNotFound)

	organizerDetail := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications/"+acceptedApplicationID, "clerk-decision-organizer", nil)
	if organizerDetail.StatusCode != http.StatusOK {
		organizerDetail.Body.Close()
		t.Fatalf("get organizer decision detail: expected %d, got %d", http.StatusOK, organizerDetail.StatusCode)
	}
	var organizerPayload struct {
		CurrentDecision *lifecycleDecisionResponse `json:"currentDecision"`
	}
	decodeIntakeResponse(t, organizerDetail, &organizerPayload)
	if organizerPayload.CurrentDecision == nil || organizerPayload.CurrentDecision.ID != secondAccepted.ID {
		t.Fatalf("organizer detail omitted the current decision: %+v", organizerPayload)
	}

	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/decisions/"+secondAccepted.ID+"/release", "clerk-decision-applicant", nil), http.StatusForbidden)
	released := releaseLifecycleDecision(t, server.URL, secondAccepted.ID, "clerk-decision-organizer")
	if released.ReleasedAt == nil || released.Outcome != "accepted" {
		t.Fatalf("unexpected released decision: %+v", released)
	}
	repeatedRelease := releaseLifecycleDecision(t, server.URL, secondAccepted.ID, "clerk-decision-organizer")
	if repeatedRelease.ReleasedAt == nil || !repeatedRelease.ReleasedAt.Equal(*released.ReleasedAt) {
		t.Fatalf("release was not idempotent: first=%+v repeated=%+v", released, repeatedRelease)
	}
	assertDecisionReleasePersistence(t, ctx, pool, acceptedApplicationID, secondAccepted.ID)

	applicantDecisionResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/"+acceptedApplicationID+"/decision", "clerk-decision-applicant", nil)
	if applicantDecisionResponse.StatusCode != http.StatusOK {
		applicantDecisionResponse.Body.Close()
		t.Fatalf("get released applicant decision: expected %d, got %d", http.StatusOK, applicantDecisionResponse.StatusCode)
	}
	var applicantPayload map[string]any
	decodeIntakeResponse(t, applicantDecisionResponse, &applicantPayload)
	encodedApplicantDecision, err := json.Marshal(applicantPayload)
	if err != nil {
		t.Fatalf("marshal released applicant decision: %v", err)
	}
	for _, forbidden := range []string{"internalReason", "recommendation", "internalNotes", "score", "review"} {
		if strings.Contains(string(encodedApplicantDecision), forbidden) {
			t.Fatalf("released applicant decision leaked %q: %s", forbidden, encodedApplicantDecision)
		}
	}
	var applicantDecision lifecycleApplicantDecisionResponse
	if err := json.Unmarshal(encodedApplicantDecision, &applicantDecision); err != nil {
		t.Fatalf("decode released applicant decision: %v", err)
	}
	if applicantDecision.ApplicationID != acceptedApplicationID || applicantDecision.Outcome != "accepted" || applicantDecision.ReleasedAt.IsZero() {
		t.Fatalf("unexpected applicant decision: %+v", applicantDecision)
	}
	assertApplicantApplicationPrivacy(t, server.URL, acceptedApplicationID)
	assertDecisionStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/"+acceptedApplicationID+"/decision", "clerk-decision-other-applicant", nil), http.StatusNotFound)

	recordLifecycleDecision(t, server.URL, waitlistedApplicationID, "clerk-decision-organizer", "waitlisted", "Internal waitlist priority")
	recordLifecycleDecision(t, server.URL, rejectedApplicationID, "clerk-decision-organizer", "rejected", "Internal rejection reason")
	assertAttendeePersistence(t, ctx, pool, waitlistedApplicationID, 0, 0)
	assertAttendeePersistence(t, ctx, pool, rejectedApplicationID, 0, 0)
	assertDecisionAuditCounts(t, ctx, pool, acceptedApplicationID, 2, 1, 1)
	assertDecisionAuditCounts(t, ctx, pool, waitlistedApplicationID, 1, 0, 0)
	assertDecisionAuditCounts(t, ctx, pool, rejectedApplicationID, 1, 0, 0)
}

func decisionTestUser(id, clerkID string, role users.Role) users.User {
	return users.User{
		ID: id, ClerkUserID: clerkID, Email: clerkID + "@example.test",
		Roles: map[users.Role]struct{}{role: {}},
	}
}

func recordLifecycleDecision(t *testing.T, baseURL, applicationID, clerkID, outcome, internalReason string) lifecycleDecisionResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/applications/"+applicationID+"/decisions", clerkID, map[string]any{"outcome": outcome, "internalReason": internalReason})
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("record %s decision: expected %d, got %d", outcome, http.StatusCreated, response.StatusCode)
	}
	var decision lifecycleDecisionResponse
	decodeIntakeResponse(t, response, &decision)
	if decision.ID == "" || decision.ApplicationID != applicationID || decision.Outcome != outcome {
		t.Fatalf("unexpected recorded decision: %+v", decision)
	}
	return decision
}

func releaseLifecycleDecision(t *testing.T, baseURL, decisionID, clerkID string) lifecycleDecisionResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/decisions/"+decisionID+"/release", clerkID, nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("release decision: expected %d, got %d", http.StatusOK, response.StatusCode)
	}
	var decision lifecycleDecisionResponse
	decodeIntakeResponse(t, response, &decision)
	return decision
}

func assertDecisionStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expected {
		t.Fatalf("expected decision endpoint status %d, got %d", expected, response.StatusCode)
	}
}

func assertApplicantApplicationPrivacy(t *testing.T, baseURL, applicationID string) {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodGet, "/v1/applications/mine", "clerk-decision-applicant", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("list applicant applications: expected %d, got %d", http.StatusOK, response.StatusCode)
	}
	var payload map[string]any
	decodeIntakeResponse(t, response, &payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal applicant projection: %v", err)
	}
	for _, forbidden := range []string{"internalReason", "recommendation", "internalNotes", "score", "review"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("applicant projection leaked %q: %s", forbidden, encoded)
		}
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected applicant application list: %+v", payload)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["id"] != applicationID || item["status"] != "submitted" {
		t.Fatalf("applicant application status leaked a decision: %+v", item)
	}
}

func assertAttendeePersistence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applicationID string, attendees, hackerRoles int) {
	t.Helper()
	var attendeeCount, roleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.attendees WHERE application_id = $1`, applicationID).Scan(&attendeeCount); err != nil {
		t.Fatalf("count attendees: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.attendee_roles AS roles JOIN ats.attendees ON attendees.id = roles.attendee_id WHERE attendees.application_id = $1 AND roles.role = 'hacker'`, applicationID).Scan(&roleCount); err != nil {
		t.Fatalf("count hacker attendee roles: %v", err)
	}
	if attendeeCount != attendees || roleCount != hackerRoles {
		t.Fatalf("unexpected attendee conversion for application %s: attendees=%d hackerRoles=%d", applicationID, attendeeCount, roleCount)
	}
}

func assertDecisionReleasePersistence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applicationID, decisionID string) {
	t.Helper()
	var currentDecisionID, status string
	var releasedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT current_decision_id::text, status, decision_released_at FROM ats.applications WHERE id = $1`, applicationID).Scan(&currentDecisionID, &status, &releasedAt); err != nil {
		t.Fatalf("load decision cache: %v", err)
	}
	if currentDecisionID != decisionID || status != "accepted" || releasedAt == nil {
		t.Fatalf("application decision cache was not atomically released: current=%s status=%s released=%v", currentDecisionID, status, releasedAt)
	}
	var outboxCount int
	var templateData string
	if err := pool.QueryRow(ctx, `SELECT count(*), COALESCE(max(template_data_json::text), '') FROM ats.email_outbox WHERE event_type = 'decision_release' AND dedupe_key = $1`, "decision_release:"+decisionID).Scan(&outboxCount, &templateData); err != nil {
		t.Fatalf("load decision outbox: %v", err)
	}
	if outboxCount != 1 || !strings.Contains(templateData, "accepted") || !strings.Contains(templateData, applicationID) {
		t.Fatalf("unexpected decision outbox: count=%d template=%s", outboxCount, templateData)
	}
	for _, forbidden := range []string{"internalReason", "reason", "recommendation", "internalNotes", "score", "review"} {
		if strings.Contains(templateData, forbidden) {
			t.Fatalf("decision email template data leaked %q: %s", forbidden, templateData)
		}
	}
}

func assertDecisionAuditCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applicationID string, recorded, attendeeCreated, released int) {
	t.Helper()
	var recordedCount, attendeeCount, releaseCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'decision_recorded' AND metadata_json ->> 'applicationId' = $1`, applicationID).Scan(&recordedCount); err != nil {
		t.Fatalf("count decision record audit events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'attendee_created' AND metadata_json ->> 'applicationId' = $1`, applicationID).Scan(&attendeeCount); err != nil {
		t.Fatalf("count attendee creation audit events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'decision_released' AND metadata_json ->> 'applicationId' = $1`, applicationID).Scan(&releaseCount); err != nil {
		t.Fatalf("count decision release audit events: %v", err)
	}
	if recordedCount != recorded || attendeeCount != attendeeCreated || releaseCount != released {
		t.Fatalf("unexpected decision audit events for %s: recorded=%d attendees=%d released=%d", applicationID, recordedCount, attendeeCount, releaseCount)
	}
}
