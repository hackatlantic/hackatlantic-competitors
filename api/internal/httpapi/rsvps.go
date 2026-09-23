package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/rsvps"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type rsvpService interface {
	GetForApplicant(context.Context, users.User, string) (rsvps.Response, error)
	Respond(context.Context, users.User, rsvps.Input) (rsvps.Response, error)
	ForOrganizer(context.Context, users.User, []string) (map[string]rsvps.Response, error)
}

func applicantRSVPHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		actor, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		if dependencies.RSVPs == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "RSVP is temporarily unavailable.")
			return
		}
		applicationID := request.PathValue("applicationId")
		var response rsvps.Response
		var err error
		if request.Method == http.MethodGet || request.Method == http.MethodHead {
			response, err = dependencies.RSVPs.GetForApplicant(request.Context(), actor, applicationID)
		} else {
			var payload struct {
				DecisionID  string `json:"decisionId"`
				Status      string `json:"status"`
				LockVersion *int32 `json:"lockVersion"`
			}
			if decodeIntakeJSON(request, &payload) != nil || payload.LockVersion == nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "The RSVP request is invalid.")
				return
			}
			response, err = dependencies.RSVPs.Respond(request.Context(), actor, rsvps.Input{
				ApplicationID: applicationID, DecisionID: payload.DecisionID, Status: payload.Status, LockVersion: *payload.LockVersion,
			})
		}
		if err != nil {
			writeRSVPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func writeRSVPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rsvps.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You cannot access this RSVP.")
	case errors.Is(err, rsvps.ErrNotFound):
		writeError(w, http.StatusNotFound, "rsvp_not_available", "RSVP is available only for your current released acceptance.")
	case errors.Is(err, rsvps.ErrConflict):
		writeError(w, http.StatusConflict, "rsvp_conflict", "Your response or acceptance has changed. Refresh before responding again.")
	case errors.Is(err, rsvps.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_rsvp", "Choose confirmed or declined with the current response version.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Your RSVP could not be saved. Please try again.")
	}
}
