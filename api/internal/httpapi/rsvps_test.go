package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/rsvps"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type fakeRSVPs struct {
	calls int
	actor users.User
	input rsvps.Input
	err   error
}

func (s *fakeRSVPs) GetForApplicant(_ context.Context, actor users.User, id string) (rsvps.Response, error) {
	s.calls++
	s.actor = actor
	return rsvps.Response{ApplicationID: id, DecisionID: "acceptance", Status: "pending"}, s.err
}
func (s *fakeRSVPs) Respond(_ context.Context, actor users.User, input rsvps.Input) (rsvps.Response, error) {
	s.calls++
	s.actor, s.input = actor, input
	return rsvps.Response{ApplicationID: input.ApplicationID, DecisionID: input.DecisionID, Status: input.Status, LockVersion: 1}, s.err
}
func (s *fakeRSVPs) ForOrganizer(context.Context, users.User, []string) (map[string]rsvps.Response, error) {
	return nil, s.err
}

func TestRSVPHTTPContract(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		role               users.Role
		authenticated      bool
		serviceError       error
		status, calls      int
	}{
		{"read", "GET", "", users.RoleApplicant, true, nil, 200, 1},
		{"head", "HEAD", "", users.RoleApplicant, true, nil, 200, 1},
		{"confirm", "PUT", `{"decisionId":"acceptance","status":"confirmed","lockVersion":0}`, users.RoleApplicant, true, nil, 200, 1},
		{"anonymous", "GET", "", users.RoleApplicant, false, nil, 401, 0},
		{"scanner read", "GET", "", users.RoleScanner, true, nil, 403, 0},
		{"scanner write", "PUT", `{}`, users.RoleScanner, true, nil, 403, 0},
		{"missing version", "PUT", `{"decisionId":"acceptance","status":"confirmed"}`, users.RoleApplicant, true, nil, 400, 0},
		{"null version", "PUT", `{"lockVersion":null}`, users.RoleApplicant, true, nil, 400, 0},
		{"fractional version", "PUT", `{"lockVersion":0.5}`, users.RoleApplicant, true, nil, 400, 0},
		{"unknown fields", "PUT", `{"lockVersion":0,"unexpected":true}`, users.RoleApplicant, true, nil, 400, 0},
		{"no acceptance", "GET", "", users.RoleApplicant, true, rsvps.ErrNotFound, 404, 1},
		{"stale version", "PUT", `{"lockVersion":0}`, users.RoleApplicant, true, rsvps.ErrConflict, 409, 1},
		{"invalid choice", "PUT", `{"lockVersion":0}`, users.RoleApplicant, true, rsvps.ErrInvalid, 422, 1},
		{"server failure", "GET", "", users.RoleApplicant, true, errors.New("private database diagnostic"), 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeRSVPs{err: tc.serviceError}
			user := users.User{ID: "caller", Roles: map[users.Role]struct{}{tc.role: {}}}
			handler := NewHandlerWithDependencies("test", Dependencies{Verifier: fakeVerifier{}, Users: fakeUsers{user: &user}, RSVPs: service})
			request := httptest.NewRequest(tc.method, "/v1/applications/application/rsvp", strings.NewReader(tc.body))
			if tc.authenticated {
				request.Header.Set("Authorization", "Bearer local-test")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || service.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, service.calls, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private database diagnostic") {
				t.Fatal("internal error leaked")
			}
			if tc.status == http.StatusOK && tc.method == http.MethodPut {
				var saved rsvps.Response
				if err := json.NewDecoder(response.Body).Decode(&saved); err != nil {
					t.Fatal(err)
				}
				if saved.Status != "confirmed" || saved.ApplicationID != "application" || service.input.LockVersion != 0 || service.input.DecisionID != "acceptance" || service.actor.ID != user.ID {
					t.Fatalf("incorrect RSVP request mapping: %+v", saved)
				}
			}
		})
	}
}
