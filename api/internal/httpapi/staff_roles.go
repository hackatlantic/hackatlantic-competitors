package httpapi

import (
	"errors"
	"net/http"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

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
		writeError(w, http.StatusNotFound, "user_not_found", "User not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
