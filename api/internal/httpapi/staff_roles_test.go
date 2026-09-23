package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type lookupRoles struct {
	calls int
	err   error
}

func (s *lookupRoles) LookupScannerUser(context.Context, users.User, string) (users.ScannerAccessUser, error) {
	s.calls++
	return users.ScannerAccessUser{ID: "test-id", Email: "volunteer@example.test", CanManage: true}, s.err
}
func (*lookupRoles) GrantScannerRole(context.Context, users.User, string) error  { return nil }
func (*lookupRoles) RevokeScannerRole(context.Context, users.User, string) error { return nil }

func TestScannerEmailLookupAuthorizationAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		role   users.Role
		token  bool
		body   string
		err    error
		status int
		calls  int
	}{
		{"anonymous", users.RoleAdmin, false, `{"email":"volunteer@example.test"}`, nil, 401, 0},
		{"applicant", users.RoleApplicant, true, `{"email":"volunteer@example.test"}`, nil, 403, 0},
		{"scanner", users.RoleScanner, true, `{"email":"volunteer@example.test"}`, nil, 403, 0},
		{"legacy organizer", users.RoleOrganizer, true, `{"email":"volunteer@example.test"}`, nil, 403, 0},
		{"admin", users.RoleAdmin, true, `{"email":"volunteer@example.test"}`, nil, 200, 1},
		{"malformed", users.RoleAdmin, true, `{"email":`, nil, 400, 0},
		{"unknown field", users.RoleAdmin, true, `{"email":"a@b.test","role":"admin"}`, nil, 400, 0},
		{"invalid email", users.RoleAdmin, true, `{"email":"bad"}`, users.ErrInvalidEmail, 422, 1},
		{"missing account", users.RoleAdmin, true, `{"email":"a@b.test"}`, users.ErrNotFound, 404, 1},
		{"ambiguous account", users.RoleAdmin, true, `{"email":"a@b.test"}`, users.ErrAmbiguousEmail, 409, 1},
		{"identity outage", users.RoleAdmin, true, `{"email":"a@b.test"}`, users.ErrProfileUnavailable, 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			roles := &lookupRoles{err: tc.err}
			actor := users.User{ID: "admin-id", Roles: map[users.Role]struct{}{tc.role: {}}}
			handler := NewHandlerWithDependencies("test", Dependencies{Verifier: fakeVerifier{}, Users: fakeUsers{user: &actor}, StaffRoles: roles})
			request := httptest.NewRequest(http.MethodPost, "/v1/admin/users/scanner-access/lookup", strings.NewReader(tc.body))
			if tc.token {
				request.Header.Set("Authorization", "Bearer test-session")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || roles.calls != tc.calls {
				t.Fatalf("status/calls: %d/%d", response.Code, roles.calls)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("lookup response must not be cached")
			}
			if strings.Contains(response.Body.String(), "clerk_user_id") || strings.Contains(response.Body.String(), "answers") {
				t.Fatal("private data leaked")
			}
		})
	}
}
