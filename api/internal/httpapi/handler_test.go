package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

func TestDecodeIntakeJSONRejectsOversizedBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(`{"value":"`+strings.Repeat("a", maxJSONRequestBytes)+`"}`))
	var payload map[string]any
	if err := decodeIntakeJSON(request, &payload); !errors.Is(err, errMalformedIntakeRequest) {
		t.Fatalf("oversized request: got %v", err)
	}
}

type fakeReadiness struct{ err error }

func (readiness fakeReadiness) Ping(context.Context) error { return readiness.err }

type fakeVerifier struct{ err error }

func (verifier fakeVerifier) Verify(context.Context, string) (auth.Principal, error) {
	if verifier.err != nil {
		return auth.Principal{}, verifier.err
	}
	return auth.Principal{ClerkUserID: "user_123"}, nil
}

type fakeUsers struct {
	err  error
	user *users.User
}

func (resolver fakeUsers) Resolve(context.Context, string) (users.User, error) {
	if resolver.err != nil {
		return users.User{}, resolver.err
	}
	if resolver.user != nil {
		return *resolver.user, nil
	}
	return users.User{
		ID:    "9d18f13d-9f79-40a2-831b-c4350f806555",
		Email: "applicant@example.test",
		Roles: map[users.Role]struct{}{users.RoleApplicant: {}},
	}, nil
}

func TestCurrentUserExposesOnlyPublicRoles(t *testing.T) {
	resolved := users.User{
		ID:    "9d18f13d-9f79-40a2-831b-c4350f806555",
		Email: "admin@example.test",
		Roles: map[users.Role]struct{}{
			users.RoleApplicant: {},
			users.RoleAdmin:     {},
			users.RoleOrganizer: {},
			users.RoleReviewer:  {},
		},
	}
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: fakeUsers{user: &resolved},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer session-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body meResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Roles) != 2 || body.Roles[0] != "applicant" || body.Roles[1] != "admin" {
		t.Fatalf("unexpected public roles: %+v", body.Roles)
	}
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler("test").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status body %q, got %q", "ok", body.Status)
	}
}

func TestVersionReportsOnlySafeBuildMetadata(t *testing.T) {
	want := BuildInfo{
		Version: "v1.2.3", GitSHA: "8d35c4218ffab39d15ff8136abedfcaf6f5bb49f",
		BuiltAt: "2026-08-07T16:24:00Z", Environment: "staging",
	}
	handler := NewHandlerWithDependencies("test", Dependencies{Build: want})
	request := httptest.NewRequest(http.MethodGet, "/versionz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body BuildInfo
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body != want {
		t.Fatalf("unexpected version metadata: got %+v want %+v", body, want)
	}
}

func TestReadinessReportsUnavailableDependency(t *testing.T) {
	handler := NewHandlerWithDependencies("test", Dependencies{Readiness: fakeReadiness{err: errors.New("down")}})
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "dependency_unavailable" {
		t.Fatalf("expected dependency error code, got %q", body.Code)
	}
}

func TestCurrentUserRequiresAndReturnsVerifiedIdentity(t *testing.T) {
	handler := NewHandlerWithDependencies("test", Dependencies{
		Readiness:      fakeReadiness{},
		Verifier:       fakeVerifier{},
		Users:          fakeUsers{},
		AllowedOrigins: []string{"https://app.example"},
	})

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing token to receive 401, got %d", unauthenticated.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer session-token")
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("expected explicit CORS origin, got %q", got)
	}
	var body meResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Email != "applicant@example.test" || len(body.Roles) != 1 || body.Roles[0] != "applicant" {
		t.Fatalf("unexpected current user response: %+v", body)
	}
}

func TestUnknownRoute(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()

	NewHandler("test").ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}
