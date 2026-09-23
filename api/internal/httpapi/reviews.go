package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/rsvps"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type reviewWorkflowService interface {
	ListOrganizerApplications(context.Context, users.User, reviews.ListFilter) ([]reviews.Application, error)
	GetOrganizerApplication(context.Context, users.User, string) (reviews.Application, error)
	GrantReviewerRole(context.Context, users.User, string) error
	AssignReviewer(context.Context, users.User, string, string) error
	ListReviewerApplications(context.Context, users.User) ([]reviews.ReviewerApplication, error)
	GetReviewerApplication(context.Context, users.User, string) (reviews.ReviewerApplication, error)
	SaveDraft(context.Context, users.User, reviews.SaveDraftInput) (reviews.ReviewerApplication, error)
	Submit(context.Context, users.User, reviews.SubmitInput) (reviews.ReviewerApplication, error)
}

type organizerApplicationListResponse struct {
	Items      []organizerApplicationResponse `json:"items"`
	NextCursor *string                        `json:"nextCursor"`
}

type organizerApplicationResponse struct {
	reviews.Application
	RSVP *rsvps.Response `json:"rsvp,omitempty"`
}

type reviewerApplicationListResponse struct {
	Items      []reviews.ReviewerApplication `json:"items"`
	NextCursor *string                       `json:"nextCursor"`
}

type organizerApplicationDetailResponse struct {
	reviews.Application
	RSVP            *rsvps.Response          `json:"rsvp,omitempty"`
	CurrentDecision *decisions.Decision      `json:"currentDecision,omitempty"`
	AttendeePass    *passes.OrganizerSummary `json:"attendeePass,omitempty"`
}

type assignReviewerRequest struct {
	ReviewerUserID *string `json:"reviewerUserId"`
}

type assignReviewerResponse struct {
	ApplicationID  string `json:"applicationId"`
	ReviewerUserID string `json:"reviewerUserId"`
}

type saveReviewDraftRequest struct {
	LockVersion    *int32  `json:"lockVersion"`
	Score          *int32  `json:"score"`
	Recommendation *string `json:"recommendation"`
	InternalNotes  *string `json:"internalNotes"`
}

type submitReviewRequest struct {
	LockVersion *int32 `json:"lockVersion"`
}

func listOrganizerApplicationsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		rsvpFilter := request.URL.Query().Get("rsvp")
		if !rsvps.ValidFilter(rsvpFilter) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_filter", "The RSVP filter is invalid.")
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListOrganizerApplications(request.Context(), organizer, reviews.ListFilter{
			Status: request.URL.Query().Get("status"),
			Search: request.URL.Query().Get("q"),
		})
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		ids := make([]string, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		responses := map[string]rsvps.Response{}
		if dependencies.RSVPs != nil {
			responses, err = dependencies.RSVPs.ForOrganizer(request.Context(), organizer, ids)
			if err != nil {
				writeRSVPError(w, err)
				return
			}
		}
		projected := make([]organizerApplicationResponse, 0, len(items))
		for _, item := range items {
			response := organizerApplicationResponse{Application: item}
			if rsvp, ok := responses[item.ID]; ok {
				response.RSVP = &rsvp
			}
			if rsvpFilter != "" && (response.RSVP == nil || response.RSVP.Status != rsvpFilter) {
				continue
			}
			projected = append(projected, response)
		}
		writeJSON(w, http.StatusOK, organizerApplicationListResponse{Items: projected})
	}
}

func getOrganizerApplicationHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		application, err := service.GetOrganizerApplication(request.Context(), organizer, request.PathValue("applicationId"))
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		response := organizerApplicationDetailResponse{Application: application}
		if dependencies.RSVPs != nil {
			responses, err := dependencies.RSVPs.ForOrganizer(request.Context(), organizer, []string{application.ID})
			if err != nil {
				writeRSVPError(w, err)
				return
			}
			if rsvp, ok := responses[application.ID]; ok {
				response.RSVP = &rsvp
			}
		}
		if dependencies.Decisions != nil {
			decision, err := dependencies.Decisions.CurrentForOrganizer(request.Context(), organizer, application.ID)
			if err != nil && !errors.Is(err, decisions.ErrNoCurrentDecision) {
				writeDecisionRecordError(w, err)
				return
			}
			if err == nil {
				response.CurrentDecision = &decision
			}
		}
		if dependencies.Passes != nil {
			summary, err := dependencies.Passes.SummaryForApplication(request.Context(), organizer, application.ID)
			if err != nil && !errors.Is(err, passes.ErrNotFound) {
				writePassError(w, err)
				return
			}
			if err == nil {
				response.AttendeePass = &summary
			}
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func grantReviewerRoleHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		if err := service.GrantReviewerRole(request.Context(), organizer, request.PathValue("userId")); err != nil {
			if errors.Is(err, reviews.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user_not_found", "User not found.")
				return
			}
			writeWorkflowError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func assignReviewerHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		var payload assignReviewerRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.ReviewerUserID == nil || strings.TrimSpace(*payload.ReviewerUserID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		applicationID := request.PathValue("applicationId")
		if err := service.AssignReviewer(request.Context(), organizer, applicationID, *payload.ReviewerUserID); err != nil {
			writeWorkflowError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, assignReviewerResponse{ApplicationID: applicationID, ReviewerUserID: *payload.ReviewerUserID})
	}
}

func listReviewerApplicationsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		reviewer, ok := requireRole(w, request, dependencies, users.RoleReviewer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListReviewerApplications(request.Context(), reviewer)
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, reviewerApplicationListResponse{Items: items})
	}
}

func getReviewerApplicationHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		reviewer, ok := requireRole(w, request, dependencies, users.RoleReviewer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		application, err := service.GetReviewerApplication(request.Context(), reviewer, request.PathValue("applicationId"))
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func saveReviewDraftHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		reviewer, ok := requireRole(w, request, dependencies, users.RoleReviewer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		var payload saveReviewDraftRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.LockVersion == nil || payload.Score == nil || payload.Recommendation == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		application, err := service.SaveDraft(request.Context(), reviewer, reviews.SaveDraftInput{
			ApplicationID: request.PathValue("applicationId"), LockVersion: *payload.LockVersion,
			Score: *payload.Score, Recommendation: *payload.Recommendation, InternalNotes: payload.InternalNotes,
		})
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func submitReviewHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		reviewer, ok := requireRole(w, request, dependencies, users.RoleReviewer)
		if !ok {
			return
		}
		service, ok := workflowService(w, dependencies)
		if !ok {
			return
		}
		var payload submitReviewRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.LockVersion == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		application, err := service.Submit(request.Context(), reviewer, reviews.SubmitInput{
			ApplicationID: request.PathValue("applicationId"), LockVersion: *payload.LockVersion,
		})
		if err != nil {
			writeWorkflowError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func requireRole(w http.ResponseWriter, request *http.Request, dependencies Dependencies, role users.Role) (users.User, bool) {
	rawToken, err := auth.BearerToken(request)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
		return users.User{}, false
	}
	principal, err := dependencies.Verifier.Verify(request.Context(), rawToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
		return users.User{}, false
	}
	if dependencies.Users == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Identity information is temporarily unavailable.")
		return users.User{}, false
	}
	user, err := dependencies.Users.Resolve(request.Context(), principal.ClerkUserID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Identity information is temporarily unavailable.")
		return users.User{}, false
	}
	if !user.HasRole(role) {
		code, message := "not_admin", "Admin access is required."
		switch role {
		case users.RoleOrganizer:
			code, message = "not_admin", "Admin access is required."
		case users.RoleApplicant:
			code, message = "not_applicant", "Applicant access is required."
		case users.RoleScanner:
			code, message = "not_scanner", "Scanner access is required."
		}
		writeError(w, http.StatusForbidden, code, message)
		return users.User{}, false
	}
	return user, true
}

func workflowService(w http.ResponseWriter, dependencies Dependencies) (reviewWorkflowService, bool) {
	if dependencies.Reviews == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Reviews, true
}

func writeWorkflowError(w http.ResponseWriter, err error) {
	var conflict *reviews.ConflictError
	switch {
	case errors.Is(err, reviews.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Staff access is required.")
	case errors.Is(err, reviews.ErrNotFound):
		writeError(w, http.StatusNotFound, "application_not_found", "Application not found.")
	case errors.Is(err, reviews.ErrInvalidFilter):
		writeError(w, http.StatusUnprocessableEntity, "invalid_filter", "The application filter is invalid.")
	case errors.Is(err, reviews.ErrInvalidReview):
		writeError(w, http.StatusUnprocessableEntity, "invalid_review", "A review requires a score from 1 to 5 and a valid recommendation.")
	case errors.Is(err, reviews.ErrInvalidReviewer):
		writeError(w, http.StatusUnprocessableEntity, "invalid_reviewer", "The target user does not have reviewer access.")
	case errors.Is(err, reviews.ErrNotSubmitted):
		writeError(w, http.StatusConflict, "application_not_submitted", "Only submitted applications can be assigned or reviewed.")
	case errors.Is(err, reviews.ErrReviewNotStarted):
		writeError(w, http.StatusConflict, "review_not_started", "Save a review draft before submitting it.")
	case errors.As(err, &conflict):
		writeErrorDetails(w, http.StatusConflict, "review_conflict", "This review changed or is no longer a draft.", map[string]any{"lockVersion": conflict.LockVersion})
	case errors.Is(err, reviews.ErrReviewConflict):
		writeError(w, http.StatusConflict, "review_conflict", "This review changed or is no longer a draft.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
