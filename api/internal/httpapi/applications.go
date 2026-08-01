package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

var (
	errMalformedIntakeRequest = errors.New("malformed intake request")
	errInvalidAnswerMap       = errors.New("invalid answer map")
)

const maxJSONRequestBytes = 1 << 20

type applicationIntakeService interface {
	CurrentForm(context.Context) (applications.Form, error)
	Create(context.Context, applications.Applicant) (applications.Application, error)
	List(context.Context, applications.Applicant) ([]applications.Application, error)
	SaveDraft(context.Context, applications.Applicant, applications.SaveDraftInput) (applications.Application, error)
	Submit(context.Context, applications.Applicant, applications.SubmitInput) (applications.Application, error)
}

type applicationListResponse struct {
	Items      []applications.Application `json:"items"`
	NextCursor *string                    `json:"nextCursor"`
}

type saveDraftRequest struct {
	LockVersion *int32          `json:"lockVersion"`
	Answers     json.RawMessage `json:"answers"`
}

type submitApplicationRequest struct {
	LockVersion *int32 `json:"lockVersion"`
}

func currentApplicationFormHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if _, ok := requireApplicant(w, request, dependencies); !ok {
			return
		}
		service, ok := applicationService(w, dependencies)
		if !ok {
			return
		}
		form, err := service.CurrentForm(request.Context())
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, form)
	}
}

func createApplicationHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireApplicant(w, request, dependencies)
		if !ok {
			return
		}
		service, ok := applicationService(w, dependencies)
		if !ok {
			return
		}
		application, err := service.Create(request.Context(), applicant)
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func listMyApplicationsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireApplicant(w, request, dependencies)
		if !ok {
			return
		}
		service, ok := applicationService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.List(request.Context(), applicant)
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, applicationListResponse{Items: items})
	}
}

func saveApplicationDraftHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireApplicant(w, request, dependencies)
		if !ok {
			return
		}
		service, ok := applicationService(w, dependencies)
		if !ok {
			return
		}
		input, err := decodeSaveDraftRequest(request)
		if errors.Is(err, errInvalidAnswerMap) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_answers", "Application answers do not match the published form.")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input.ApplicationID = request.PathValue("applicationId")
		application, err := service.SaveDraft(request.Context(), applicant, input)
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func submitApplicationHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireApplicant(w, request, dependencies)
		if !ok {
			return
		}
		service, ok := applicationService(w, dependencies)
		if !ok {
			return
		}
		input, err := decodeSubmitRequest(request)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input.ApplicationID = request.PathValue("applicationId")
		application, err := service.Submit(request.Context(), applicant, input)
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, application)
	}
}

func requireApplicant(w http.ResponseWriter, request *http.Request, dependencies Dependencies) (applications.Applicant, bool) {
	rawToken, err := auth.BearerToken(request)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
		return applications.Applicant{}, false
	}
	principal, err := dependencies.Verifier.Verify(request.Context(), rawToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
		return applications.Applicant{}, false
	}
	if dependencies.Users == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Identity information is temporarily unavailable.")
		return applications.Applicant{}, false
	}
	user, err := dependencies.Users.Resolve(request.Context(), principal.ClerkUserID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Identity information is temporarily unavailable.")
		return applications.Applicant{}, false
	}
	if !user.HasRole(users.RoleApplicant) {
		writeError(w, http.StatusForbidden, "not_applicant", "Applicant access is required.")
		return applications.Applicant{}, false
	}
	return applications.Applicant{ID: user.ID, Email: user.Email}, true
}

func applicationService(w http.ResponseWriter, dependencies Dependencies) (applicationIntakeService, bool) {
	if dependencies.Applications == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Applications, true
}

func decodeSaveDraftRequest(request *http.Request) (applications.SaveDraftInput, error) {
	var payload saveDraftRequest
	if err := decodeIntakeJSON(request, &payload); err != nil {
		return applications.SaveDraftInput{}, err
	}
	if payload.LockVersion == nil || *payload.LockVersion < 0 || len(payload.Answers) == 0 {
		return applications.SaveDraftInput{}, errMalformedIntakeRequest
	}
	var answers map[string]json.RawMessage
	if err := json.Unmarshal(payload.Answers, &answers); err != nil || answers == nil {
		return applications.SaveDraftInput{}, errInvalidAnswerMap
	}
	return applications.SaveDraftInput{LockVersion: *payload.LockVersion, Answers: answers}, nil
}

func decodeSubmitRequest(request *http.Request) (applications.SubmitInput, error) {
	var payload submitApplicationRequest
	if err := decodeIntakeJSON(request, &payload); err != nil {
		return applications.SubmitInput{}, err
	}
	if payload.LockVersion == nil || *payload.LockVersion < 0 {
		return applications.SubmitInput{}, errMalformedIntakeRequest
	}
	return applications.SubmitInput{LockVersion: *payload.LockVersion}, nil
}

func decodeIntakeJSON(request *http.Request, destination any) error {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxJSONRequestBytes+1))
	if err != nil || len(body) > maxJSONRequestBytes {
		return errMalformedIntakeRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errMalformedIntakeRequest
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errMalformedIntakeRequest
	}
	return nil
}

func writeApplicationError(w http.ResponseWriter, err error) {
	var conflict *applications.ConflictError
	switch {
	case errors.Is(err, applications.ErrNoCurrentForm):
		writeError(w, http.StatusNotFound, "current_form_not_found", "There is no open application form.")
	case errors.Is(err, applications.ErrNotFound):
		writeError(w, http.StatusNotFound, "application_not_found", "Application not found.")
	case errors.Is(err, applications.ErrApplicationWindowClosed):
		writeError(w, http.StatusConflict, "application_window_closed", "This application's submission window is closed.")
	case errors.As(err, &conflict):
		writeErrorDetails(w, http.StatusConflict, "application_conflict", "This application changed or is no longer a draft.", map[string]any{"lockVersion": conflict.LockVersion})
	case errors.Is(err, applications.ErrConflict):
		writeError(w, http.StatusConflict, "application_conflict", "This application changed or is no longer a draft.")
	case errors.Is(err, applications.ErrInvalidAnswers):
		writeError(w, http.StatusUnprocessableEntity, "invalid_answers", "Application answers do not match the published form.")
	case errors.Is(err, applications.ErrIncomplete):
		writeError(w, http.StatusUnprocessableEntity, "incomplete_application", "Complete all required questions before submitting.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
