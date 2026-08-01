//go:build integration

package migrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type intakeVerifier struct{}

func (intakeVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token == "" {
		return auth.Principal{}, errors.New("missing test identity")
	}
	return auth.Principal{ClerkUserID: token}, nil
}

type intakeUserResolver struct {
	users map[string]users.User
}

func (resolver intakeUserResolver) Resolve(_ context.Context, clerkUserID string) (users.User, error) {
	user, ok := resolver.users[clerkUserID]
	if !ok {
		return users.User{}, fmt.Errorf("unknown test identity %q", clerkUserID)
	}
	return user, nil
}

type intakeFormResponse struct {
	ID        string `json:"id"`
	CycleID   string `json:"cycleId"`
	Questions []struct {
		Key string `json:"key"`
	} `json:"questions"`
}

type intakeApplicationResponse struct {
	ID          string                     `json:"id"`
	CycleID     string                     `json:"cycleId"`
	Status      string                     `json:"status"`
	SubmittedAt *time.Time                 `json:"submittedAt"`
	LockVersion int32                      `json:"lockVersion"`
	Answers     map[string]json.RawMessage `json:"answers"`
}

type intakeListResponse struct {
	Items []intakeApplicationResponse `json:"items"`
}

type intakeErrorResponse struct {
	Code string `json:"code"`
}

func TestApplicantIntakeJourney(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	applicantID := createUser(t, ctx, pool, "clerk-intake-applicant")
	otherApplicantID := createUser(t, ctx, pool, "clerk-intake-other")
	creatorID := createUser(t, ctx, pool, "clerk-intake-creator")
	cycleID := insertCycle(t, ctx, pool, "intake-current", true)
	formID := insertIntakeForm(t, ctx, pool, cycleID, creatorID)

	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool,
		Verifier:  intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-intake-applicant": {
				ID:          applicantID,
				ClerkUserID: "clerk-intake-applicant",
				Email:       "applicant@example.test",
				Roles:       map[users.Role]struct{}{users.RoleApplicant: {}},
			},
			"clerk-intake-other": {
				ID:          otherApplicantID,
				ClerkUserID: "clerk-intake-other",
				Email:       "other@example.test",
				Roles:       map[users.Role]struct{}{users.RoleApplicant: {}},
			},
		},
		},
		Applications: applications.NewService(pool, 5*time.Second, 15*time.Second),
	}))
	defer server.Close()

	unauthenticated := intakeRequest(t, server.URL, http.MethodGet, "/v1/application-forms/current", "", nil)
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated form lookup to return %d, got %d", http.StatusUnauthorized, unauthenticated.StatusCode)
	}
	unauthenticated.Body.Close()

	formResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/application-forms/current", "clerk-intake-applicant", nil)
	if formResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected current form status %d, got %d", http.StatusOK, formResponse.StatusCode)
	}
	var form intakeFormResponse
	decodeIntakeResponse(t, formResponse, &form)
	if form.ID != formID || form.CycleID != cycleID || len(form.Questions) != 3 {
		t.Fatalf("unexpected current form: %+v", form)
	}

	firstCreate := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications", "clerk-intake-applicant", nil)
	if firstCreate.StatusCode != http.StatusOK {
		t.Fatalf("expected draft creation status %d, got %d", http.StatusOK, firstCreate.StatusCode)
	}
	var application intakeApplicationResponse
	decodeIntakeResponse(t, firstCreate, &application)
	if application.Status != "draft" || application.LockVersion != 0 || application.CycleID != cycleID {
		t.Fatalf("unexpected new draft: %+v", application)
	}

	secondCreate := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications", "clerk-intake-applicant", nil)
	if secondCreate.StatusCode != http.StatusOK {
		t.Fatalf("expected idempotent draft creation status %d, got %d", http.StatusOK, secondCreate.StatusCode)
	}
	var repeatedApplication intakeApplicationResponse
	decodeIntakeResponse(t, secondCreate, &repeatedApplication)
	if repeatedApplication.ID != application.ID || repeatedApplication.LockVersion != application.LockVersion {
		t.Fatalf("draft creation was not idempotent: first=%+v repeated=%+v", application, repeatedApplication)
	}

	saveNameOnly := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+application.ID+"/draft", "clerk-intake-applicant", map[string]any{
		"lockVersion": application.LockVersion,
		"answers":     map[string]any{"name": "Ada Lovelace"},
	})
	if saveNameOnly.StatusCode != http.StatusOK {
		t.Fatalf("expected draft save status %d, got %d", http.StatusOK, saveNameOnly.StatusCode)
	}
	decodeIntakeResponse(t, saveNameOnly, &application)
	if application.LockVersion != 1 || string(application.Answers["name"]) != `"Ada Lovelace"` {
		t.Fatalf("draft save did not persist expected answer: %+v", application)
	}

	reloaded := intakeRequest(t, server.URL, http.MethodGet, "/v1/applications/mine", "clerk-intake-applicant", nil)
	if reloaded.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard reload status %d, got %d", http.StatusOK, reloaded.StatusCode)
	}
	var dashboard intakeListResponse
	decodeIntakeResponse(t, reloaded, &dashboard)
	if len(dashboard.Items) != 1 || dashboard.Items[0].ID != application.ID || string(dashboard.Items[0].Answers["name"]) != `"Ada Lovelace"` {
		t.Fatalf("dashboard did not reload persisted draft: %+v", dashboard)
	}

	crossUserSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+application.ID+"/draft", "clerk-intake-other", map[string]any{
		"lockVersion": application.LockVersion,
		"answers":     map[string]any{"name": "Not Allowed"},
	})
	if crossUserSave.StatusCode != http.StatusNotFound {
		t.Fatalf("expected cross-user save status %d, got %d", http.StatusNotFound, crossUserSave.StatusCode)
	}
	crossUserSave.Body.Close()

	invalidDraft := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+application.ID+"/draft", "clerk-intake-applicant", map[string]any{
		"lockVersion": application.LockVersion,
		"answers":     map[string]any{"unknownQuestion": "Rejected"},
	})
	if invalidDraft.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid draft status %d, got %d", http.StatusUnprocessableEntity, invalidDraft.StatusCode)
	}
	invalidDraft.Body.Close()

	incompleteSubmit := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications/"+application.ID+"/submit", "clerk-intake-applicant", map[string]any{"lockVersion": application.LockVersion})
	if incompleteSubmit.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected incomplete submit status %d, got %d", http.StatusUnprocessableEntity, incompleteSubmit.StatusCode)
	}
	incompleteSubmit.Body.Close()

	completeSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+application.ID+"/draft", "clerk-intake-applicant", map[string]any{
		"lockVersion": application.LockVersion,
		"answers": map[string]any{
			"name":       "Ada Lovelace",
			"experience": 3,
			"attending":  true,
		},
	})
	if completeSave.StatusCode != http.StatusOK {
		t.Fatalf("expected complete draft save status %d, got %d", http.StatusOK, completeSave.StatusCode)
	}
	decodeIntakeResponse(t, completeSave, &application)

	submission := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications/"+application.ID+"/submit", "clerk-intake-applicant", map[string]any{"lockVersion": application.LockVersion})
	if submission.StatusCode != http.StatusOK {
		t.Fatalf("expected submit status %d, got %d", http.StatusOK, submission.StatusCode)
	}
	decodeIntakeResponse(t, submission, &application)
	if application.Status != "submitted" || application.SubmittedAt == nil {
		t.Fatalf("submission did not return status and timestamp: %+v", application)
	}

	retriedSubmission := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications/"+application.ID+"/submit", "clerk-intake-applicant", map[string]any{"lockVersion": application.LockVersion - 1})
	if retriedSubmission.StatusCode != http.StatusOK {
		t.Fatalf("expected idempotent submit status %d, got %d", http.StatusOK, retriedSubmission.StatusCode)
	}
	var retried intakeApplicationResponse
	decodeIntakeResponse(t, retriedSubmission, &retried)
	if retried.ID != application.ID || retried.Status != "submitted" || retried.SubmittedAt == nil {
		t.Fatalf("submission retry did not return submitted application: %+v", retried)
	}

	immutableSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+application.ID+"/draft", "clerk-intake-applicant", map[string]any{
		"lockVersion": application.LockVersion,
		"answers":     map[string]any{"name": "Grace Hopper"},
	})
	if immutableSave.StatusCode != http.StatusConflict {
		t.Fatalf("expected post-submission API save status %d, got %d", http.StatusConflict, immutableSave.StatusCode)
	}
	immutableSave.Body.Close()
	if _, err := pool.Exec(ctx, `UPDATE ats.application_answers SET value_json = '"Grace Hopper"'::jsonb WHERE application_id = $1 AND question_key = 'name'`, application.ID); err == nil {
		t.Fatal("expected database trigger to reject post-submission answer modification")
	}
	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.email_outbox WHERE event_type = 'submission_confirmation' AND recipient_user_id = $1`, applicantID).Scan(&outboxCount); err != nil {
		t.Fatalf("count confirmation outbox records: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one transactional submission confirmation, got %d", outboxCount)
	}

	closedDraftCreate := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications", "clerk-intake-other", nil)
	if closedDraftCreate.StatusCode != http.StatusOK {
		t.Fatalf("expected closed-window draft creation status %d, got %d", http.StatusOK, closedDraftCreate.StatusCode)
	}
	var closedDraft intakeApplicationResponse
	decodeIntakeResponse(t, closedDraftCreate, &closedDraft)

	closedDraftSave := intakeRequest(t, server.URL, http.MethodPut, "/v1/applications/"+closedDraft.ID+"/draft", "clerk-intake-other", map[string]any{
		"lockVersion": closedDraft.LockVersion,
		"answers": map[string]any{
			"name":       "Katherine Johnson",
			"experience": 2,
			"attending":  true,
		},
	})
	if closedDraftSave.StatusCode != http.StatusOK {
		t.Fatalf("expected closed-window draft save status %d, got %d", http.StatusOK, closedDraftSave.StatusCode)
	}
	decodeIntakeResponse(t, closedDraftSave, &closedDraft)
	if closedDraft.Status != "draft" || closedDraft.LockVersion != 1 {
		t.Fatalf("expected a fully saved closed-window draft, got %+v", closedDraft)
	}

	if _, err := pool.Exec(ctx, `UPDATE ats.application_cycles SET applications_open_at = CURRENT_TIMESTAMP - INTERVAL '2 days', applications_close_at = CURRENT_TIMESTAMP - INTERVAL '1 second' WHERE id = $1`, cycleID); err != nil {
		t.Fatalf("close application cycle before submission: %v", err)
	}
	closedSubmission := intakeRequest(t, server.URL, http.MethodPost, "/v1/applications/"+closedDraft.ID+"/submit", "clerk-intake-other", map[string]any{"lockVersion": closedDraft.LockVersion})
	if closedSubmission.StatusCode != http.StatusConflict {
		t.Fatalf("expected closed-window submit status %d, got %d", http.StatusConflict, closedSubmission.StatusCode)
	}
	var closedSubmissionError intakeErrorResponse
	decodeIntakeResponse(t, closedSubmission, &closedSubmissionError)
	if closedSubmissionError.Code != "application_window_closed" {
		t.Fatalf("expected closed-window error code, got %+v", closedSubmissionError)
	}

	var closedDraftStatus string
	var closedDraftSubmittedAtIsNull bool
	if err := pool.QueryRow(ctx, `SELECT status, submitted_at IS NULL FROM ats.applications WHERE id = $1`, closedDraft.ID).Scan(&closedDraftStatus, &closedDraftSubmittedAtIsNull); err != nil {
		t.Fatalf("load closed-window draft state: %v", err)
	}
	if closedDraftStatus != "draft" || !closedDraftSubmittedAtIsNull {
		t.Fatalf("closed-window submit changed draft state: status=%q submitted_at_is_null=%t", closedDraftStatus, closedDraftSubmittedAtIsNull)
	}
	var closedDraftOutboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.email_outbox WHERE event_type = 'submission_confirmation' AND recipient_user_id = $1`, otherApplicantID).Scan(&closedDraftOutboxCount); err != nil {
		t.Fatalf("count closed-window confirmation outbox records: %v", err)
	}
	if closedDraftOutboxCount != 0 {
		t.Fatalf("expected no closed-window submission confirmation, got %d", closedDraftOutboxCount)
	}
}

func insertIntakeForm(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID, creatorID string) string {
	t.Helper()
	const schema = `{"questions":[{"key":"name","label":"Full name","type":"string","required":true},{"key":"experience","label":"Prior hackathons","type":"number","required":true},{"key":"attending","label":"Can you attend?","type":"boolean","required":true}]}`
	var formID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.application_forms (cycle_id, version, schema_json, published_at, created_by) VALUES ($1, 1, $2::jsonb, CURRENT_TIMESTAMP, $3) RETURNING id::text`, cycleID, schema, creatorID).Scan(&formID); err != nil {
		t.Fatalf("create published intake form: %v", err)
	}
	return formID
}

func intakeRequest(t *testing.T, baseURL, method, path, clerkUserID string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if clerkUserID != "" {
		request.Header.Set("Authorization", "Bearer "+clerkUserID)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	return response
}

func decodeIntakeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
