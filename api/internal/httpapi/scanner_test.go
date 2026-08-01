package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/checkpoints"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/redemptions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type scannerTestUsers struct {
	user users.User
}

func (resolver scannerTestUsers) Resolve(context.Context, string) (users.User, error) {
	return resolver.user, nil
}

type scannerTestCheckpoints struct {
	items []checkpoints.Checkpoint
	actor users.User
}

func (service *scannerTestCheckpoints) ListActive(_ context.Context, actor users.User) ([]checkpoints.Checkpoint, error) {
	service.actor = actor
	return service.items, nil
}

type scannerTestRedemptions struct {
	lookupInput string
	redeemInput redemptions.RedeemInput
	lookup      redemptions.Lookup
	result      redemptions.Result
	err         error
}

func (service *scannerTestRedemptions) Lookup(_ context.Context, _ users.User, qrToken string) (redemptions.Lookup, error) {
	service.lookupInput = qrToken
	return service.lookup, service.err
}

func (service *scannerTestRedemptions) Redeem(_ context.Context, _ users.User, input redemptions.RedeemInput) (redemptions.Result, error) {
	service.redeemInput = input
	return service.result, service.err
}

func scannerUser() users.User {
	return users.User{
		ID:    "9d18f13d-9f79-40a2-831b-c4350f806555",
		Roles: map[users.Role]struct{}{users.RoleScanner: {}},
	}
}

func TestScannerRoutesRequireGlobalScannerRoleAndReturnMinimalContracts(t *testing.T) {
	checkpointService := &scannerTestCheckpoints{items: []checkpoints.Checkpoint{{ID: "1e9f1f04-6d37-44f3-8765-0b3492851e90", Name: "Main entrance"}}}
	redemptionService := &scannerTestRedemptions{
		lookup: redemptions.Lookup{Attendee: redemptions.Attendee{DisplayName: "Ada"}, Pass: redemptions.Pass{Status: "active"}},
		result: redemptions.Result{
			Outcome:    redemptions.OutcomeRedeemed,
			Attendee:   &redemptions.Attendee{DisplayName: "Ada"},
			Pass:       &redemptions.Pass{Status: "active"},
			Checkpoint: redemptions.Checkpoint{ID: "1e9f1f04-6d37-44f3-8765-0b3492851e90", Name: "Main entrance"},
		},
	}
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: scannerTestUsers{user: scannerUser()},
		Checkpoints: checkpointService, Redemptions: redemptionService,
	})

	checkpointsRequest := httptest.NewRequest(http.MethodGet, "/v1/checkpoints", nil)
	checkpointsRequest.Header.Set("Authorization", "Bearer scanner-session")
	checkpointsResponse := httptest.NewRecorder()
	handler.ServeHTTP(checkpointsResponse, checkpointsRequest)
	if checkpointsResponse.Code != http.StatusOK {
		t.Fatalf("list global scanner checkpoints: got %d", checkpointsResponse.Code)
	}
	var checkpointPayload scannerCheckpointListResponse
	if err := json.NewDecoder(checkpointsResponse.Body).Decode(&checkpointPayload); err != nil {
		t.Fatalf("decode scanner checkpoint list: %v", err)
	}
	if len(checkpointPayload.Items) != 1 || checkpointPayload.Items[0].Name != "Main entrance" || checkpointPayload.NextCursor != nil || checkpointService.actor.ID != scannerUser().ID {
		t.Fatalf("unexpected global scanner checkpoint response: %+v", checkpointPayload)
	}

	lookupRequest := httptest.NewRequest(http.MethodPost, "/v1/scans/lookup", strings.NewReader(`{"qrToken":"qr_v1.token"}`))
	lookupRequest.Header.Set("Authorization", "Bearer scanner-session")
	lookupResponse := httptest.NewRecorder()
	handler.ServeHTTP(lookupResponse, lookupRequest)
	if lookupResponse.Code != http.StatusOK || redemptionService.lookupInput != "qr_v1.token" {
		t.Fatalf("scanner lookup was not authorized and forwarded: status=%d token=%q", lookupResponse.Code, redemptionService.lookupInput)
	}
	var lookupPayload map[string]any
	if err := json.NewDecoder(lookupResponse.Body).Decode(&lookupPayload); err != nil {
		t.Fatalf("decode scanner lookup: %v", err)
	}
	encodedLookup, err := json.Marshal(lookupPayload)
	if err != nil {
		t.Fatalf("encode scanner lookup: %v", err)
	}
	for _, forbidden := range []string{"application", "review", "decision", "email", "qrToken", "claimToken", "Hash", "id"} {
		if strings.Contains(string(encodedLookup), forbidden) {
			t.Fatalf("scanner lookup leaked %q: %s", forbidden, encodedLookup)
		}
	}

	redemptionRequest := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(`{"qrToken":"qr_v1.token","checkpointId":"1e9f1f04-6d37-44f3-8765-0b3492851e90","idempotencyKey":"2e9f1f04-6d37-44f3-8765-0b3492851e90"}`))
	redemptionRequest.Header.Set("Authorization", "Bearer scanner-session")
	redemptionResponse := httptest.NewRecorder()
	handler.ServeHTTP(redemptionResponse, redemptionRequest)
	if redemptionResponse.Code != http.StatusOK || redemptionService.redeemInput.IdempotencyKey != "2e9f1f04-6d37-44f3-8765-0b3492851e90" {
		t.Fatalf("scanner redemption did not preserve request: status=%d input=%+v", redemptionResponse.Code, redemptionService.redeemInput)
	}
}

func TestScannerRoutesRejectNonScannerMalformedAndIdempotencyConflict(t *testing.T) {
	redemptionService := &scannerTestRedemptions{err: redemptions.ErrIdempotencyConflict}
	nonScannerHandler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: fakeUsers{}, Redemptions: redemptionService,
	})
	nonScannerRequest := httptest.NewRequest(http.MethodPost, "/v1/scans/lookup", strings.NewReader(`{"qrToken":"qr_v1.token"}`))
	nonScannerRequest.Header.Set("Authorization", "Bearer applicant-session")
	nonScannerResponse := httptest.NewRecorder()
	nonScannerHandler.ServeHTTP(nonScannerResponse, nonScannerRequest)
	if nonScannerResponse.Code != http.StatusForbidden {
		t.Fatalf("non-scanner scan lookup: expected %d, got %d", http.StatusForbidden, nonScannerResponse.Code)
	}

	scannerHandler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: scannerTestUsers{user: scannerUser()}, Redemptions: redemptionService,
	})
	malformed := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(`{"qrToken":"qr_v1.token","unknown":true}`))
	malformed.Header.Set("Authorization", "Bearer scanner-session")
	malformedResponse := httptest.NewRecorder()
	scannerHandler.ServeHTTP(malformedResponse, malformed)
	if malformedResponse.Code != http.StatusBadRequest {
		t.Fatalf("malformed scanner redemption: expected %d, got %d", http.StatusBadRequest, malformedResponse.Code)
	}

	conflict := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(`{"qrToken":"qr_v1.token","checkpointId":"1e9f1f04-6d37-44f3-8765-0b3492851e90","idempotencyKey":"2e9f1f04-6d37-44f3-8765-0b3492851e90"}`))
	conflict.Header.Set("Authorization", "Bearer scanner-session")
	conflictResponse := httptest.NewRecorder()
	scannerHandler.ServeHTTP(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict: expected %d, got %d", http.StatusConflict, conflictResponse.Code)
	}
	var errorPayload errorResponse
	if err := json.NewDecoder(conflictResponse.Body).Decode(&errorPayload); err != nil {
		t.Fatalf("decode conflict: %v", err)
	}
	if !errors.Is(redemptionService.err, redemptions.ErrIdempotencyConflict) || errorPayload.Code != "idempotency_conflict" {
		t.Fatalf("unexpected conflict response: %+v", errorPayload)
	}
}
