package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type operationsTestUsers struct {
	user users.User
}

func (resolver operationsTestUsers) Resolve(context.Context, string) (users.User, error) {
	return resolver.user, nil
}

type operationsTestService struct {
	activities            []operations.Activity
	checkpoints           []operations.Checkpoint
	counts                []operations.CheckpointCount
	redemptions           []operations.Redemption
	override              *operations.Entitlement
	err                   error
	checkpointInput       operations.CheckpointInput
	entitlementInput      operations.EntitlementInput
	entitlementAttendee   string
	entitlementCheckpoint string
	exportKind            operations.ExportKind
}

func (service *operationsTestService) ListActivities(context.Context, users.User) ([]operations.Activity, error) {
	return service.activities, service.err
}

func (service *operationsTestService) CreateActivity(_ context.Context, _ users.User, input operations.ActivityInput) (operations.Activity, error) {
	return operations.Activity{ID: "bec68bc5-f3c1-4ee6-a452-b02de18bf41d", CycleID: input.CycleID, Slug: input.Slug, Name: input.Name}, service.err
}

func (service *operationsTestService) UpdateActivity(_ context.Context, _ users.User, _ string, input operations.ActivityInput) (operations.Activity, error) {
	return operations.Activity{ID: "bec68bc5-f3c1-4ee6-a452-b02de18bf41d", Slug: input.Slug, Name: input.Name}, service.err
}

func (service *operationsTestService) DeleteActivity(context.Context, users.User, string) error {
	return service.err
}

func (service *operationsTestService) ListCheckpoints(context.Context, users.User) ([]operations.Checkpoint, error) {
	return service.checkpoints, service.err
}

func (service *operationsTestService) CreateCheckpoint(_ context.Context, _ users.User, input operations.CheckpointInput) (operations.Checkpoint, error) {
	service.checkpointInput = input
	return operations.Checkpoint{ID: "2768bc6d-03c1-4ee6-a452-b02de18bf41d", CycleID: input.CycleID, ActivityID: input.ActivityID, Slug: input.Slug, Name: input.Name, DefaultAllowed: input.DefaultAllowed, DefaultMaxRedemptions: input.DefaultMaxRedemptions, Active: input.Active}, service.err
}

func (service *operationsTestService) UpdateCheckpoint(_ context.Context, _ users.User, _ string, input operations.CheckpointInput) (operations.Checkpoint, error) {
	service.checkpointInput = input
	return operations.Checkpoint{ID: "2768bc6d-03c1-4ee6-a452-b02de18bf41d", Slug: input.Slug, Name: input.Name, DefaultAllowed: input.DefaultAllowed, DefaultMaxRedemptions: input.DefaultMaxRedemptions, Active: input.Active}, service.err
}

func (service *operationsTestService) DeleteCheckpoint(context.Context, users.User, string) error {
	return service.err
}

func (service *operationsTestService) GetEntitlement(context.Context, users.User, string, string) (*operations.Entitlement, error) {
	return service.override, service.err
}

func (service *operationsTestService) PutEntitlement(_ context.Context, _ users.User, attendeeID, checkpointID string, input operations.EntitlementInput) (operations.Entitlement, error) {
	service.entitlementInput = input
	service.entitlementAttendee = attendeeID
	service.entitlementCheckpoint = checkpointID
	return operations.Entitlement{AttendeeID: attendeeID, CheckpointID: checkpointID, Allowed: input.Allowed, MaxRedemptions: input.MaxRedemptions}, service.err
}

func (service *operationsTestService) DeleteEntitlement(context.Context, users.User, string, string) error {
	return service.err
}

func (service *operationsTestService) ListCheckpointCounts(context.Context, users.User) ([]operations.CheckpointCount, error) {
	return service.counts, service.err
}

func (service *operationsTestService) ListRedemptions(context.Context, users.User, *string, int) ([]operations.Redemption, error) {
	return service.redemptions, service.err
}

func (service *operationsTestService) ExportRedemptions(_ context.Context, _ users.User, kind operations.ExportKind, _ *string) ([]operations.Redemption, error) {
	service.exportKind = kind
	return service.redemptions, service.err
}

func organizerTestUser() users.User {
	return users.User{ID: "9d18f13d-9f79-40a2-831b-c4350f806555", Roles: map[users.Role]struct{}{users.RoleOrganizer: {}}}
}

func TestOrganizerOperationsRoutesAuthorizeValidateAndExportMinimalCSV(t *testing.T) {
	when := time.Date(2026, time.July, 31, 14, 0, 0, 0, time.UTC)
	service := &operationsTestService{
		activities: []operations.Activity{{ID: "bec68bc5-f3c1-4ee6-a452-b02de18bf41d", CycleID: "1e9f1f04-6d37-44f3-8765-0b3492851e90", Slug: "lunch", Name: "Lunch"}},
		redemptions: []operations.Redemption{{
			ID: "a71a9df0-c7e0-4af3-a0fe-9fa7c0e33a7d", RedeemedAt: when, Ordinal: 1, ScannerUserID: "0bfac5c6-7de5-4a79-82ae-7dc14a5f0418",
			Checkpoint: operations.RedemptionCheckpoint{ID: "2768bc6d-03c1-4ee6-a452-b02de18bf41d", Slug: "main-entry", Name: "Main entry"},
			Attendee:   operations.RedemptionAttendee{ID: "a5d274fd-6fd2-43f5-9fe9-5b04a11ae6d4", DisplayName: "Ada, Lovelace"},
			Pass:       operations.RedemptionPass{ID: "1171bc7c-8f14-4159-a834-26f20e8ad0d8", Status: "active"},
		}},
	}
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: operationsTestUsers{user: organizerTestUser()}, Operations: service,
		AllowedOrigins: []string{"https://app.example"},
	})

	unauthorized := httptest.NewRequest(http.MethodGet, "/v1/admin/activities", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated organizer activities: got %d", unauthorizedResponse.Code)
	}

	nonOrganizerHandler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: fakeUsers{}, Operations: service,
	})
	nonOrganizer := httptest.NewRequest(http.MethodGet, "/v1/admin/checkpoints", nil)
	nonOrganizer.Header.Set("Authorization", "Bearer applicant-session")
	nonOrganizerResponse := httptest.NewRecorder()
	nonOrganizerHandler.ServeHTTP(nonOrganizerResponse, nonOrganizer)
	if nonOrganizerResponse.Code != http.StatusForbidden {
		t.Fatalf("non-organizer checkpoints: got %d", nonOrganizerResponse.Code)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/v1/admin/checkpoints", strings.NewReader(`{"cycleId":"1e9f1f04-6d37-44f3-8765-0b3492851e90","unknown":true}`))
	invalid.Header.Set("Authorization", "Bearer organizer-session")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("unknown checkpoint field: got %d", invalidResponse.Code)
	}

	checkpointID := "2768bc6d-03c1-4ee6-a452-b02de18bf41d"
	created := httptest.NewRequest(http.MethodPost, "/v1/admin/checkpoints", strings.NewReader(`{"cycleId":"1e9f1f04-6d37-44f3-8765-0b3492851e90","activityId":null,"slug":"main-entry","name":"Main entry","opensAt":null,"closesAt":null,"defaultAllowed":true,"defaultMaxRedemptions":1,"active":true}`))
	created.Header.Set("Authorization", "Bearer organizer-session")
	createdResponse := httptest.NewRecorder()
	handler.ServeHTTP(createdResponse, created)
	if createdResponse.Code != http.StatusCreated || service.checkpointInput.Slug != "main-entry" || service.checkpointInput.ActivityID != nil || !service.checkpointInput.DefaultAllowed || service.checkpointInput.DefaultMaxRedemptions != 1 {
		t.Fatalf("checkpoint create did not preserve configuration: status=%d input=%+v", createdResponse.Code, service.checkpointInput)
	}

	omittedNullablePatch := httptest.NewRequest(http.MethodPatch, "/v1/admin/checkpoints/"+checkpointID, strings.NewReader(`{"activityId":null,"slug":"main-entry","name":"Main entry","defaultAllowed":true,"defaultMaxRedemptions":1,"active":true}`))
	omittedNullablePatch.Header.Set("Authorization", "Bearer organizer-session")
	omittedNullablePatchResponse := httptest.NewRecorder()
	handler.ServeHTTP(omittedNullablePatchResponse, omittedNullablePatch)
	if omittedNullablePatchResponse.Code != http.StatusBadRequest {
		t.Fatalf("partial checkpoint replacement that omits nullable fields: got %d", omittedNullablePatchResponse.Code)
	}

	attendeeID := "a5d274fd-6fd2-43f5-9fe9-5b04a11ae6d4"
	override := httptest.NewRequest(http.MethodPut, "/v1/admin/attendees/"+attendeeID+"/entitlements/"+checkpointID, strings.NewReader(`{"allowed":false,"maxRedemptions":0}`))
	override.Header.Set("Authorization", "Bearer organizer-session")
	overrideResponse := httptest.NewRecorder()
	handler.ServeHTTP(overrideResponse, override)
	if overrideResponse.Code != http.StatusOK || service.entitlementInput.Allowed || service.entitlementInput.MaxRedemptions != 0 || service.entitlementAttendee != attendeeID || service.entitlementCheckpoint != checkpointID {
		t.Fatalf("entitlement override did not preserve explicit denial: status=%d input=%+v", overrideResponse.Code, service.entitlementInput)
	}

	csvRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/exports/attendance.csv", nil)
	csvRequest.Header.Set("Authorization", "Bearer organizer-session")
	csvResponse := httptest.NewRecorder()
	handler.ServeHTTP(csvResponse, csvRequest)
	if csvResponse.Code != http.StatusOK || service.exportKind != operations.ExportAttendance {
		t.Fatalf("attendance export: status=%d kind=%q", csvResponse.Code, service.exportKind)
	}
	if got := csvResponse.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("attendance content type: %q", got)
	}
	if got := csvResponse.Header().Get("Content-Disposition"); got != `attachment; filename="attendance.csv"` {
		t.Fatalf("attendance disposition: %q", got)
	}
	csvPayload := csvResponse.Body.String()
	csvRows, err := csv.NewReader(strings.NewReader(csvPayload)).ReadAll()
	if err != nil {
		t.Fatalf("parse attendance CSV: %v", err)
	}
	if len(csvRows) != 2 || strings.Join(csvRows[0], ",") != "redeemed_at,checkpoint_id,checkpoint_slug,attendee_id,attendee_display_name" || csvRows[1][4] != "Ada, Lovelace" {
		t.Fatalf("unexpected attendance CSV: %#v", csvRows)
	}
	for _, forbidden := range []string{"applicant@example.test", "internal reason", "qr_token_hash", "claim_token_hash"} {
		if strings.Contains(csvPayload, forbidden) {
			t.Fatalf("attendance CSV leaked %q: %s", forbidden, csvPayload)
		}
	}

	reconciliationRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/exports/reconciliation.csv", nil)
	reconciliationRequest.Header.Set("Authorization", "Bearer organizer-session")
	reconciliationResponse := httptest.NewRecorder()
	handler.ServeHTTP(reconciliationResponse, reconciliationRequest)
	if reconciliationResponse.Code != http.StatusOK || service.exportKind != operations.ExportReconciliation {
		t.Fatalf("reconciliation export: status=%d kind=%q", reconciliationResponse.Code, service.exportKind)
	}
	if got := reconciliationResponse.Header().Get("Content-Disposition"); got != `attachment; filename="reconciliation.csv"` {
		t.Fatalf("reconciliation disposition: %q", got)
	}
	reconciliationPayload := reconciliationResponse.Body.String()
	reconciliationRows, err := csv.NewReader(strings.NewReader(reconciliationPayload)).ReadAll()
	if err != nil {
		t.Fatalf("parse reconciliation CSV: %v", err)
	}
	if len(reconciliationRows) != 2 || strings.Join(reconciliationRows[0], ",") != "redemption_id,redeemed_at,checkpoint_id,checkpoint_slug,attendee_id,pass_id,scanner_user_id,ordinal" || strings.Contains(reconciliationPayload, "Ada, Lovelace") {
		t.Fatalf("unexpected reconciliation CSV: %#v", reconciliationRows)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/v1/admin/checkpoints/"+checkpointID, nil)
	preflight.Header.Set("Origin", "https://app.example")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || !strings.Contains(preflightResponse.Header().Get("Access-Control-Allow-Methods"), "PATCH") || !strings.Contains(preflightResponse.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatalf("operations preflight did not allow PATCH and DELETE: status=%d allow=%q", preflightResponse.Code, preflightResponse.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestOrganizerOperationsMapsDeletionConflict(t *testing.T) {
	service := &operationsTestService{err: operations.ErrInUse}
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: operationsTestUsers{user: organizerTestUser()}, Operations: service,
	})
	request := httptest.NewRequest(http.MethodDelete, "/v1/admin/activities/bec68bc5-f3c1-4ee6-a452-b02de18bf41d", nil)
	request.Header.Set("Authorization", "Bearer organizer-session")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("in-use activity deletion: got %d", response.Code)
	}
	var payload errorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode in-use activity error: %v", err)
	}
	if payload.Code != "operations_in_use" || !errors.Is(service.err, operations.ErrInUse) {
		t.Fatalf("unexpected in-use activity error: %+v", payload)
	}
}

func TestRedemptionCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	rows := []operations.Redemption{{
		RedeemedAt: time.Date(2026, time.July, 31, 14, 0, 0, 0, time.UTC),
		Checkpoint: operations.RedemptionCheckpoint{
			ID:   "2768bc6d-03c1-4ee6-a452-b02de18bf41d",
			Slug: " =HYPERLINK(\"https://example.test\")",
		},
		Attendee: operations.RedemptionAttendee{
			ID:          "a5d274fd-6fd2-43f5-9fe9-5b04a11ae6d4",
			DisplayName: "+cmd|' /C calc'!A0",
		},
	}}
	payload, err := redemptionCSV(operations.ExportAttendance, rows)
	if err != nil {
		t.Fatalf("build attendance CSV: %v", err)
	}
	parsed, err := csv.NewReader(strings.NewReader(string(payload))).ReadAll()
	if err != nil {
		t.Fatalf("parse attendance CSV: %v", err)
	}
	if got := parsed[1][2]; got != "' =HYPERLINK(\"https://example.test\")" {
		t.Fatalf("checkpoint formula was not neutralized: %q", got)
	}
	if got := parsed[1][4]; got != "'+cmd|' /C calc'!A0" {
		t.Fatalf("display-name formula was not neutralized: %q", got)
	}
}
