package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

func TestCheckInSetupRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		signedIn, admin          bool
		serviceError             error
		want                     int
	}{
		{"summary signed out", "GET", "/v1/admin/attendance-summary", "", false, false, nil, 401},
		{"summary applicant", "GET", "/v1/admin/attendance-summary", "", true, false, nil, 403},
		{"summary admin", "GET", "/v1/admin/attendance-summary", "", true, true, nil, 200},
		{"no active cycle", "GET", "/v1/admin/attendance-summary", "", true, true, operations.ErrNotFound, 404},
		{"setup signed out", "POST", "/v1/admin/check-in/entrance", "{}", false, false, nil, 401},
		{"setup applicant", "POST", "/v1/admin/check-in/entrance", "{}", true, false, nil, 403},
		{"missing cycle", "POST", "/v1/admin/check-in/entrance", "{}", true, true, nil, 400},
		{"arbitrary rules", "POST", "/v1/admin/check-in/entrance", `{"cycleId":"cycle","defaultMaxRedemptions":99}`, true, true, nil, 400},
		{"malformed", "POST", "/v1/admin/check-in/entrance", "{", true, true, nil, 400},
		{"setup admin", "POST", "/v1/admin/check-in/entrance", `{"cycleId":"1e9f1f04-6d37-44f3-8765-0b3492851e90"}`, true, true, nil, 200},
		{"stale configuration", "POST", "/v1/admin/check-in/entrance", `{"cycleId":"cycle"}`, true, true, operations.ErrConflict, 409},
		{"invalid UUID", "POST", "/v1/admin/check-in/entrance", `{"cycleId":"cycle"}`, true, true, operations.ErrInvalidInput, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := organizerTestUser()
			if !tc.admin {
				actor.Roles = map[users.Role]struct{}{users.RoleApplicant: {}}
			}
			service := &operationsTestService{err: tc.serviceError, summary: operations.AttendanceSummary{CycleID: "current", CycleName: "Event", ConfirmedRSVPs: 42}}
			handler := NewHandlerWithDependencies("test", Dependencies{Readiness: fakeReadiness{}, Verifier: fakeVerifier{}, Users: operationsTestUsers{user: actor}, Operations: service})
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.signedIn {
				req.Header.Set("Authorization", "Bearer organizer-session")
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("got %d want %d: %s", res.Code, tc.want, res.Body.String())
			}
			if tc.want == http.StatusOK && tc.method == "GET" && !strings.Contains(res.Body.String(), `"confirmedRsvps":42`) {
				t.Fatal(res.Body.String())
			}
			if tc.want == http.StatusOK && tc.method == "POST" && service.entranceCycle != "1e9f1f04-6d37-44f3-8765-0b3492851e90" {
				t.Fatal("wrong cycle")
			}
			if !tc.admin && service.entranceCycle != "" {
				t.Fatal("unauthorized setup reached service")
			}
		})
	}
}
