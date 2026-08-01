//go:build integration

package migrations_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
)

type eventActivityResponse struct {
	ID      string `json:"id"`
	CycleID string `json:"cycleId"`
}

type eventCheckpointResponse struct {
	ID string `json:"id"`
}

type eventEntitlementResponse struct {
	Override *struct {
		Allowed        bool `json:"allowed"`
		MaxRedemptions int  `json:"maxRedemptions"`
	} `json:"override"`
}

type eventCountResponse struct {
	Items []struct {
		CheckpointID     string `json:"checkpointId"`
		TotalRedemptions int64  `json:"totalRedemptions"`
	} `json:"items"`
}

func TestEventAdministrationOperationsAreAuditedAndExportsAreMinimal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply event operations migrations: %v", err)
	}

	organizerID := createUser(t, ctx, pool, "clerk-m7-organizer")
	attendeeUserID := createUser(t, ctx, pool, "clerk-m7-attendee")
	scannerID := createUser(t, ctx, pool, "clerk-m7-scanner")
	if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, 'organizer'), ($2, 'scanner')`, organizerID, scannerID); err != nil {
		t.Fatalf("grant event operation roles: %v", err)
	}
	cycleID := insertCycle(t, ctx, pool, "m7-operations", true)
	formID := insertForm(t, ctx, pool, cycleID, 1, organizerID)
	var applicationID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id) VALUES ($1, $2, $3) RETURNING id::text`, cycleID, formID, attendeeUserID).Scan(&applicationID); err != nil {
		t.Fatalf("create event operations application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.application_answers (application_id, question_key, value_json) VALUES ($1, 'private_answer', '"do-not-export"'::jsonb)`, applicationID); err != nil {
		t.Fatalf("insert private application answer fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ats.applications SET status = 'accepted', decision_released_at = CURRENT_TIMESTAMP WHERE id = $1`, applicationID); err != nil {
		t.Fatalf("release accepted attendee fixture: %v", err)
	}
	var attendeeID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.attendees (cycle_id, application_id, user_id, display_name, email) VALUES ($1, $2, $3, 'Ada Lovelace', 'ada.operations@example.test') RETURNING id::text`, cycleID, applicationID, attendeeUserID).Scan(&attendeeID); err != nil {
		t.Fatalf("create event operations attendee: %v", err)
	}

	service := operations.NewService(pool, 5*time.Second, 15*time.Second)
	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool,
		Verifier:  intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-m7-organizer": {ID: organizerID, ClerkUserID: "clerk-m7-organizer", Roles: map[users.Role]struct{}{users.RoleOrganizer: {}}},
			"clerk-m7-attendee":  {ID: attendeeUserID, ClerkUserID: "clerk-m7-attendee", Roles: map[users.Role]struct{}{users.RoleApplicant: {}}},
			"clerk-m7-scanner":   {ID: scannerID, ClerkUserID: "clerk-m7-scanner", Roles: map[users.Role]struct{}{users.RoleScanner: {}}},
		}},
		Operations: service,
	}))
	defer server.Close()

	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/activities", "", nil), http.StatusUnauthorized)
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/activities", "clerk-m7-attendee", nil), http.StatusForbidden)

	activityResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/activities", "clerk-m7-organizer", map[string]any{
		"cycleId": cycleID, "slug": "lunch", "name": "Lunch", "startsAt": "2026-08-01T12:00:00Z", "endsAt": "2026-08-01T13:00:00Z",
	})
	if activityResponse.StatusCode != http.StatusCreated {
		activityResponse.Body.Close()
		t.Fatalf("create activity: got %d", activityResponse.StatusCode)
	}
	var activity eventActivityResponse
	decodeIntakeResponse(t, activityResponse, &activity)
	if activity.ID == "" || activity.CycleID != cycleID {
		t.Fatalf("unexpected created activity: %+v", activity)
	}
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodPatch, "/v1/admin/activities/"+activity.ID, "clerk-m7-organizer", map[string]any{
		"slug": "lunch", "name": "Lunch service", "startsAt": "2026-08-01T12:00:00Z", "endsAt": "2026-08-01T13:00:00Z",
	}), http.StatusOK)

	checkpointResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/checkpoints", "clerk-m7-organizer", map[string]any{
		"cycleId": cycleID, "activityId": activity.ID, "slug": "main-entry", "name": "Main entry", "opensAt": nil, "closesAt": nil,
		"defaultAllowed": true, "defaultMaxRedemptions": 1, "active": true,
	})
	if checkpointResponse.StatusCode != http.StatusCreated {
		checkpointResponse.Body.Close()
		t.Fatalf("create checkpoint: got %d", checkpointResponse.StatusCode)
	}
	var checkpoint eventCheckpointResponse
	decodeIntakeResponse(t, checkpointResponse, &checkpoint)
	if checkpoint.ID == "" {
		t.Fatal("created checkpoint is missing its id")
	}
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodPatch, "/v1/admin/checkpoints/"+checkpoint.ID, "clerk-m7-organizer", map[string]any{
		"activityId": activity.ID, "slug": "main-entry", "name": "Main entry", "opensAt": nil, "closesAt": nil,
		"defaultAllowed": true, "defaultMaxRedemptions": 1, "active": true,
	}), http.StatusOK)

	temporaryActivityResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/activities", "clerk-m7-organizer", map[string]any{
		"cycleId": cycleID, "slug": "temporary-activity", "name": "Temporary activity", "startsAt": nil, "endsAt": nil,
	})
	if temporaryActivityResponse.StatusCode != http.StatusCreated {
		temporaryActivityResponse.Body.Close()
		t.Fatalf("create deletable activity: got %d", temporaryActivityResponse.StatusCode)
	}
	var temporaryActivity eventActivityResponse
	decodeIntakeResponse(t, temporaryActivityResponse, &temporaryActivity)
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/activities/"+temporaryActivity.ID, "clerk-m7-organizer", nil), http.StatusNoContent)

	temporaryCheckpointResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/checkpoints", "clerk-m7-organizer", map[string]any{
		"cycleId": cycleID, "activityId": nil, "slug": "temporary-checkpoint", "name": "Temporary checkpoint", "opensAt": nil, "closesAt": nil,
		"defaultAllowed": false, "defaultMaxRedemptions": 0, "active": false,
	})
	if temporaryCheckpointResponse.StatusCode != http.StatusCreated {
		temporaryCheckpointResponse.Body.Close()
		t.Fatalf("create deletable checkpoint: got %d", temporaryCheckpointResponse.StatusCode)
	}
	var temporaryCheckpoint eventCheckpointResponse
	decodeIntakeResponse(t, temporaryCheckpointResponse, &temporaryCheckpoint)
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/checkpoints/"+temporaryCheckpoint.ID, "clerk-m7-organizer", nil), http.StatusNoContent)

	overrideResponse := intakeRequest(t, server.URL, http.MethodPut, "/v1/admin/attendees/"+attendeeID+"/entitlements/"+checkpoint.ID, "clerk-m7-organizer", map[string]any{"allowed": false, "maxRedemptions": 2})
	if overrideResponse.StatusCode != http.StatusOK {
		overrideResponse.Body.Close()
		t.Fatalf("set entitlement override: got %d", overrideResponse.StatusCode)
	}
	overrideResponse.Body.Close()
	readOverride := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/attendees/"+attendeeID+"/entitlements/"+checkpoint.ID, "clerk-m7-organizer", nil)
	if readOverride.StatusCode != http.StatusOK {
		readOverride.Body.Close()
		t.Fatalf("read entitlement override: got %d", readOverride.StatusCode)
	}
	var entitlement eventEntitlementResponse
	decodeIntakeResponse(t, readOverride, &entitlement)
	if entitlement.Override == nil || entitlement.Override.Allowed || entitlement.Override.MaxRedemptions != 2 {
		t.Fatalf("explicit false override was not preserved: %+v", entitlement)
	}
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/attendees/"+attendeeID+"/entitlements/"+checkpoint.ID, "clerk-m7-organizer", nil), http.StatusNoContent)
	readReset := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/attendees/"+attendeeID+"/entitlements/"+checkpoint.ID, "clerk-m7-organizer", nil)
	if readReset.StatusCode != http.StatusOK {
		readReset.Body.Close()
		t.Fatalf("read reset entitlement: got %d", readReset.StatusCode)
	}
	decodeIntakeResponse(t, readReset, &entitlement)
	if entitlement.Override != nil {
		t.Fatalf("deleted entitlement override remained visible: %+v", entitlement)
	}

	var passID string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash) VALUES ($1, $2, $3) RETURNING id::text`, attendeeID, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)).Scan(&passID); err != nil {
		t.Fatalf("create pass for redemption reporting: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ats.redemptions (pass_id, attendee_id, checkpoint_id, cycle_id, ordinal, scanner_user_id, idempotency_key) VALUES ($1, $2, $3, $4, 1, $5, gen_random_uuid())`, passID, attendeeID, checkpoint.ID, cycleID, scannerID); err != nil {
		t.Fatalf("insert immutable redemption fixture: %v", err)
	}

	countsResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/redemptions/counts", "clerk-m7-organizer", nil)
	if countsResponse.StatusCode != http.StatusOK {
		countsResponse.Body.Close()
		t.Fatalf("load operational redemption counts: got %d", countsResponse.StatusCode)
	}
	var counts eventCountResponse
	decodeIntakeResponse(t, countsResponse, &counts)
	if len(counts.Items) != 1 || counts.Items[0].CheckpointID != checkpoint.ID || counts.Items[0].TotalRedemptions != 1 {
		t.Fatalf("unexpected operational count: %+v", counts)
	}

	reportResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/redemptions?checkpointId="+checkpoint.ID, "clerk-m7-organizer", nil)
	if reportResponse.StatusCode != http.StatusOK {
		reportResponse.Body.Close()
		t.Fatalf("load redemption report: got %d", reportResponse.StatusCode)
	}
	reportPayload, err := io.ReadAll(reportResponse.Body)
	reportResponse.Body.Close()
	if err != nil {
		t.Fatalf("read redemption report: %v", err)
	}
	for _, forbidden := range []string{"ada.operations@example.test", "do-not-export", "internal_reason", "qr_token_hash", "claim_token_hash"} {
		if strings.Contains(string(reportPayload), forbidden) {
			t.Fatalf("redemption report leaked %q: %s", forbidden, reportPayload)
		}
	}
	var report map[string]any
	if err := json.Unmarshal(reportPayload, &report); err != nil {
		t.Fatalf("decode redemption report: %v", err)
	}
	if _, ok := report["items"]; !ok {
		t.Fatalf("redemption report lacks items: %s", reportPayload)
	}

	attendanceResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/exports/attendance.csv?checkpointId="+checkpoint.ID, "clerk-m7-organizer", nil)
	if attendanceResponse.StatusCode != http.StatusOK {
		attendanceResponse.Body.Close()
		t.Fatalf("export attendance: got %d", attendanceResponse.StatusCode)
	}
	attendanceCSV, err := io.ReadAll(attendanceResponse.Body)
	attendanceResponse.Body.Close()
	if err != nil {
		t.Fatalf("read attendance CSV: %v", err)
	}
	if attendanceResponse.Header.Get("Content-Type") != "text/csv; charset=utf-8" || attendanceResponse.Header.Get("Content-Disposition") != `attachment; filename="attendance.csv"` {
		t.Fatalf("unexpected attendance headers: type=%q disposition=%q", attendanceResponse.Header.Get("Content-Type"), attendanceResponse.Header.Get("Content-Disposition"))
	}
	parsedCSV, err := csv.NewReader(strings.NewReader(string(attendanceCSV))).ReadAll()
	if err != nil {
		t.Fatalf("parse attendance CSV: %v", err)
	}
	if len(parsedCSV) != 2 || strings.Join(parsedCSV[0], ",") != "redeemed_at,checkpoint_id,checkpoint_slug,attendee_id,attendee_display_name" || parsedCSV[1][4] != "Ada Lovelace" {
		t.Fatalf("unexpected attendance CSV: %#v", parsedCSV)
	}
	for _, forbidden := range []string{"ada.operations@example.test", "do-not-export", "internal_reason", "qr_token_hash", "claim_token_hash"} {
		if strings.Contains(string(attendanceCSV), forbidden) {
			t.Fatalf("attendance export leaked %q: %s", forbidden, attendanceCSV)
		}
	}

	reconciliationResponse := intakeRequest(t, server.URL, http.MethodGet, "/v1/admin/exports/reconciliation.csv?checkpointId="+checkpoint.ID, "clerk-m7-organizer", nil)
	if reconciliationResponse.StatusCode != http.StatusOK {
		reconciliationResponse.Body.Close()
		t.Fatalf("export reconciliation: got %d", reconciliationResponse.StatusCode)
	}
	reconciliationCSV, err := io.ReadAll(reconciliationResponse.Body)
	reconciliationResponse.Body.Close()
	if err != nil {
		t.Fatalf("read reconciliation CSV: %v", err)
	}
	if reconciliationResponse.Header.Get("Content-Disposition") != `attachment; filename="reconciliation.csv"` {
		t.Fatalf("unexpected reconciliation disposition: %q", reconciliationResponse.Header.Get("Content-Disposition"))
	}
	parsedReconciliation, err := csv.NewReader(strings.NewReader(string(reconciliationCSV))).ReadAll()
	if err != nil {
		t.Fatalf("parse reconciliation CSV: %v", err)
	}
	if len(parsedReconciliation) != 2 || strings.Join(parsedReconciliation[0], ",") != "redemption_id,redeemed_at,checkpoint_id,checkpoint_slug,attendee_id,pass_id,scanner_user_id,ordinal" || strings.Contains(string(reconciliationCSV), "Ada Lovelace") {
		t.Fatalf("unexpected reconciliation CSV: %#v", parsedReconciliation)
	}
	for _, forbidden := range []string{"ada.operations@example.test", "do-not-export", "internal_reason", "qr_token_hash", "claim_token_hash"} {
		if strings.Contains(string(reconciliationCSV), forbidden) {
			t.Fatalf("reconciliation export leaked %q: %s", forbidden, reconciliationCSV)
		}
	}

	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/checkpoints/"+checkpoint.ID, "clerk-m7-organizer", nil), http.StatusConflict)
	assertOperationsStatus(t, intakeRequest(t, server.URL, http.MethodDelete, "/v1/admin/activities/"+activity.ID, "clerk-m7-organizer", nil), http.StatusConflict)

	eventCounts := map[string]int{
		"activity_created":       2,
		"activity_updated":       1,
		"activity_deleted":       1,
		"checkpoint_created":     2,
		"checkpoint_updated":     1,
		"checkpoint_deleted":     1,
		"entitlement_overridden": 1,
		"entitlement_removed":    1,
	}
	for eventType, expected := range eventCounts {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.audit_events WHERE event_type = $1 AND actor_user_id = $2`, eventType, organizerID).Scan(&count); err != nil {
			t.Fatalf("count %s audit events: %v", eventType, err)
		}
		if count != expected {
			t.Fatalf("expected %d %s audit events, got %d", expected, eventType, count)
		}
	}
	exportRows, err := pool.Query(ctx, `SELECT metadata_json ->> 'kind' FROM ats.audit_events WHERE event_type = 'redemption_exported' AND actor_user_id = $1`, organizerID)
	if err != nil {
		t.Fatalf("list redemption export audits: %v", err)
	}
	defer exportRows.Close()
	exportKinds := make(map[string]bool)
	exportCount := 0
	for exportRows.Next() {
		var kind string
		if err := exportRows.Scan(&kind); err != nil {
			t.Fatalf("scan redemption export audit: %v", err)
		}
		exportKinds[kind] = true
		exportCount++
	}
	if err := exportRows.Err(); err != nil {
		t.Fatalf("iterate redemption export audits: %v", err)
	}
	if exportCount != 2 || !exportKinds["attendance"] || !exportKinds["reconciliation"] {
		t.Fatalf("unexpected redemption export audit metadata: count=%d kinds=%+v", exportCount, exportKinds)
	}
}

func assertOperationsStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expected {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected status %d, got %d: %s", expected, response.StatusCode, body)
	}
}
