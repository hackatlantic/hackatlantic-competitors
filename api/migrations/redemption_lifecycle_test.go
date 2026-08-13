//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/checkpoints"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/redemptions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type redemptionOutcomeResponse struct {
	Outcome  string `json:"outcome"`
	Attendee *struct {
		DisplayName string `json:"displayName"`
	} `json:"attendee"`
	Pass *struct {
		Status string `json:"status"`
	} `json:"pass"`
	Checkpoint struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"checkpoint"`
	RedemptionID *string `json:"redemptionId"`
}

func TestScannerRedemptionsAreAtomicIdempotentAndMinimal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	pool, cleanup := disposableDatabase(t, ctx)
	defer cleanup()
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("apply M6 migrations: %v", err)
	}

	organizerID := createUser(t, ctx, pool, "clerk-redemption-organizer")
	scannerID := createUser(t, ctx, pool, "clerk-redemption-scanner")
	applicantID := createUser(t, ctx, pool, "clerk-redemption-applicant")
	revokedApplicantID := createUser(t, ctx, pool, "clerk-redemption-revoked")
	concurrentApplicantID := createUser(t, ctx, pool, "clerk-redemption-concurrent")
	for _, assignment := range []struct {
		userID string
		role   string
	}{{organizerID, "organizer"}, {scannerID, "scanner"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO ats.user_roles (user_id, role) VALUES ($1, $2)`, assignment.userID, assignment.role); err != nil {
			t.Fatalf("grant fixture %s: %v", assignment.role, err)
		}
	}

	cycleID := insertCycle(t, ctx, pool, "redemption-lifecycle", true)
	formID := insertWorkflowForm(t, ctx, pool, cycleID, organizerID)
	applicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, applicantID, "Scanner-safe Ada")
	revokedApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, revokedApplicantID, "Revoked Bea")
	concurrentApplicationID := insertSubmittedWorkflowApplication(t, ctx, pool, cycleID, formID, concurrentApplicantID, "Concurrent Cy")

	passService, err := passes.NewService(pool, 5*time.Second, 15*time.Second, passes.Config{
		QRTokenPepper: encodedTestPepper(0x41), ClaimTokenPepper: encodedTestPepper(0x42), AppBaseURL: "https://app.example",
	})
	if err != nil {
		t.Fatalf("create pass service: %v", err)
	}
	checkpointService := checkpoints.NewService(pool, 5*time.Second)
	redemptionService := redemptions.NewService(pool, 15*time.Second, passService)
	organizerUser := decisionTestUser(organizerID, "clerk-redemption-organizer", users.RoleOrganizer)
	server := httptest.NewServer(httpapi.NewHandlerWithDependencies("test", httpapi.Dependencies{
		Readiness: pool, Verifier: intakeVerifier{},
		Users: intakeUserResolver{users: map[string]users.User{
			"clerk-redemption-organizer":  organizerUser,
			"clerk-redemption-scanner":    decisionTestUser(scannerID, "clerk-redemption-scanner", users.RoleScanner),
			"clerk-redemption-applicant":  decisionTestUser(applicantID, "clerk-redemption-applicant", users.RoleApplicant),
			"clerk-redemption-revoked":    decisionTestUser(revokedApplicantID, "clerk-redemption-revoked", users.RoleApplicant),
			"clerk-redemption-concurrent": decisionTestUser(concurrentApplicantID, "clerk-redemption-concurrent", users.RoleApplicant),
		}},
		Applications: applications.NewService(pool, 5*time.Second, 15*time.Second),
		Reviews:      reviews.NewService(pool, 5*time.Second, 15*time.Second),
		Decisions:    decisions.NewService(pool, 5*time.Second, 15*time.Second),
		Passes:       passService, Checkpoints: checkpointService, Redemptions: redemptionService,
	}))
	defer server.Close()

	mainDecision := recordLifecycleDecision(t, server.URL, applicationID, "clerk-redemption-organizer", "accepted", "accepted for scanning")
	releaseLifecycleDecision(t, server.URL, mainDecision.ID, "clerk-redemption-organizer")
	mainAttendeeID := attendeeForApplication(t, ctx, pool, applicationID)
	mainPass := issueRedemptionPass(t, server.URL, mainAttendeeID)

	revokedDecision := recordLifecycleDecision(t, server.URL, revokedApplicationID, "clerk-redemption-organizer", "accepted", "accepted then revoked")
	releaseLifecycleDecision(t, server.URL, revokedDecision.ID, "clerk-redemption-organizer")
	revokedPass := issueRedemptionPass(t, server.URL, attendeeForApplication(t, ctx, pool, revokedApplicationID))
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/admin/passes/"+revokedPass.ID+"/revoke", "clerk-redemption-organizer", nil), http.StatusOK)

	concurrentDecision := recordLifecycleDecision(t, server.URL, concurrentApplicationID, "clerk-redemption-organizer", "accepted", "accepted for atomic scan")
	releaseLifecycleDecision(t, server.URL, concurrentDecision.ID, "clerk-redemption-organizer")
	concurrentPass := issueRedemptionPass(t, server.URL, attendeeForApplication(t, ctx, pool, concurrentApplicationID))

	defaultCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "entry", "Main entrance", true, 2, true, nil, nil)
	overrideAllowedID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "mentor", "Mentor dinner", false, 0, true, nil, nil)
	overrideDeniedID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "vip", "VIP room", true, 3, true, nil, nil)
	futureOpen := time.Now().UTC().Add(time.Hour)
	futureCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "future", "Future workshop", true, 1, true, &futureOpen, nil)
	alreadyClosed := time.Now().UTC().Add(-time.Second)
	closedCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "closed", "Closed workshop", true, 1, true, nil, &alreadyClosed)
	inactiveCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "inactive", "Inactive checkpoint", true, 1, false, nil, nil)
	concurrentCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "concurrent", "Concurrent entrance", true, 1, true, nil, nil)
	parallelCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "parallel", "Parallel entrance", true, 1, true, nil, nil)
	policyCheckpointID := insertRedemptionCheckpoint(t, ctx, pool, cycleID, "policy", "Policy entrance", true, 1, true, nil, nil)
	insertRedemptionEntitlement(t, ctx, pool, mainAttendeeID, overrideAllowedID, cycleID, true, 1)
	insertRedemptionEntitlement(t, ctx, pool, mainAttendeeID, overrideDeniedID, cycleID, false, 99)

	// A scanner role is global: no checkpoint-scoped assignment is created or consulted.
	checkpointList := intakeRequest(t, server.URL, http.MethodGet, "/v1/checkpoints", "clerk-redemption-scanner", nil)
	if checkpointList.StatusCode != http.StatusOK {
		checkpointList.Body.Close()
		t.Fatalf("list active checkpoints: got %d", checkpointList.StatusCode)
	}
	var listPayload struct {
		Items      []struct{ ID, Name string } `json:"items"`
		NextCursor *string                     `json:"nextCursor"`
	}
	decodeIntakeResponse(t, checkpointList, &listPayload)
	if len(listPayload.Items) != 8 || listPayload.NextCursor != nil {
		t.Fatalf("scanner did not receive every active checkpoint globally: %+v", listPayload)
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodGet, "/v1/checkpoints", "clerk-redemption-applicant", nil), http.StatusForbidden)

	lookupResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/scans/lookup", "clerk-redemption-scanner", map[string]any{"qrToken": mainPass.QRToken})
	if lookupResponse.StatusCode != http.StatusOK {
		lookupResponse.Body.Close()
		t.Fatalf("lookup active QR: got %d", lookupResponse.StatusCode)
	}
	var lookupPayload map[string]any
	decodeIntakeResponse(t, lookupResponse, &lookupPayload)
	assertScannerSafePayload(t, lookupPayload)
	if lookupPayload["attendee"].(map[string]any)["displayName"] != "clerk-redemption-applicant@example.test" || lookupPayload["pass"].(map[string]any)["status"] != "active" {
		t.Fatalf("unexpected minimal QR lookup payload: %+v", lookupPayload)
	}
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/scans/lookup", "clerk-redemption-scanner", map[string]any{"qrToken": mainPass.ClaimToken}), http.StatusNotFound)
	revokedLookupResponse := intakeRequest(t, server.URL, http.MethodPost, "/v1/scans/lookup", "clerk-redemption-scanner", map[string]any{"qrToken": revokedPass.QRToken})
	if revokedLookupResponse.StatusCode != http.StatusOK {
		revokedLookupResponse.Body.Close()
		t.Fatalf("lookup revoked QR: got %d", revokedLookupResponse.StatusCode)
	}
	var revokedLookupPayload map[string]any
	decodeIntakeResponse(t, revokedLookupResponse, &revokedLookupPayload)
	assertScannerSafePayload(t, revokedLookupPayload)
	if revokedLookupPayload["pass"].(map[string]any)["status"] != "revoked" {
		t.Fatalf("revoked QR lookup did not expose minimal lifecycle state: %+v", revokedLookupPayload)
	}

	firstKey := redemptionKey(1)
	first := redeemLifecyclePass(t, server.URL, mainPass.QRToken, defaultCheckpointID, firstKey)
	assertRedemptionOutcome(t, first, redemptions.OutcomeRedeemed, "Main entrance", true)
	repeated := redeemLifecyclePass(t, server.URL, mainPass.QRToken, defaultCheckpointID, firstKey)
	if repeated.Outcome != redemptions.OutcomeRedeemed || repeated.RedemptionID == nil || first.RedemptionID == nil || *repeated.RedemptionID != *first.RedemptionID {
		t.Fatalf("same redemption idempotency key did not replay original authoritative result: first=%+v repeated=%+v", first, repeated)
	}
	second := redeemLifecyclePass(t, server.URL, mainPass.QRToken, defaultCheckpointID, redemptionKey(2))
	assertRedemptionOutcome(t, second, redemptions.OutcomeRedeemed, "Main entrance", true)
	exhausted := redeemLifecyclePass(t, server.URL, mainPass.QRToken, defaultCheckpointID, redemptionKey(3))
	assertRedemptionOutcome(t, exhausted, redemptions.OutcomeAlreadyExhausted, "Main entrance", false)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/redemptions", "clerk-redemption-scanner", map[string]any{"qrToken": mainPass.QRToken, "checkpointId": overrideAllowedID, "idempotencyKey": firstKey}), http.StatusConflict)
	assertPassStatus(t, intakeRequest(t, server.URL, http.MethodPost, "/v1/redemptions", "clerk-redemption-scanner", map[string]any{"qrToken": concurrentPass.QRToken, "checkpointId": defaultCheckpointID, "idempotencyKey": firstKey}), http.StatusConflict)

	allowedOverride := redeemLifecyclePass(t, server.URL, mainPass.QRToken, overrideAllowedID, redemptionKey(4))
	assertRedemptionOutcome(t, allowedOverride, redemptions.OutcomeRedeemed, "Mentor dinner", true)
	assertScannerSafeRedemptionPayload(t, allowedOverride)
	notEntitled := redeemLifecyclePass(t, server.URL, mainPass.QRToken, overrideDeniedID, redemptionKey(5))
	assertRedemptionOutcome(t, notEntitled, redemptions.OutcomeNotEntitled, "VIP room", false)
	future := redeemLifecyclePass(t, server.URL, mainPass.QRToken, futureCheckpointID, redemptionKey(6))
	assertRedemptionOutcome(t, future, redemptions.OutcomeOutsideWindow, "Future workshop", false)
	closed := redeemLifecyclePass(t, server.URL, mainPass.QRToken, closedCheckpointID, redemptionKey(7))
	assertRedemptionOutcome(t, closed, redemptions.OutcomeOutsideWindow, "Closed workshop", false)
	inactive := redeemLifecyclePass(t, server.URL, mainPass.QRToken, inactiveCheckpointID, redemptionKey(8))
	assertRedemptionOutcome(t, inactive, redemptions.OutcomeOutsideWindow, "Inactive checkpoint", false)
	invalidQR := "qr_v1." + strings.Repeat("A", 43)
	invalid := redeemLifecyclePass(t, server.URL, invalidQR, defaultCheckpointID, redemptionKey(9))
	assertRedemptionOutcome(t, invalid, redemptions.OutcomeInvalidPass, "Main entrance", false)
	invalidRepeated := redeemLifecyclePass(t, server.URL, invalidQR, defaultCheckpointID, redemptionKey(9))
	if invalidRepeated.Outcome != redemptions.OutcomeInvalidPass {
		t.Fatalf("invalid QR idempotency replay changed outcome: %+v", invalidRepeated)
	}
	claimRedeem := redeemLifecyclePass(t, server.URL, mainPass.ClaimToken, defaultCheckpointID, redemptionKey(11))
	assertRedemptionOutcome(t, claimRedeem, redemptions.OutcomeInvalidPass, "Main entrance", false)
	revoked := redeemLifecyclePass(t, server.URL, revokedPass.QRToken, defaultCheckpointID, redemptionKey(10))
	assertRedemptionOutcome(t, revoked, redemptions.OutcomeRevokedPass, "Main entrance", false)
	if revoked.Pass == nil || revoked.Pass.Status != "revoked" {
		t.Fatalf("revoked pass did not return minimal revoked verification state: %+v", revoked)
	}

	concurrentResults := concurrentlyRedeemLifecyclePasses(t, server.URL, concurrentPass.QRToken, concurrentCheckpointID)
	outcomes := map[string]int{}
	for _, result := range concurrentResults {
		outcomes[result.Outcome]++
	}
	if outcomes[redemptions.OutcomeRedeemed] != 1 || outcomes[redemptions.OutcomeAlreadyExhausted] != 1 {
		t.Fatalf("concurrent scans exceeded or failed capacity: %+v", outcomes)
	}
	var concurrentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ats.redemptions WHERE checkpoint_id = $1`, concurrentCheckpointID).Scan(&concurrentCount); err != nil {
		t.Fatalf("count concurrent redemptions: %v", err)
	}
	if concurrentCount != 1 {
		t.Fatalf("committed redemptions exceeded maximum: %d", concurrentCount)
	}

	// A transaction paused after taking its checkpoint/pass locks must not prevent
	// an unrelated pass from redeeming at the same checkpoint.
	parallelFirstKey := redemptionKey(30)
	parallelRelease, parallelCleanup := holdRedemptionRequestAtInsert(t, ctx, pool, parallelFirstKey)
	parallelFirst := redeemLifecyclePassAsync(server.URL, mainPass.QRToken, parallelCheckpointID, parallelFirstKey)
	waitForAdvisoryBlockedRedemption(t, ctx, pool)
	parallelSecond := redeemLifecyclePassAsync(server.URL, concurrentPass.QRToken, parallelCheckpointID, redemptionKey(31))
	var secondWhileFirstBlocked asyncRedemptionResult
	secondCompletedWhileFirstBlocked := false
	select {
	case secondWhileFirstBlocked = <-parallelSecond:
		secondCompletedWhileFirstBlocked = true
	case <-time.After(time.Second):
	}
	parallelRelease()
	firstParallelResult := awaitRedemptionResult(t, parallelFirst)
	if !secondCompletedWhileFirstBlocked {
		secondWhileFirstBlocked.result = awaitRedemptionResult(t, parallelSecond)
	}
	parallelCleanup()
	if !secondCompletedWhileFirstBlocked {
		t.Fatal("a different pass could not redeem while the first pass held the same checkpoint lock")
	}
	if secondWhileFirstBlocked.err != nil {
		t.Fatalf("redeem different pass concurrently: %v", secondWhileFirstBlocked.err)
	}
	assertRedemptionOutcome(t, firstParallelResult, redemptions.OutcomeRedeemed, "Parallel entrance", true)
	assertRedemptionOutcome(t, secondWhileFirstBlocked.result, redemptions.OutcomeRedeemed, "Parallel entrance", true)

	// Organizer policy changes retain an exclusive checkpoint lock. They must wait
	// for an in-flight redemption and take effect only after that transaction ends.
	policyKey := redemptionKey(32)
	policyRelease, policyCleanup := holdRedemptionRequestAtInsert(t, ctx, pool, policyKey)
	policyRedemption := redeemLifecyclePassAsync(server.URL, mainPass.QRToken, policyCheckpointID, policyKey)
	waitForAdvisoryBlockedRedemption(t, ctx, pool)
	operationService := operations.NewService(pool, 5*time.Second, 15*time.Second)
	type updateResult struct {
		checkpoint operations.Checkpoint
		err        error
	}
	policyUpdate := make(chan updateResult, 1)
	go func() {
		updated, err := operationService.UpdateCheckpoint(context.Background(), organizerUser, policyCheckpointID, operations.CheckpointInput{
			CycleID: cycleID, Slug: "policy", Name: "Policy entrance closed", DefaultAllowed: false, DefaultMaxRedemptions: 0, Active: false,
		})
		policyUpdate <- updateResult{checkpoint: updated, err: err}
	}()
	updateCompletedDuringRedemption := false
	var updatedPolicy updateResult
	select {
	case updatedPolicy = <-policyUpdate:
		updateCompletedDuringRedemption = true
	case <-time.After(300 * time.Millisecond):
	}
	policyRelease()
	policyResult := awaitRedemptionResult(t, policyRedemption)
	if !updateCompletedDuringRedemption {
		select {
		case updatedPolicy = <-policyUpdate:
		case <-time.After(5 * time.Second):
			t.Fatal("checkpoint update did not finish after the redemption committed")
		}
	}
	policyCleanup()
	if updateCompletedDuringRedemption {
		t.Fatal("checkpoint configuration update raced with an in-flight redemption")
	}
	if updatedPolicy.err != nil {
		t.Fatalf("update checkpoint after redemption: %v", updatedPolicy.err)
	}
	assertRedemptionOutcome(t, policyResult, redemptions.OutcomeRedeemed, "Policy entrance", true)
	if updatedPolicy.checkpoint.Active || updatedPolicy.checkpoint.DefaultAllowed || updatedPolicy.checkpoint.Name != "Policy entrance closed" {
		t.Fatalf("checkpoint update was not applied after redemption: %+v", updatedPolicy.checkpoint)
	}

	assertRedemptionPersistence(t, ctx, pool, scannerID, mainAttendeeID, mainPass.ID, defaultCheckpointID, firstKey, *first.RedemptionID, mainPass.QRToken)
}

type asyncRedemptionResult struct {
	result redemptionOutcomeResponse
	err    error
}

func redeemLifecyclePassAsync(baseURL, qrToken, checkpointID, idempotencyKey string) <-chan asyncRedemptionResult {
	result := make(chan asyncRedemptionResult, 1)
	go func() {
		payload, err := json.Marshal(map[string]any{"qrToken": qrToken, "checkpointId": checkpointID, "idempotencyKey": idempotencyKey})
		if err != nil {
			result <- asyncRedemptionResult{err: fmt.Errorf("encode redemption: %w", err)}
			return
		}
		request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/redemptions", strings.NewReader(string(payload)))
		if err != nil {
			result <- asyncRedemptionResult{err: fmt.Errorf("create redemption request: %w", err)}
			return
		}
		request.Header.Set("Authorization", "Bearer clerk-redemption-scanner")
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			result <- asyncRedemptionResult{err: fmt.Errorf("perform redemption: %w", err)}
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			result <- asyncRedemptionResult{err: fmt.Errorf("redemption status: got %d", response.StatusCode)}
			return
		}
		var decoded redemptionOutcomeResponse
		if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
			result <- asyncRedemptionResult{err: fmt.Errorf("decode redemption: %w", err)}
			return
		}
		result <- asyncRedemptionResult{result: decoded}
	}()
	return result
}

func awaitRedemptionResult(t *testing.T, result <-chan asyncRedemptionResult) redemptionOutcomeResponse {
	t.Helper()
	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatalf("await redemption: %v", completed.err)
		}
		return completed.result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for redemption")
		return redemptionOutcomeResponse{}
	}
}

const redemptionTestAdvisoryLock int64 = 712026

func holdRedemptionRequestAtInsert(t *testing.T, ctx context.Context, pool *pgxpool.Pool, idempotencyKey string) (release func(), cleanup func()) {
	t.Helper()
	if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE OR REPLACE FUNCTION ats.test_hold_redemption_request() RETURNS trigger AS $$
		BEGIN
			IF NEW.idempotency_key = '%s'::uuid THEN
				PERFORM pg_advisory_xact_lock(%d);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER test_hold_redemption_request
		BEFORE INSERT ON ats.redemption_requests
		FOR EACH ROW EXECUTE FUNCTION ats.test_hold_redemption_request()`, idempotencyKey, redemptionTestAdvisoryLock)); err != nil {
		t.Fatalf("install redemption test gate: %v", err)
	}
	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin redemption test gate: %v", err)
	}
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, redemptionTestAdvisoryLock); err != nil {
		_ = blocker.Rollback(ctx)
		t.Fatalf("hold redemption test gate: %v", err)
	}
	return func() {
			if err := blocker.Commit(ctx); err != nil {
				t.Errorf("release redemption test gate: %v", err)
			}
		}, func() {
			if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_hold_redemption_request ON ats.redemption_requests;
			DROP FUNCTION IF EXISTS ats.test_hold_redemption_request()`); err != nil {
				t.Errorf("remove redemption test gate: %v", err)
			}
		}
}

func waitForAdvisoryBlockedRedemption(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND granted = false
		)`).Scan(&blocked); err != nil {
			t.Fatalf("inspect blocked redemption: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("redemption did not reach the post-lock test gate")
}

func issueRedemptionPass(t *testing.T, baseURL, attendeeID string) lifecyclePassResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/admin/attendees/"+attendeeID+"/passes", "clerk-redemption-organizer", nil)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("issue redemption pass: got %d", response.StatusCode)
	}
	var pass lifecyclePassResponse
	decodeIntakeResponse(t, response, &pass)
	return pass
}

func insertRedemptionCheckpoint(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cycleID, slug, name string, allowed bool, maximum int32, active bool, opensAt, closesAt *time.Time) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO ats.checkpoints (cycle_id, slug, name, opens_at, closes_at, default_allowed, default_max_redemptions, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text`, cycleID, slug, name, opensAt, closesAt, allowed, maximum, active).Scan(&id); err != nil {
		t.Fatalf("insert checkpoint %s: %v", slug, err)
	}
	return id
}

func insertRedemptionEntitlement(t *testing.T, ctx context.Context, pool *pgxpool.Pool, attendeeID, checkpointID, cycleID string, allowed bool, maximum int32) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO ats.attendee_entitlements (attendee_id, checkpoint_id, cycle_id, allowed, max_redemptions)
		VALUES ($1, $2, $3, $4, $5)`, attendeeID, checkpointID, cycleID, allowed, maximum); err != nil {
		t.Fatalf("insert entitlement: %v", err)
	}
}

func redeemLifecyclePass(t *testing.T, baseURL, qrToken, checkpointID, idempotencyKey string) redemptionOutcomeResponse {
	t.Helper()
	response := intakeRequest(t, baseURL, http.MethodPost, "/v1/redemptions", "clerk-redemption-scanner", map[string]any{
		"qrToken": qrToken, "checkpointId": checkpointID, "idempotencyKey": idempotencyKey,
	})
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("redeem QR: got %d", response.StatusCode)
	}
	var result redemptionOutcomeResponse
	decodeIntakeResponse(t, response, &result)
	return result
}

func redemptionKey(sequence int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", sequence)
}

func assertRedemptionOutcome(t *testing.T, result redemptionOutcomeResponse, outcome, checkpointName string, redeemed bool) {
	t.Helper()
	if result.Outcome != outcome || result.Checkpoint.Name != checkpointName || result.Checkpoint.ID == "" {
		t.Fatalf("unexpected redemption result: %+v", result)
	}
	if redeemed && (result.RedemptionID == nil || result.Attendee == nil || result.Pass == nil || result.Pass.Status != "active") {
		t.Fatalf("redeemed result omitted required minimal verification state: %+v", result)
	}
	if outcome != redemptions.OutcomeInvalidPass && (result.Attendee == nil || result.Pass == nil) {
		t.Fatalf("verification outcome omitted safe attendee or pass state: %+v", result)
	}
	if outcome == redemptions.OutcomeInvalidPass && (result.Attendee != nil || result.Pass != nil) {
		t.Fatalf("invalid pass outcome disclosed attendee or pass state: %+v", result)
	}
	if !redeemed && result.RedemptionID != nil {
		t.Fatalf("non-redemption outcome incorrectly returned a redemption ID: %+v", result)
	}
}

func concurrentlyRedeemLifecyclePasses(t *testing.T, baseURL, qrToken, checkpointID string) [2]redemptionOutcomeResponse {
	t.Helper()
	var results [2]redemptionOutcomeResponse
	var wait sync.WaitGroup
	wait.Add(len(results))
	for index := range results {
		go func(index int) {
			defer wait.Done()
			payload, err := json.Marshal(map[string]any{"qrToken": qrToken, "checkpointId": checkpointID, "idempotencyKey": redemptionKey(20 + index)})
			if err != nil {
				t.Errorf("encode concurrent redemption: %v", err)
				return
			}
			request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/redemptions", strings.NewReader(string(payload)))
			if err != nil {
				t.Errorf("create concurrent redemption request: %v", err)
				return
			}
			request.Header.Set("Authorization", "Bearer clerk-redemption-scanner")
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Errorf("perform concurrent redemption: %v", err)
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Errorf("concurrent redemption status: got %d", response.StatusCode)
				return
			}
			if err := json.NewDecoder(response.Body).Decode(&results[index]); err != nil {
				t.Errorf("decode concurrent redemption: %v", err)
			}
		}(index)
	}
	wait.Wait()
	return results
}

func assertScannerSafePayload(t *testing.T, payload map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode scanner payload: %v", err)
	}
	for _, forbidden := range []string{"application", "review", "decision", "internalReason", "internalNotes", "recommendation", "email", "qrToken", "claimToken", "Hash", "attendeeId", "passId"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("scanner response leaked %q: %s", forbidden, encoded)
		}
	}
}

func assertScannerSafeRedemptionPayload(t *testing.T, payload redemptionOutcomeResponse) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode scanner redemption payload: %v", err)
	}
	for _, forbidden := range []string{"application", "review", "decision", "internalReason", "internalNotes", "recommendation", "email", "qrToken", "claimToken", "Hash", "attendeeId", "passId"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("scanner redemption response leaked %q: %s", forbidden, encoded)
		}
	}
}

func assertRedemptionPersistence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, scannerID, attendeeID, passID, checkpointID, idempotencyKey, redemptionID, rawQR string) {
	t.Helper()
	var rowPassID, rowAttendeeID, rowCheckpointID, rowScannerID, rowKey, auditData string
	if err := pool.QueryRow(ctx, `SELECT redemption.pass_id::text, redemption.attendee_id::text, redemption.checkpoint_id::text, redemption.scanner_user_id::text, redemption.idempotency_key::text,
		COALESCE((SELECT metadata_json::text FROM ats.audit_events WHERE event_type = 'redemption_recorded' AND subject_id = redemption.id), '')
		FROM ats.redemptions AS redemption WHERE redemption.id = $1`, redemptionID).Scan(&rowPassID, &rowAttendeeID, &rowCheckpointID, &rowScannerID, &rowKey, &auditData); err != nil {
		t.Fatalf("load redemption ledger and audit: %v", err)
	}
	if rowPassID != passID || rowAttendeeID != attendeeID || rowCheckpointID != checkpointID || rowScannerID != scannerID || rowKey != idempotencyKey {
		t.Fatalf("redemption ledger lost authoritative identity: pass=%s attendee=%s checkpoint=%s scanner=%s key=%s", rowPassID, rowAttendeeID, rowCheckpointID, rowScannerID, rowKey)
	}
	for _, required := range []string{scannerID, attendeeID, passID, checkpointID, redemptionID, idempotencyKey} {
		if !strings.Contains(auditData, required) {
			t.Fatalf("redemption audit omitted %q: %s", required, auditData)
		}
	}
	if strings.Contains(auditData, rawQR) {
		t.Fatalf("redemption audit leaked raw QR credential: %s", auditData)
	}
	var qrHash string
	if err := pool.QueryRow(ctx, `SELECT encode(qr_token_hash, 'hex') FROM ats.passes WHERE id = $1`, passID).Scan(&qrHash); err != nil {
		t.Fatalf("load stored QR hash: %v", err)
	}
	if strings.Contains(auditData, qrHash) {
		t.Fatalf("redemption audit leaked QR hash: %s", auditData)
	}
	for _, statement := range []string{
		`UPDATE ats.redemptions SET ordinal = ordinal + 1 WHERE id = $1`,
		`DELETE FROM ats.redemptions WHERE id = $1`,
		`UPDATE ats.redemption_requests SET outcome = 'invalid_pass' WHERE idempotency_key = $1`,
		`DELETE FROM ats.redemption_requests WHERE idempotency_key = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, func() string {
			if strings.Contains(statement, "redemption_requests") {
				return idempotencyKey
			}
			return redemptionID
		}()); err == nil {
			t.Fatalf("append-only redemption history accepted mutation: %s", statement)
		}
	}
}
