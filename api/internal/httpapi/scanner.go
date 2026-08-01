package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/checkpoints"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/redemptions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type checkpointService interface {
	ListActive(context.Context, users.User) ([]checkpoints.Checkpoint, error)
}

type redemptionService interface {
	Lookup(context.Context, users.User, string) (redemptions.Lookup, error)
	Redeem(context.Context, users.User, redemptions.RedeemInput) (redemptions.Result, error)
}

type scannerLookupRequest struct {
	QRToken *string `json:"qrToken"`
}

type scannerCheckpointListResponse struct {
	Items      []checkpoints.Checkpoint `json:"items"`
	NextCursor *string                  `json:"nextCursor"`
}

type scannerRedemptionRequest struct {
	QRToken        *string `json:"qrToken"`
	CheckpointID   *string `json:"checkpointId"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

func listScannerCheckpointsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		scanner, ok := requireRole(w, request, dependencies, users.RoleScanner)
		if !ok {
			return
		}
		if dependencies.Checkpoints == nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
			return
		}
		items, err := dependencies.Checkpoints.ListActive(request.Context(), scanner)
		if err != nil {
			writeCheckpointError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scannerCheckpointListResponse{Items: items})
	}
}

func scannerLookupHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		scanner, ok := requireRole(w, request, dependencies, users.RoleScanner)
		if !ok {
			return
		}
		service, ok := redemptionServiceDependency(w, dependencies)
		if !ok {
			return
		}
		var payload scannerLookupRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.QRToken == nil || strings.TrimSpace(*payload.QRToken) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		lookup, err := service.Lookup(request.Context(), scanner, strings.TrimSpace(*payload.QRToken))
		if err != nil {
			writeRedemptionError(w, err, true)
			return
		}
		writeJSON(w, http.StatusOK, lookup)
	}
}

func scannerRedemptionHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		scanner, ok := requireRole(w, request, dependencies, users.RoleScanner)
		if !ok {
			return
		}
		service, ok := redemptionServiceDependency(w, dependencies)
		if !ok {
			return
		}
		var payload scannerRedemptionRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.QRToken == nil || payload.CheckpointID == nil || payload.IdempotencyKey == nil || strings.TrimSpace(*payload.QRToken) == "" || strings.TrimSpace(*payload.CheckpointID) == "" || strings.TrimSpace(*payload.IdempotencyKey) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		result, err := service.Redeem(request.Context(), scanner, redemptions.RedeemInput{
			QRToken: strings.TrimSpace(*payload.QRToken), CheckpointID: strings.TrimSpace(*payload.CheckpointID), IdempotencyKey: strings.TrimSpace(*payload.IdempotencyKey),
		})
		if err != nil {
			writeRedemptionError(w, err, false)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func redemptionServiceDependency(w http.ResponseWriter, dependencies Dependencies) (redemptionService, bool) {
	if dependencies.Redemptions == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Redemptions, true
}

func writeCheckpointError(w http.ResponseWriter, err error) {
	if errors.Is(err, checkpoints.ErrForbidden) {
		writeError(w, http.StatusForbidden, "not_scanner", "Scanner access is required.")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
}

func writeRedemptionError(w http.ResponseWriter, err error, lookup bool) {
	switch {
	case errors.Is(err, redemptions.ErrForbidden):
		writeError(w, http.StatusForbidden, "not_scanner", "Scanner access is required.")
	case errors.Is(err, redemptions.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
	case errors.Is(err, redemptions.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "The idempotency key belongs to a different redemption operation.")
	case errors.Is(err, redemptions.ErrCheckpointNotFound):
		writeError(w, http.StatusNotFound, "checkpoint_not_found", "Checkpoint not found.")
	case lookup && errors.Is(err, redemptions.ErrNotFound):
		writeError(w, http.StatusNotFound, "pass_not_found", "Pass not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
