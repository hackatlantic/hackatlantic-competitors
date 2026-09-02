package httpapi

import (
	"errors"
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

func lookupScannerUserHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		// POST keeps the email out of URL/access-log and browser-history fields.
		w.Header().Set("Cache-Control", "no-store")
		admin, ok := requireRole(w, request, dependencies, users.RoleAdmin)
		if !ok {
			return
		}
		var payload struct {
			Email string `json:"email"`
		}
		if err := decodeIntakeJSON(request, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Enter a valid email address.")
			return
		}
		if dependencies.StaffRoles == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Staff role management is temporarily unavailable.")
			return
		}
		user, err := dependencies.StaffRoles.LookupScannerUser(request.Context(), admin, payload.Email)
		if err != nil {
			writeStaffRoleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func grantScannerRoleHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		if dependencies.StaffRoles == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Staff role management is temporarily unavailable.")
			return
		}
		if err := dependencies.StaffRoles.GrantScannerRole(request.Context(), organizer, request.PathValue("userId")); err != nil {
			writeStaffRoleError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func revokeScannerRoleHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		if dependencies.StaffRoles == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Staff role management is temporarily unavailable.")
			return
		}
		if err := dependencies.StaffRoles.RevokeScannerRole(request.Context(), organizer, request.PathValue("userId")); err != nil {
			writeStaffRoleError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeStaffRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Admin access is required and self-service role changes are not allowed.")
	case errors.Is(err, users.ErrNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "No matching account found. Ask the volunteer to sign up, verify this email, and open HackAtlantic once, then try again.")
	case errors.Is(err, users.ErrInvalidEmail):
		writeError(w, http.StatusUnprocessableEntity, "invalid_email", "Enter a valid email address.")
	case errors.Is(err, users.ErrAmbiguousEmail):
		writeError(w, http.StatusConflict, "ambiguous_email", "More than one account matches this email. Resolve the duplicate accounts before changing access.")
	case errors.Is(err, users.ErrProfileUnavailable):
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Unable to verify this account right now. Try again shortly.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
