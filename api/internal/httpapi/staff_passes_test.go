package httpapi

import (
	"context"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"net/http/httptest"
	"strings"
	"testing"
)

type staffPassStub struct {
	actor string
	err   error
}

func (s *staffPassStub) EnsureStaffPass(_ context.Context, actor users.User) (passes.StaffPass, error) {
	s.actor = actor.ID
	return passes.StaffPass{ID: "test", Kind: "volunteer", Status: "active", QRToken: "synthetic"}, s.err
}
func TestStaffPassHTTPIsOwnerOnly(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		token              bool
		err                error
		status             int
		called             bool
	}{
		{"signed out", "POST", "", false, nil, 401, false},
		{"owner", "POST", "", true, nil, 200, true},
		{"cannot select another owner", "POST", `{"userId":"victim","role":"admin"}`, true, nil, 200, true},
		{"ineligible", "POST", "", true, passes.ErrNotFound, 404, true},
		{"issuance is not GET", "GET", "", true, nil, 405, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &staffPassStub{err: tc.err}
			h := NewHandlerWithDependencies("test", Dependencies{Verifier: fakeVerifier{}, Users: fakeUsers{}, StaffPasses: service})
			r := httptest.NewRequest(tc.method, "/v1/staff/pass", strings.NewReader(tc.body))
			if tc.token {
				r.Header.Set("Authorization", "Bearer test")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if tc.called && service.actor != "9d18f13d-9f79-40a2-831b-c4350f806555" {
				t.Fatal("not bound to session")
			}
			if !tc.called && service.actor != "" {
				t.Fatal("unauthorized issuance")
			}
			if tc.method == "POST" && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("credential may be cached")
			}
		})
	}
}
