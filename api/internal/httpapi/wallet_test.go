package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type walletTestPasses struct {
	passTestService
	err          error
	actor, cycle string
}

func (s *walletTestPasses) WalletPass(ctx context.Context, actor users.User, cycle string) (passes.WebPass, error) {
	s.actor = actor.ID
	s.cycle = cycle
	if s.err != nil {
		return passes.WebPass{}, s.err
	}
	return s.WebPass(ctx, actor)
}

type walletTestSigner struct {
	calls int
	err   error
}

func (s *walletTestSigner) CycleSlug() string { return "hackatlantic-2026" }
func (s *walletTestSigner) SaveURL(passes.WebPass) (string, error) {
	s.calls++
	return "https://pay.google.com/gp/v/save/test.jwt.signature", s.err
}

func TestGoogleWalletAuthorizationAndFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		authenticated, scanner, disabled bool
		passErr, signErr                 error
		want                             int
	}{
		{name: "owner", authenticated: true, want: 200},
		{name: "anonymous", want: 401},
		{name: "scanner", authenticated: true, scanner: true, want: 403},
		{name: "disabled", authenticated: true, disabled: true, want: 503},
		{name: "unreleased revoked wrong owner or unconfirmed", authenticated: true, passErr: passes.ErrNotFound, want: 404},
		{name: "database unavailable", authenticated: true, passErr: errors.New("private database details"), want: 500},
		{name: "sign failure", authenticated: true, signErr: errors.New("private signing details"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &walletTestPasses{err: tc.passErr}
			signer := &walletTestSigner{err: tc.signErr}
			deps := Dependencies{Verifier: fakeVerifier{}, Users: fakeUsers{}, Passes: service, GoogleWallet: signer}
			if tc.disabled {
				deps.GoogleWallet = nil
			}
			if tc.scanner {
				deps.Users = fakeUsers{user: &users.User{ID: "scanner", Roles: map[users.Role]struct{}{users.RoleScanner: {}}}}
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/attendee/pass/google-wallet", strings.NewReader(`{"attendeeId":"someone-else","qrToken":"forged"}`))
			if tc.authenticated {
				request.Header.Set("Authorization", "Bearer session-token")
			}
			response := httptest.NewRecorder()
			NewHandlerWithDependencies("test", deps).ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status %d, want %d", response.Code, tc.want)
			}
			if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") {
				t.Fatal("cacheable response or private error leak")
			}
			if tc.want == 200 {
				if signer.calls != 1 || service.actor != "9d18f13d-9f79-40a2-831b-c4350f806555" || service.cycle != "hackatlantic-2026" || response.Header().Get("Referrer-Policy") != "no-referrer" {
					t.Fatal("failed ownership, cycle or privacy contract")
				}
			} else if tc.signErr == nil && signer.calls != 0 {
				t.Fatal("ineligible request reached signer")
			}
		})
	}
}
