package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type decisionLifecycleService interface {
	Record(context.Context, users.User, decisions.RecordInput) (decisions.Decision, error)
	Release(context.Context, users.User, string) (decisions.Decision, error)
	CurrentForOrganizer(context.Context, users.User, string) (decisions.Decision, error)
	GetReleasedForApplicant(context.Context, users.User, string) (decisions.ApplicantDecision, error)
}

type recordDecisionRequest struct {
	Outcome        *string `json:"outcome"`
	InternalReason *string `json:"internalReason"`
}

func recordDecisionHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := decisionService(w, dependencies)
		if !ok {
			return
		}
		var payload recordDecisionRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.Outcome == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		decision, err := service.Record(request.Context(), organizer, decisions.RecordInput{
			ApplicationID:  request.PathValue("applicationId"),
			Outcome:        *payload.Outcome,
			InternalReason: payload.InternalReason,
		})
		if err != nil {
			writeDecisionRecordError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, decision)
	}
}

func releaseDecisionHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := decisionService(w, dependencies)
		if !ok {
			return
		}
		decision, err := service.Release(request.Context(), organizer, request.PathValue("decisionId"))
		if err != nil {
			writeDecisionReleaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decision)
	}
}

func getApplicantDecisionHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		service, ok := decisionService(w, dependencies)
		if !ok {
			return
		}
		decision, err := service.GetReleasedForApplicant(request.Context(), applicant, request.PathValue("applicationId"))
		if err != nil {
			writeApplicantDecisionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decision)
	}
}

func decisionService(w http.ResponseWriter, dependencies Dependencies) (decisionLifecycleService, bool) {
	if dependencies.Decisions == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Decisions, true
}

func writeDecisionRecordError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, decisions.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Admin access is required.")
	case errors.Is(err, decisions.ErrNotFound):
		writeError(w, http.StatusNotFound, "application_not_found", "Application not found.")
	case errors.Is(err, decisions.ErrNotSubmitted):
		writeError(w, http.StatusConflict, "application_not_submitted", "Only submitted applications can receive decisions.")
	case errors.Is(err, decisions.ErrInvalidOutcome):
		writeError(w, http.StatusUnprocessableEntity, "invalid_decision", "The decision outcome is invalid.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

func writeDecisionReleaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, decisions.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Admin access is required.")
	case errors.Is(err, decisions.ErrNotFound):
		writeError(w, http.StatusNotFound, "decision_not_found", "Decision not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

func writeApplicantDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, decisions.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Applicant access is required.")
	case errors.Is(err, decisions.ErrNotFound):
		writeError(w, http.StatusNotFound, "decision_not_found", "Decision not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
