package httpapi

import (
	"context"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"net/http/httptest"
	"strings"
	"testing"
)

type volunteerStub struct {
	calls int
	actor string
	err   error
}

func (s *volunteerStub) GetVolunteerRequest(_ context.Context, u users.User) (*users.VolunteerRequest, error) {
	s.calls++
	s.actor = u.ID
	return nil, s.err
}
func (s *volunteerStub) RequestVolunteerAccess(_ context.Context, u users.User, _ string) (*users.VolunteerRequest, error) {
	s.calls++
	s.actor = u.ID
	return &users.VolunteerRequest{UserID: u.ID, Status: "pending"}, s.err
}
func (s *volunteerStub) ListVolunteerRequests(context.Context, users.User, int) ([]users.VolunteerRequest, error) {
	s.calls++
	return []users.VolunteerRequest{}, s.err
}
func (s *volunteerStub) ReviewVolunteerRequest(context.Context, users.User, string, string, string) error {
	s.calls++
	return s.err
}

func TestVolunteerHTTPAuthorizationAndContracts(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		role                     users.Role
		token                    bool
		err                      error
		status, calls            int
	}{
		{"anonymous", "POST", "/v1/volunteer/request", `{"realName":"Alex Morgan"}`, users.RoleApplicant, false, nil, 401, 0},
		{"own request", "POST", "/v1/volunteer/request", `{"realName":"Alex Morgan"}`, users.RoleApplicant, true, nil, 200, 1},
		{"cannot pick user", "POST", "/v1/volunteer/request", `{"realName":"Alex Morgan","userId":"victim"}`, users.RoleApplicant, true, nil, 400, 0},
		{"cannot pick role", "POST", "/v1/volunteer/request", `{"realName":"Alex Morgan","role":"admin"}`, users.RoleApplicant, true, nil, 400, 0},
		{"own status", "GET", "/v1/volunteer/request", "", users.RoleApplicant, true, nil, 200, 1},
		{"private queue", "GET", "/v1/admin/volunteer-requests", "", users.RoleApplicant, true, nil, 403, 0},
		{"scanner cannot list", "GET", "/v1/admin/volunteer-requests", "", users.RoleScanner, true, nil, 403, 0},
		{"admin list", "GET", "/v1/admin/volunteer-requests", "", users.RoleAdmin, true, nil, 200, 1},
		{"invalid pagination", "GET", "/v1/admin/volunteer-requests?offset=-1", "", users.RoleAdmin, true, nil, 422, 0},
		{"no self promotion", "PUT", "/v1/admin/volunteer-requests/target", `{"status":"approved","expectedStatus":"pending"}`, users.RoleApplicant, true, nil, 403, 0},
		{"legacy organizer", "PUT", "/v1/admin/volunteer-requests/target", `{"status":"approved","expectedStatus":"pending"}`, users.RoleOrganizer, true, nil, 403, 0},
		{"admin approval", "PUT", "/v1/admin/volunteer-requests/target", `{"status":"approved","expectedStatus":"pending"}`, users.RoleAdmin, true, nil, 204, 1},
		{"stale approval", "PUT", "/v1/admin/volunteer-requests/target", `{"status":"approved","expectedStatus":"pending"}`, users.RoleAdmin, true, users.ErrVolunteerConflict, 409, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &volunteerStub{err: tc.err}
			actor := users.User{ID: "signed-in-user", Roles: map[users.Role]struct{}{tc.role: {}}}
			h := NewHandlerWithDependencies("test", Dependencies{Verifier: fakeVerifier{}, Users: fakeUsers{user: &actor}, Volunteers: s})
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.token {
				r.Header.Set("Authorization", "Bearer test")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || s.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("sensitive response cached")
			}
			if s.actor != "" && s.actor != "signed-in-user" {
				t.Fatal("request not bound to session")
			}
		})
	}
}
