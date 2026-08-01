package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type passTestService struct {
	claimCalls int
}

func (service *passTestService) Issue(context.Context, users.User, string) (passes.Issuance, error) {
	return passes.Issuance{}, passes.ErrNotFound
}
func (service *passTestService) Reissue(context.Context, users.User, string) (passes.Issuance, error) {
	return passes.Issuance{}, passes.ErrNotFound
}
func (service *passTestService) Revoke(context.Context, users.User, string) (passes.Pass, error) {
	return passes.Pass{}, passes.ErrNotFound
}
func (service *passTestService) WebPass(context.Context, users.User) (passes.WebPass, error) {
	return passes.WebPass{
		Pass:    passes.Pass{ID: "pass-id", AttendeeID: "attendee-id", DisplayName: "Ada", Status: "active", IssuedAt: time.Unix(0, 0).UTC()},
		QRToken: "derived-qr-token",
	}, nil
}
func (service *passTestService) ResolveClaim(_ context.Context, token string) (passes.ClaimPass, error) {
	service.claimCalls++
	if token != "claim-credential" {
		return passes.ClaimPass{}, passes.ErrNotFound
	}
	return passes.ClaimPass{ID: "pass-id", DisplayName: "Ada", Status: "active", IssuedAt: time.Unix(0, 0).UTC()}, nil
}
func (service *passTestService) SummaryForApplication(context.Context, users.User, string) (passes.OrganizerSummary, error) {
	return passes.OrganizerSummary{}, passes.ErrNotFound
}

func TestClaimCredentialSeparationAndRateLimit(t *testing.T) {
	service := &passTestService{}
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness:        fakeReadiness{},
		Passes:           service,
		ClaimRateLimiter: NewClaimRateLimiter(1, time.Minute, 4),
	})

	claim := httptest.NewRequest(http.MethodGet, "/v1/claim/claim-credential", nil)
	claim.RemoteAddr = "192.0.2.1:1234"
	claimResponse := httptest.NewRecorder()
	handler.ServeHTTP(claimResponse, claim)
	if claimResponse.Code != http.StatusOK {
		t.Fatalf("claim credential: expected %d, got %d", http.StatusOK, claimResponse.Code)
	}

	qr := httptest.NewRequest(http.MethodGet, "/v1/claim/qr-credential", nil)
	qr.RemoteAddr = "192.0.2.2:1234"
	qrResponse := httptest.NewRecorder()
	handler.ServeHTTP(qrResponse, qr)
	if qrResponse.Code != http.StatusNotFound {
		t.Fatalf("QR credential must not resolve a claim route: expected %d, got %d", http.StatusNotFound, qrResponse.Code)
	}

	rateLimited := httptest.NewRequest(http.MethodGet, "/v1/claim/another-claim", nil)
	rateLimited.RemoteAddr = "192.0.2.1:5678"
	rateLimitedResponse := httptest.NewRecorder()
	handler.ServeHTTP(rateLimitedResponse, rateLimited)
	if rateLimitedResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("expected public claim limiter to return %d, got %d", http.StatusTooManyRequests, rateLimitedResponse.Code)
	}
	if rateLimitedResponse.Header().Get("Retry-After") != "60" {
		t.Fatalf("expected retry header for public claim limit, got %q", rateLimitedResponse.Header().Get("Retry-After"))
	}
	if service.claimCalls != 2 {
		t.Fatalf("rate-limited request reached claim resolver: calls=%d", service.claimCalls)
	}
}

func TestAuthenticatedWebPassReturnsDerivedQRButNoClaimCredential(t *testing.T) {
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: fakeUsers{}, Passes: &passTestService{},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/attendee/pass", nil)
	request.Header.Set("Authorization", "Bearer session-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected authenticated web pass to return %d, got %d", http.StatusOK, response.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode web pass: %v", err)
	}
	for _, credentialField := range []string{"claimToken", "claimUrl", "qrTokenHash", "claimTokenHash"} {
		if _, ok := body[credentialField]; ok {
			t.Fatalf("web pass leaked %s", credentialField)
		}
	}
	if body["id"] != "pass-id" || body["status"] != "active" || body["qrToken"] != "derived-qr-token" {
		t.Fatal("unexpected active web pass metadata or derived QR")
	}
}

func TestClaimRateLimiterBoundsAddressState(t *testing.T) {
	limiter := NewClaimRateLimiter(1, time.Hour, 2)
	for _, address := range []string{"192.0.2.10:1", "192.0.2.11:1", "192.0.2.12:1"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/claim/credential", nil)
		request.RemoteAddr = address
		if !limiter.Allow(request) {
			t.Fatalf("first attempt from %s was unexpectedly limited", address)
		}
	}
	if len(limiter.buckets) > 2 {
		t.Fatalf("claim limiter exceeded configured address cap: %d", len(limiter.buckets))
	}
}

func TestClaimRateLimiterUsesForwardedAddressOnlyFromTrustedProxy(t *testing.T) {
	limiter := NewClaimRateLimiter(1, time.Hour, 10)
	if err := limiter.TrustProxyCIDRs([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("configure trusted proxies: %v", err)
	}

	untrusted := httptest.NewRequest(http.MethodGet, "/v1/claim/credential", nil)
	untrusted.RemoteAddr = "192.0.2.10:1234"
	untrusted.Header.Set("X-Forwarded-For", "198.51.100.20")
	if got := limiter.requestClientAddress(untrusted); got != "192.0.2.10" {
		t.Fatalf("untrusted peer spoofed client address: %q", got)
	}

	trusted := httptest.NewRequest(http.MethodGet, "/v1/claim/credential", nil)
	trusted.RemoteAddr = "10.0.0.8:1234"
	trusted.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.7")
	if got := limiter.requestClientAddress(trusted); got != "203.0.113.5" {
		t.Fatalf("trusted proxy chain resolved wrong client: %q", got)
	}

	if err := limiter.TrustProxyCIDRs([]string{"not-a-network"}); err == nil {
		t.Fatal("invalid trusted proxy network was accepted")
	}
}
