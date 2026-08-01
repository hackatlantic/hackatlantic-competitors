package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/resumes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type resumeService interface {
	Upload(context.Context, users.User, string, string, []byte) (resumes.Metadata, error)
	ForApplicant(context.Context, users.User, string) (resumes.Metadata, error)
	ForAdmin(context.Context, users.User, string) (resumes.PDF, error)
}

func uploadApplicationResumeHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		if dependencies.Resumes == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Resume storage is unavailable.")
			return
		}
		if request.Header.Get("Content-Type") != "application/pdf" || strings.TrimSpace(request.Header.Get("X-File-Name")) == "" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_resume", "Resume must be a PDF file.")
			return
		}
		content, err := io.ReadAll(io.LimitReader(request.Body, resumes.MaxPDFBytes+1))
		if err != nil || int64(len(content)) > resumes.MaxPDFBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "resume_too_large", "Resume must be 5 MB or smaller.")
			return
		}
		metadata, err := dependencies.Resumes.Upload(request.Context(), applicant, request.PathValue("applicationId"), request.Header.Get("X-File-Name"), content)
		if err != nil {
			writeResumeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, metadata)
	}
}

func getApplicantResumeHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		applicant, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		if dependencies.Resumes == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Resume storage is unavailable.")
			return
		}
		metadata, err := dependencies.Resumes.ForApplicant(request.Context(), applicant, request.PathValue("applicationId"))
		if err != nil {
			writeResumeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, metadata)
	}
}

func getAdminResumeHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		admin, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		if dependencies.Resumes == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Resume storage is unavailable.")
			return
		}
		pdf, err := dependencies.Resumes.ForAdmin(request.Context(), admin, request.PathValue("applicationId"))
		if err != nil {
			writeResumeError(w, err)
			return
		}
		filename := strings.ReplaceAll(pdf.Metadata.OriginalFilename, `"`, "")
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(pdf.Content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdf.Content)
	}
}

func writeResumeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, resumes.ErrInvalidPDF):
		writeError(w, http.StatusUnprocessableEntity, "invalid_resume", "Resume must be a valid PDF file no larger than 5 MB.")
	case errors.Is(err, resumes.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Resume access is forbidden.")
	case errors.Is(err, resumes.ErrNotFound):
		writeError(w, http.StatusNotFound, "resume_not_found", "Resume not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
