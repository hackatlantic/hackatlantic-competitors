package httpapi

import (
	"context"
	"errors"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"net/http"
	"strconv"
)

type volunteerService interface {
	GetVolunteerRequest(context.Context, users.User) (*users.VolunteerRequest, error)
	RequestVolunteerAccess(context.Context, users.User, string) (*users.VolunteerRequest, error)
	ListVolunteerRequests(context.Context, users.User, int) ([]users.VolunteerRequest, error)
	ReviewVolunteerRequest(context.Context, users.User, string, string, string) error
}

func volunteerRequestHandler(d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, ok := requireRole(w, r, d, users.RoleApplicant)
		if !ok {
			return
		}
		if d.Volunteers == nil {
			writeError(w, 503, "dependency_unavailable", "Volunteer requests are temporarily unavailable.")
			return
		}
		var result *users.VolunteerRequest
		var err error
		if r.Method == http.MethodPost {
			var body struct {
				RealName string `json:"realName"`
			}
			if decodeIntakeJSON(r, &body) != nil {
				writeError(w, 400, "invalid_request", "Enter your name as it appears on the volunteer schedule.")
				return
			}
			result, err = d.Volunteers.RequestVolunteerAccess(r.Context(), actor, body.RealName)
		} else {
			result, err = d.Volunteers.GetVolunteerRequest(r.Context(), actor)
		}
		if err != nil {
			writeVolunteerError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Request       *users.VolunteerRequest `json:"request"`
			ScannerAccess bool                    `json:"scannerAccess"`
		}{result, actor.HasRole(users.RoleScanner)})
	}
}

func volunteerQueueHandler(d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, ok := requireRole(w, r, d, users.RoleAdmin)
		if !ok {
			return
		}
		if d.Volunteers == nil {
			writeError(w, 503, "dependency_unavailable", "Volunteer requests are temporarily unavailable.")
			return
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 || offset > 100000 {
				writeVolunteerError(w, users.ErrVolunteerInvalid)
				return
			}
		}
		items, err := d.Volunteers.ListVolunteerRequests(r.Context(), actor, offset)
		if err != nil {
			writeVolunteerError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Items []users.VolunteerRequest `json:"items"`
		}{items})
	}
}

func volunteerReviewHandler(d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, ok := requireRole(w, r, d, users.RoleAdmin)
		if !ok {
			return
		}
		if d.Volunteers == nil {
			writeError(w, 503, "dependency_unavailable", "Volunteer requests are temporarily unavailable.")
			return
		}
		var body struct {
			Status         string `json:"status"`
			ExpectedStatus string `json:"expectedStatus"`
		}
		if decodeIntakeJSON(r, &body) != nil {
			writeVolunteerError(w, users.ErrVolunteerInvalid)
			return
		}
		if err := d.Volunteers.ReviewVolunteerRequest(r.Context(), actor, r.PathValue("userId"), body.Status, body.ExpectedStatus); err != nil {
			writeVolunteerError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeVolunteerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrVolunteerInvalid):
		writeError(w, 422, "invalid_request", "Use a real name between 2 and 100 characters, or refresh the request before reviewing.")
	case errors.Is(err, users.ErrVolunteerConflict):
		writeError(w, 409, "request_changed", "This request has changed. Refresh the list before reviewing it.")
	default:
		writeStaffRoleError(w, err)
	}
}
