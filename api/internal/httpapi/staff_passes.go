package httpapi

import (
	"context"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"net/http"
)

type staffPassService interface {
	EnsureStaffPass(context.Context, users.User) (passes.StaffPass, error)
}

func staffPassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, ok := requireRole(w, r, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		if dependencies.StaffPasses == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
			return
		}
		pass, err := dependencies.StaffPasses.EnsureStaffPass(r.Context(), actor)
		if err != nil {
			writePassError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, pass)
	}
}
