// Package httpapi owns HTTP routing, middleware, and transport concerns.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type serviceResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
}

// BuildInfo is safe deployment metadata compiled into the API image.
type BuildInfo struct {
	Version     string `json:"version"`
	GitSHA      string `json:"gitSha"`
	BuiltAt     string `json:"builtAt"`
	Environment string `json:"environment"`
}

type healthResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"requestId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type meResponse struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName *string  `json:"displayName"`
	Roles       []string `json:"roles"`
}

// Readiness is the database dependency required to serve traffic.
type Readiness interface {
	Ping(context.Context) error
}

// UserResolver reconciles a verified Clerk identity with local authorization.
type UserResolver interface {
	Resolve(context.Context, string) (users.User, error)
}

type staffRoleService interface {
	LookupScannerUser(context.Context, users.User, string) (users.ScannerAccessUser, error)
	GrantScannerRole(context.Context, users.User, string) error
	RevokeScannerRole(context.Context, users.User, string) error
}

// Dependencies are explicit runtime dependencies for API transport handlers.
type Dependencies struct {
	Build            BuildInfo
	Readiness        Readiness
	Verifier         auth.Verifier
	Users            UserResolver
	StaffRoles       staffRoleService
	Applications     applicationIntakeService
	Reviews          reviewWorkflowService
	Decisions        decisionLifecycleService
	Passes           passLifecycleService
	Checkpoints      checkpointService
	Redemptions      redemptionService
	Operations       organizerOperationsService
	Resumes          resumeService
	ClaimRateLimiter *ClaimRateLimiter
	AllowedOrigins   []string
}

type unavailableReadiness struct{}

func (unavailableReadiness) Ping(context.Context) error {
	return errors.New("database is not configured")
}

// NewHandler returns a dependency-safe API handler suitable for public
// liveness checks. Production startup must use NewHandlerWithDependencies.
func NewHandler(version string) http.Handler {
	return NewHandlerWithDependencies(version, Dependencies{})
}

// NewHandlerWithDependencies returns the API's root HTTP handler.
func NewHandlerWithDependencies(version string, dependencies Dependencies) http.Handler {
	if strings.TrimSpace(dependencies.Build.Version) == "" {
		dependencies.Build.Version = version
	}
	if strings.TrimSpace(dependencies.Build.GitSHA) == "" {
		dependencies.Build.GitSHA = "unknown"
	}
	if strings.TrimSpace(dependencies.Build.BuiltAt) == "" {
		dependencies.Build.BuiltAt = "unknown"
	}
	if strings.TrimSpace(dependencies.Build.Environment) == "" {
		dependencies.Build.Environment = "development"
	}
	if dependencies.Readiness == nil {
		dependencies.Readiness = unavailableReadiness{}
	}
	if dependencies.Verifier == nil {
		dependencies.Verifier = auth.RejectingVerifier{}
	}
	origins := make(map[string]struct{}, len(dependencies.AllowedOrigins))
	if dependencies.ClaimRateLimiter == nil {
		dependencies.ClaimRateLimiter = NewClaimRateLimiter(defaultClaimLimit, defaultClaimWindow, defaultClaimBucketCap)
	}
	for _, origin := range dependencies.AllowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, serviceResponse{
			Service: "hackatlantic-ats-api",
			Version: version,
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /versionz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, dependencies.Build)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, request *http.Request) {
		if err := dependencies.Readiness.Ping(request.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
	})
	mux.HandleFunc("GET /v1/me", func(w http.ResponseWriter, request *http.Request) {
		if dependencies.Users == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
			return
		}
		rawToken, err := auth.BearerToken(request)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
			return
		}
		principal, err := dependencies.Verifier.Verify(request.Context(), rawToken)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required.")
			return
		}
		user, err := dependencies.Users.Resolve(request.Context(), principal.ClerkUserID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Identity information is temporarily unavailable.")
			return
		}
		roles := make([]string, 0, len(user.Roles))
		for _, role := range []users.Role{users.RoleApplicant, users.RoleAdmin, users.RoleScanner} {
			if _, assigned := user.Roles[role]; assigned {
				roles = append(roles, string(role))
			}
		}
		writeJSON(w, http.StatusOK, meResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Roles:       roles,
		})
	})
	mux.HandleFunc("GET /v1/application-forms/current", currentApplicationFormHandler(dependencies))
	mux.HandleFunc("POST /v1/applications", createApplicationHandler(dependencies))
	mux.HandleFunc("GET /v1/applications/mine", listMyApplicationsHandler(dependencies))
	mux.HandleFunc("PUT /v1/applications/{applicationId}/draft", saveApplicationDraftHandler(dependencies))
	mux.HandleFunc("POST /v1/applications/{applicationId}/submit", submitApplicationHandler(dependencies))
	mux.HandleFunc("PUT /v1/applications/{applicationId}/resume", uploadApplicationResumeHandler(dependencies))
	mux.HandleFunc("GET /v1/applications/{applicationId}/resume", getApplicantResumeHandler(dependencies))
	mux.HandleFunc("GET /v1/applications/{applicationId}/decision", getApplicantDecisionHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/applications", listOrganizerApplicationsHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/applications/{applicationId}", getOrganizerApplicationHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/applications/{applicationId}/resume", getAdminResumeHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/applications/{applicationId}/assignments", assignReviewerHandler(dependencies))
	mux.HandleFunc("PUT /v1/admin/users/{userId}/roles/reviewer", grantReviewerRoleHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/users/scanner-access/lookup", lookupScannerUserHandler(dependencies))
	mux.HandleFunc("PUT /v1/admin/users/{userId}/roles/scanner", grantScannerRoleHandler(dependencies))
	mux.HandleFunc("DELETE /v1/admin/users/{userId}/roles/scanner", revokeScannerRoleHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/applications/{applicationId}/decisions", recordDecisionHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/decisions/{decisionId}/release", releaseDecisionHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/attendees/{attendeeId}/passes", issuePassHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/passes/{passId}/reissue", reissuePassHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/passes/{passId}/revoke", revokePassHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/activities", listOrganizerActivitiesHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/activities", createOrganizerActivityHandler(dependencies))
	mux.HandleFunc("PATCH /v1/admin/activities/{activityId}", updateOrganizerActivityHandler(dependencies))
	mux.HandleFunc("DELETE /v1/admin/activities/{activityId}", deleteOrganizerActivityHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/checkpoints", listOrganizerCheckpointsHandler(dependencies))
	mux.HandleFunc("POST /v1/admin/checkpoints", createOrganizerCheckpointHandler(dependencies))
	mux.HandleFunc("PATCH /v1/admin/checkpoints/{checkpointId}", updateOrganizerCheckpointHandler(dependencies))
	mux.HandleFunc("DELETE /v1/admin/checkpoints/{checkpointId}", deleteOrganizerCheckpointHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/attendees/{attendeeId}/entitlements/{checkpointId}", getOrganizerEntitlementHandler(dependencies))
	mux.HandleFunc("PUT /v1/admin/attendees/{attendeeId}/entitlements/{checkpointId}", putOrganizerEntitlementHandler(dependencies))
	mux.HandleFunc("DELETE /v1/admin/attendees/{attendeeId}/entitlements/{checkpointId}", deleteOrganizerEntitlementHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/redemptions/counts", listOrganizerCheckpointCountsHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/redemptions", listOrganizerRedemptionsHandler(dependencies))
	mux.HandleFunc("GET /v1/admin/exports/attendance.csv", exportOrganizerRedemptionsHandler(dependencies, operations.ExportAttendance))
	mux.HandleFunc("GET /v1/admin/exports/reconciliation.csv", exportOrganizerRedemptionsHandler(dependencies, operations.ExportReconciliation))
	mux.HandleFunc("GET /v1/attendee/pass", webPassHandler(dependencies))
	mux.HandleFunc("GET /v1/claim/{claimToken}", claimPassHandler(dependencies, dependencies.ClaimRateLimiter))
	mux.HandleFunc("GET /v1/checkpoints", listScannerCheckpointsHandler(dependencies))
	mux.HandleFunc("POST /v1/scans/lookup", scannerLookupHandler(dependencies))
	mux.HandleFunc("POST /v1/redemptions", scannerRedemptionHandler(dependencies))
	mux.HandleFunc("GET /v1/reviewer/assignments", listReviewerApplicationsHandler(dependencies))
	mux.HandleFunc("GET /v1/reviewer/applications/{applicationId}", getReviewerApplicationHandler(dependencies))
	mux.HandleFunc("PUT /v1/reviewer/applications/{applicationId}/review", saveReviewDraftHandler(dependencies))
	mux.HandleFunc("POST /v1/reviewer/applications/{applicationId}/review/submit", submitReviewHandler(dependencies))

	return securityHeaders(cors(origins, mux))
}

func cors(origins map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if _, allowed := origins[origin]; allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-File-Name")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if request.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, request)
	})
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeErrorDetails(w, status, code, message, nil)
}

func writeErrorDetails(w http.ResponseWriter, status int, code string, message string, details map[string]any) {
	writeJSON(w, status, errorResponse{Code: code, Message: message, Details: details})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
