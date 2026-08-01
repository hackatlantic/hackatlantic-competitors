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
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workflowApplicationResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Assignment *struct {
		AssignedBy string    `json:"assignedBy"`
		AssignedAt time.Time `json:"assignedAt"`
	} `json:"assignment"`
	Review *struct {
		ID             string     `json:"id"`
		Status         string     `json:"status"`
		Score          int32      `json:"score"`
		Recommendation string     `json:"recommendation"`
		InternalNotes  *string    `json:"internalNotes"`
		LockVersion    int32      `json:"lockVersion"`
		SubmittedAt    *time.Time `json:"submittedAt"`
	} `json:"review"`
}

type workflowListResponse struct {
	Items []workflowApplicationResponse `json:"items"`
}

func TestReviewerWorkflowAuthorizationVisibilityAndImmutability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	organizerID := createUser(t, ctx, pool, "clerk-workflow-organizer")
	reviewerID := createUser(t, ctx, pool, "clerk-workflow-reviewer")
	roleTargetID := createUser(t, ctx, pool, "clerk-workflow-role-target")
	applicantID := createUser(t, ctx, pool, "clerk-workflow-applicant")
	otherApplicantID := createUser(t, ctx, pool, "clerk-workflow-other-applicant")
	scannerID := createUser(t, ctx, pool, "clerk-workflow-scanner")
	for _, fixture := range []struct {
		userID string
		role   string
	}{
		{organizerID, "organizer"},
		{reviewerID, "reviewer"},
		{scannerID, "scanner"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, $2)`, fixture.userID, fixture.role); err != nil {
			t.Fatalf("grant fixture role %s: %v", fixture.role, err)
		}
	}
	cycleID := insertCycle(t, ctx, pool, "reviewer-workflow", true)
	formID := insertWorkflowForm(t, ctx, pool, cycleID, organizerID)
	assignedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, applicantID, "Assigned candidate")
	unassignedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, otherApplicantID, "Search target")
	draftApplicationID := insertDraftWorkflowApplication(t, ctx, pool, cycleID, formID, roleTargetID, "Private draft answer")

	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool,
		Verifier:  intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-workflow-organizer":   {ID: organizerID, ClerkUserID: "clerk-workflow-organizer", Email: "organizer@example.test", Roles: map[users.Role]struct{}{users.RoleOrganizer: {}}},
			"clerk-workflow-reviewer":    {ID: reviewerID, ClerkUserID: "clerk-workflow-reviewer", Email: "reviewer@example.test", Roles: map[users.Role]struct{}{users.RoleReviewer: {}}},
			"clerk-workflow-role-target": {ID: roleTargetID, ClerkUserID: "clerk-workflow-role-target", Email: "target@example.test", Roles: map[users.Role]struct{}{users.RoleApplicant: {}}},
			"clerk-workflow-applicant":   {ID: applicantID, ClerkUserID: "clerk-workflow-applicant", Email: "applicant@example.test", Roles: map[users.Role]struct{}{users.RoleApplicant: {}}},
			"clerk-workflow-scanner":     {ID: scannerID, ClerkUserID: "clerk-workflow-scanner", Email: "scanner@example.test", Roles: map[users.Role]struct{}{users.RoleScanner: {}}},
		}},
		Applications: applications.NewService(pool, 5*time.Second, 15*time.Second),
		Reviews:      reviews.NewService(pool, 5*time.Second, 15*time.Second),
		StaffRoles:   users.NewService(pool, nil, 5*time.Second),
	}))
	defer server.Close()

	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/reviewer/assignments", "clerk-workflow-applicant", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/reviewer/assignments", "clerk-workflow-scanner", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications", "clerk-workflow-applicant", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications", "clerk-workflow-scanner", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications", "clerk-workflow-reviewer", nil), http.StatusForbidden)
	organizerUnfiltered := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications", "clerk-workflow-organizer", nil)
	if organizerUnfiltered.StatusCode != http.StatusOK {
		organizerUnfiltered.Body.Close()
		t.Fatalf("expected organizer application list status %d, got %d", http.StatusOK, organizerUnfiltered.StatusCode)
	}
	var organizerVisible workflowListResponse
	decodeIntakeResponse(t, organizerUnfiltered, &organizerVisible)
	for _, item := range organizerVisible.Items {
		if item.ID == draftApplicationID {
			t.Fatalf("organizer list exposed unsubmitted draft %s", draftApplicationID)
		}
	}
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications/"+draftApplicationID, "clerk-workflow-organizer", nil), http.StatusNotFound)

	grantRole := intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/users/"+roleTargetID+"/roles/reviewer", "clerk-workflow-organizer", nil)
	assertWorkflowStatus(t, grantRole, http.StatusNoContent)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/users/"+roleTargetID+"/roles/scanner", "clerk-workflow-applicant", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/users/"+organizerID+"/roles/scanner", "clerk-workflow-organizer", nil), http.StatusForbidden)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/users/"+roleTargetID+"/roles/scanner", "clerk-workflow-organizer", nil), http.StatusNoContent)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/users/"+roleTargetID+"/roles/scanner", "clerk-workflow-organizer", nil), http.StatusNoContent)
	assertWorkflowAudit(t, ctx, pool, "scanner_role_assigned", roleTargetID, 1)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/users/"+roleTargetID+"/roles/scanner", "clerk-workflow-organizer", nil), http.StatusNoContent)
	assertWorkflowStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/users/"+roleTargetID+"/roles/scanner", "clerk-workflow-organizer", nil), http.StatusNoContent)
	assertWorkflowAudit(t, ctx, pool, "scanner_role_revoked", roleTargetID, 1)
	var scannerRoleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.user_roles WHERE user_id = $1 AND role = 'scanner'`, roleTargetID).Scan(&scannerRoleCount); err != nil {
		t.Fatalf("count scanner roles after revocation: %v", err)
	}
	if scannerRoleCount != 0 {
		t.Fatalf("expected scanner role to be revoked, got %d rows", scannerRoleCount)
	}
	assign := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+assignedApplicationID+"/assignments", "clerk-workflow-organizer", map[string]any{"reviewerUserId": reviewerID})
	assertWorkflowStatus(t, assign, http.StatusOK)
	repeatedAssign := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/applications/"+assignedApplicationID+"/assignments", "clerk-workflow-organizer", map[string]any{"reviewerUserId": reviewerID})
	assertWorkflowStatus(t, repeatedAssign, http.StatusOK)
	assertWorkflowAudit(t, ctx, pool, "reviewer_role_assigned", roleTargetID, 1)
	assertWorkflowAudit(t, ctx, pool, "reviewer_assigned", assignedApplicationID, 1)

	filtered := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/applications?status=submitted&q=Search%20target", "clerk-workflow-organizer", nil)
	if filtered.StatusCode != http.StatusOK {
		filtered.Body.Close()
		t.Fatalf("expected organizer server filter status %d, got %d", http.StatusOK, filtered.StatusCode)
	}
	var organizerList workflowListResponse
	decodeIntakeResponse(t, filtered, &organizerList)
	if len(organizerList.Items) != 1 || organizerList.Items[0].ID != unassignedApplicationID {
		t.Fatalf("organizer filter did not return only searched submitted application: %+v", organizerList)
	}

	queue := intakeRequest(t, server.URL, http.MethodGet, "/v1/reviewer/assignments", "clerk-workflow-reviewer", nil)
	if queue.StatusCode != http.StatusOK {
		queue.Body.Close()
		t.Fatalf("expected reviewer queue status %d, got %d", http.StatusOK, queue.StatusCode)
	}
	var reviewerQueue workflowListResponse
	decodeIntakeResponse(t, queue, &reviewerQueue)
	if len(reviewerQueue.Items) != 2 {
		t.Fatalf("reviewer queue should contain every submitted application, got %+v", reviewerQueue)
	}
	byID := make(map[string]workflowApplicationResponse, len(reviewerQueue.Items))
	for _, item := range reviewerQueue.Items {
		byID[item.ID] = item
	}
	if byID[assignedApplicationID].Assignment == nil || byID[unassignedApplicationID].Assignment != nil {
		t.Fatalf("assignment metadata did not distinguish assigned queue item: %+v", reviewerQueue)
	}

	unassignedDetail := intakeRequest(t, server.URL, http.MethodGet, "/v1/reviewer/applications/"+unassignedApplicationID, "clerk-workflow-reviewer", nil)
	if unassignedDetail.StatusCode != http.StatusOK {
		unassignedDetail.Body.Close()
		t.Fatalf("expected reviewer access to unassigned submitted application, got %d", unassignedDetail.StatusCode)
	}
	var reviewApplication workflowApplicationResponse
	decodeIntakeResponse(t, unassignedDetail, &reviewApplication)
	if reviewApplication.Review != nil {
		t.Fatalf("unexpected pre-draft review: %+v", reviewApplication.Review)
	}

	invalidReview := intakeRequest(t, server.URL, http.MethodPut, "/v1/reviewer/applications/"+assignedApplicationID+"/review", "clerk-workflow-reviewer", map[string]any{"lockVersion": 0, "score": 6, "recommendation": "yes"})
	assertWorkflowStatus(t, invalidReview, http.StatusUnprocessableEntity)
	invalidRecommendation := intakeRequest(t, server.URL, http.MethodPut, "/v1/reviewer/applications/"+assignedApplicationID+"/review", "clerk-workflow-reviewer", map[string]any{"lockVersion": 0, "score": 3, "recommendation": "maybe"})
	assertWorkflowStatus(t, invalidRecommendation, http.StatusUnprocessableEntity)
	firstSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/reviewer/applications/"+assignedApplicationID+"/review", "clerk-workflow-reviewer", map[string]any{"lockVersion": 0, "score": 4, "recommendation": "strong_yes", "internalNotes": "Excellent collaborator"})
	if firstSave.StatusCode != http.StatusOK {
		firstSave.Body.Close()
		t.Fatalf("expected initial review draft save status %d, got %d", http.StatusOK, firstSave.StatusCode)
	}
	decodeIntakeResponse(t, firstSave, &reviewApplication)
	if reviewApplication.Review == nil || reviewApplication.Review.LockVersion != 1 || reviewApplication.Review.Score != 4 || reviewApplication.Review.Recommendation != "strong_yes" {
		t.Fatalf("review draft did not persist structured review: %+v", reviewApplication.Review)
	}
	staleSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/reviewer/applications/"+assignedApplicationID+"/review", "clerk-workflow-reviewer", map[string]any{"lockVersion": 0, "score": 2, "recommendation": "no"})
	assertWorkflowStatus(t, staleSave, http.StatusConflict)

	submission := intakeRequest(t, server.URL, http.MethodPost, "/v1/reviewer/applications/"+assignedApplicationID+"/review/submit", "clerk-workflow-reviewer", map[string]any{"lockVersion": 1})
	if submission.StatusCode != http.StatusOK {
		submission.Body.Close()
		t.Fatalf("expected review submit status %d, got %d", http.StatusOK, submission.StatusCode)
	}
	decodeIntakeResponse(t, submission, &reviewApplication)
	if reviewApplication.Review == nil || reviewApplication.Review.Status != "submitted" || reviewApplication.Review.LockVersion != 2 || reviewApplication.Review.SubmittedAt == nil {
		t.Fatalf("review submission did not lock review: %+v", reviewApplication.Review)
	}
	postSubmitSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/reviewer/applications/"+assignedApplicationID+"/review", "clerk-workflow-reviewer", map[string]any{"lockVersion": 2, "score": 3, "recommendation": "neutral"})
	assertWorkflowStatus(t, postSubmitSave, http.StatusConflict)
	if _, err := pool.Exec(ctx, `UPDATE ats.reviews SET internal_notes = 'tamper' WHERE application_id = $1 AND reviewer_user_id = $2`, assignedApplicationID, reviewerID); err == nil {
		t.Fatal("expected submitted review database mutation to be rejected")
	}
	var reviewSubmissionAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = 'review_submitted' AND actor_user_id = $1`, reviewerID).Scan(&reviewSubmissionAudits); err != nil {
		t.Fatalf("count review submission audit events: %v", err)
	}
	if reviewSubmissionAudits != 1 {
		t.Fatalf("expected one review submission audit event, got %d", reviewSubmissionAudits)
	}

	applicantResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/mine", "clerk-workflow-applicant", nil)
	if applicantResponse.StatusCode != http.StatusOK {
		applicantResponse.Body.Close()
		t.Fatalf("expected applicant application list status %d, got %d", http.StatusOK, applicantResponse.StatusCode)
	}
	var applicantPayload map[string]any
	decodeIntakeResponse(t, applicantResponse, &applicantPayload)
	encodedApplicantPayload, err := json.Marshal(applicantPayload)
	if err != nil {
		t.Fatalf("encode applicant response for privacy assertion: %v", err)
	}
	for _, forbidden := range []string{"review", "recommendation", "internalNotes", "score"} {
		if strings.Contains(string(encodedApplicantPayload), forbidden) {
			t.Fatalf("applicant response leaked internal review field %q: %s", forbidden, encodedApplicantPayload)
		}
	}
}

func insertWorkflowForm(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID, creatorID string) string {
	t.Helper()
	var formID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.application_forms (cycle_id, version, schema_json, published_at, created_by) VALUES ($1, 1, '{"questions":[{"key":"name","label":"Name","type":"string","required":true}]}'::jsonb, CURRENT_TIMESTAMP, $2) RETURNING id::text`, cycleID, creatorID).Scan(&formID); err != nil {
		t.Fatalf("create workflow form: %v", err)
	}
	return formID
}

func insertSubmittedWorkflowApplication(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID, formID, applicantID, name string) string {
	t.Helper()
	applicationID := insertDraftWorkflowApplication(t, ctx, pool, cycleID, formID, applicantID, name)
	if _, err := pool.Exec(ctx, `UPDATE ats.applications SET status = 'submitted', submitted_at = CURRENT_TIMESTAMP WHERE id = $1`, applicationID); err != nil {
		t.Fatalf("submit workflow application fixture: %v", err)
	}
	return applicationID
}

func insertDraftWorkflowApplication(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID, formID, applicantID, name string) string {
	t.Helper()
	var applicationID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id) VALUES ($1, $2, $3) RETURNING id::text`, cycleID, formID, applicantID).Scan(&applicationID); err != nil {
		t.Fatalf("create workflow application draft: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.application_answers (application_id, question_key, value_json) VALUES ($1, 'name', to_jsonb($2::text))`, applicationID, name); err != nil {
		t.Fatalf("create workflow application answer: %v", err)
	}
	return applicationID
}

func assertWorkflowStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expected {
		t.Fatalf("expected status %d, got %d", expected, response.StatusCode)
	}
}

func assertWorkflowAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType, subjectID string, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = $1 AND subject_id = $2`, eventType, subjectID).Scan(&count); err != nil {
		t.Fatalf("count %s audit events: %v", eventType, err)
	}
	if count != expected {
		t.Fatalf("expected %d %s audit events, got %d", expected, eventType, count)
	}
}
