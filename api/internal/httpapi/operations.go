package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

type organizerOperationsService interface {
	ListActivities(context.Context, users.User) ([]operations.Activity, error)
	CreateActivity(context.Context, users.User, operations.ActivityInput) (operations.Activity, error)
	UpdateActivity(context.Context, users.User, string, operations.ActivityInput) (operations.Activity, error)
	DeleteActivity(context.Context, users.User, string) error
	ListCheckpoints(context.Context, users.User) ([]operations.Checkpoint, error)
	CreateCheckpoint(context.Context, users.User, operations.CheckpointInput) (operations.Checkpoint, error)
	UpdateCheckpoint(context.Context, users.User, string, operations.CheckpointInput) (operations.Checkpoint, error)
	DeleteCheckpoint(context.Context, users.User, string) error
	GetEntitlement(context.Context, users.User, string, string) (*operations.Entitlement, error)
	PutEntitlement(context.Context, users.User, string, string, operations.EntitlementInput) (operations.Entitlement, error)
	DeleteEntitlement(context.Context, users.User, string, string) error
	ListCheckpointCounts(context.Context, users.User) ([]operations.CheckpointCount, error)
	ListRedemptions(context.Context, users.User, *string, int) ([]operations.Redemption, error)
	ExportRedemptions(context.Context, users.User, operations.ExportKind, *string) ([]operations.Redemption, error)
}

type organizerActivityListResponse struct {
	Items []operations.Activity `json:"items"`
}

type organizerCheckpointListResponse struct {
	Items []operations.Checkpoint `json:"items"`
}

type organizerEntitlementResponse struct {
	Override *operations.Entitlement `json:"override"`
}

type organizerCheckpointCountsResponse struct {
	Items []operations.CheckpointCount `json:"items"`
}

type organizerRedemptionListResponse struct {
	Items []operations.Redemption `json:"items"`
}

type activityRequest struct {
	CycleID  *string         `json:"cycleId"`
	Slug     *string         `json:"slug"`
	Name     *string         `json:"name"`
	StartsAt json.RawMessage `json:"startsAt"`
	EndsAt   json.RawMessage `json:"endsAt"`
}

type checkpointRequest struct {
	CycleID               *string         `json:"cycleId"`
	ActivityID            json.RawMessage `json:"activityId"`
	Slug                  *string         `json:"slug"`
	Name                  *string         `json:"name"`
	OpensAt               json.RawMessage `json:"opensAt"`
	ClosesAt              json.RawMessage `json:"closesAt"`
	DefaultAllowed        *bool           `json:"defaultAllowed"`
	DefaultMaxRedemptions *int            `json:"defaultMaxRedemptions"`
	Active                *bool           `json:"active"`
}

type entitlementRequest struct {
	Allowed        *bool `json:"allowed"`
	MaxRedemptions *int  `json:"maxRedemptions"`
}

func listOrganizerActivitiesHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListActivities(request.Context(), organizer)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, organizerActivityListResponse{Items: items})
	}
}

func createOrganizerActivityHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		var payload activityRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.CycleID == nil || payload.Slug == nil || payload.Name == nil || blank(*payload.CycleID) || blank(*payload.Slug) || blank(*payload.Name) {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input, inputErr := activityInput(payload, false)
		if inputErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		activity, err := service.CreateActivity(request.Context(), organizer, input)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, activity)
	}
}

func updateOrganizerActivityHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		var payload activityRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.Slug == nil || payload.Name == nil || blank(*payload.Slug) || blank(*payload.Name) {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input, inputErr := activityInput(payload, true)
		if inputErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		activity, err := service.UpdateActivity(request.Context(), organizer, request.PathValue("activityId"), input)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, activity)
	}
}

func deleteOrganizerActivityHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		if err := service.DeleteActivity(request.Context(), organizer, request.PathValue("activityId")); err != nil {
			writeOperationsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listOrganizerCheckpointsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListCheckpoints(request.Context(), organizer)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, organizerCheckpointListResponse{Items: items})
	}
}

func createOrganizerCheckpointHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		var payload checkpointRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || !validCheckpointRequest(payload, true) {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input, inputErr := checkpointInput(payload, false)
		if inputErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		checkpoint, err := service.CreateCheckpoint(request.Context(), organizer, input)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, checkpoint)
	}
}

func updateOrganizerCheckpointHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		var payload checkpointRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || !validCheckpointRequest(payload, false) {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		input, inputErr := checkpointInput(payload, true)
		if inputErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		checkpoint, err := service.UpdateCheckpoint(request.Context(), organizer, request.PathValue("checkpointId"), input)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, checkpoint)
	}
}

func deleteOrganizerCheckpointHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		if err := service.DeleteCheckpoint(request.Context(), organizer, request.PathValue("checkpointId")); err != nil {
			writeOperationsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getOrganizerEntitlementHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		override, err := service.GetEntitlement(request.Context(), organizer, request.PathValue("attendeeId"), request.PathValue("checkpointId"))
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, organizerEntitlementResponse{Override: override})
	}
}

func putOrganizerEntitlementHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		var payload entitlementRequest
		if err := decodeIntakeJSON(request, &payload); err != nil || payload.Allowed == nil || payload.MaxRedemptions == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The request body is invalid.")
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		override, err := service.PutEntitlement(request.Context(), organizer, request.PathValue("attendeeId"), request.PathValue("checkpointId"), operations.EntitlementInput{Allowed: *payload.Allowed, MaxRedemptions: *payload.MaxRedemptions})
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, override)
	}
}

func deleteOrganizerEntitlementHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		if err := service.DeleteEntitlement(request.Context(), organizer, request.PathValue("attendeeId"), request.PathValue("checkpointId")); err != nil {
			writeOperationsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listOrganizerCheckpointCountsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListCheckpointCounts(request.Context(), organizer)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, organizerCheckpointCountsResponse{Items: items})
	}
}

func listOrganizerRedemptionsHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		checkpointID := optionalQuery(request, "checkpointId")
		limit, ok := reportLimit(w, request)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		items, err := service.ListRedemptions(request.Context(), organizer, checkpointID, limit)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, organizerRedemptionListResponse{Items: items})
	}
}

func exportOrganizerRedemptionsHandler(dependencies Dependencies, kind operations.ExportKind) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := operationsService(w, dependencies)
		if !ok {
			return
		}
		rows, err := service.ExportRedemptions(request.Context(), organizer, kind, optionalQuery(request, "checkpointId"))
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		body, err := redemptionCSV(kind, rows)
		if err != nil {
			writeOperationsError(w, err)
			return
		}
		filename := "attendance.csv"
		if kind == operations.ExportReconciliation {
			filename = "reconciliation.csv"
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func operationsService(w http.ResponseWriter, dependencies Dependencies) (organizerOperationsService, bool) {
	if dependencies.Operations == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Operations, true
}

func writeOperationsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operations.ErrForbidden):
		writeError(w, http.StatusForbidden, "not_admin", "Admin access is required.")
	case errors.Is(err, operations.ErrNotFound):
		writeError(w, http.StatusNotFound, "operations_not_found", "The requested event operations resource was not found.")
	case errors.Is(err, operations.ErrConflict):
		writeError(w, http.StatusConflict, "operations_conflict", "The requested change conflicts with existing event operations state.")
	case errors.Is(err, operations.ErrInUse):
		writeError(w, http.StatusConflict, "operations_in_use", "The resource has attendance or entitlement history and cannot be deleted.")
	case errors.Is(err, operations.ErrInvalidInput):
		writeError(w, http.StatusUnprocessableEntity, "invalid_operations_input", "The requested event operations values are invalid.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

func activityInput(payload activityRequest, requireTimestamps bool) (operations.ActivityInput, error) {
	startsAt, err := nullableTimestamp(payload.StartsAt, requireTimestamps)
	if err != nil {
		return operations.ActivityInput{}, err
	}
	endsAt, err := nullableTimestamp(payload.EndsAt, requireTimestamps)
	if err != nil {
		return operations.ActivityInput{}, err
	}
	return operations.ActivityInput{
		CycleID: dereference(payload.CycleID), Slug: dereference(payload.Slug), Name: dereference(payload.Name),
		StartsAt: startsAt, EndsAt: endsAt,
	}, nil
}

func checkpointInput(payload checkpointRequest, requireNullableFields bool) (operations.CheckpointInput, error) {
	activityID, err := nullableString(payload.ActivityID, requireNullableFields)
	if err != nil {
		return operations.CheckpointInput{}, err
	}
	opensAt, err := nullableTimestamp(payload.OpensAt, requireNullableFields)
	if err != nil {
		return operations.CheckpointInput{}, err
	}
	closesAt, err := nullableTimestamp(payload.ClosesAt, requireNullableFields)
	if err != nil {
		return operations.CheckpointInput{}, err
	}
	return operations.CheckpointInput{
		CycleID: dereference(payload.CycleID), ActivityID: activityID, Slug: dereference(payload.Slug), Name: dereference(payload.Name),
		OpensAt: opensAt, ClosesAt: closesAt, DefaultAllowed: dereferenceBool(payload.DefaultAllowed),
		DefaultMaxRedemptions: dereferenceInt(payload.DefaultMaxRedemptions), Active: dereferenceBool(payload.Active),
	}, nil
}

func validCheckpointRequest(payload checkpointRequest, create bool) bool {
	if create && (payload.CycleID == nil || blank(*payload.CycleID)) {
		return false
	}
	if !create && (len(payload.ActivityID) == 0 || len(payload.OpensAt) == 0 || len(payload.ClosesAt) == 0) {
		return false
	}
	return payload.Slug != nil && !blank(*payload.Slug) && payload.Name != nil && !blank(*payload.Name) &&
		payload.DefaultAllowed != nil && payload.DefaultMaxRedemptions != nil && payload.Active != nil
}

func nullableString(value json.RawMessage, required bool) (*string, error) {
	if len(value) == 0 {
		if required {
			return nil, errors.New("missing required nullable string")
		}
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func nullableTimestamp(value json.RawMessage, required bool) (*time.Time, error) {
	if len(value) == 0 {
		if required {
			return nil, errors.New("missing required nullable timestamp")
		}
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, nil
	}
	var result time.Time
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func optionalQuery(request *http.Request, name string) *string {
	values, present := request.URL.Query()[name]
	if !present || len(values) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(values[0])
	return &trimmed
}

func reportLimit(w http.ResponseWriter, request *http.Request) (int, bool) {
	value := optionalQuery(request, "limit")
	if value == nil {
		return 0, true
	}
	limit, err := strconv.Atoi(*value)
	if err != nil || limit < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "The request query is invalid.")
		return 0, false
	}
	return limit, true
}

func redemptionCSV(kind operations.ExportKind, rows []operations.Redemption) ([]byte, error) {
	var body bytes.Buffer
	writer := csv.NewWriter(&body)
	var header []string
	switch kind {
	case operations.ExportAttendance:
		header = []string{"redeemed_at", "checkpoint_id", "checkpoint_slug", "attendee_id", "attendee_display_name"}
	case operations.ExportReconciliation:
		header = []string{"redemption_id", "redeemed_at", "checkpoint_id", "checkpoint_slug", "attendee_id", "pass_id", "scanner_user_id", "ordinal"}
	default:
		return nil, operations.ErrInvalidInput
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		var record []string
		if kind == operations.ExportAttendance {
			record = []string{row.RedeemedAt.Format(time.RFC3339), row.Checkpoint.ID, row.Checkpoint.Slug, row.Attendee.ID, row.Attendee.DisplayName}
		} else {
			record = []string{row.ID, row.RedeemedAt.Format(time.RFC3339), row.Checkpoint.ID, row.Checkpoint.Slug, row.Attendee.ID, row.Pass.ID, row.ScannerUserID, strconv.Itoa(row.Ordinal)}
		}
		if err := writer.Write(spreadsheetSafeRecord(record)); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

// spreadsheetSafeRecord prevents user-controlled values from being evaluated
// as formulas when an organizer opens an export in spreadsheet software.
func spreadsheetSafeRecord(record []string) []string {
	safe := make([]string, len(record))
	for index, value := range record {
		safe[index] = spreadsheetSafeCell(value)
	}
	return safe
}

func spreadsheetSafeCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if value == "" || trimmed == "" {
		return value
	}
	if strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.ContainsAny(value[:1], "\t\r\n") {
		return "'" + value
	}
	return value
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dereferenceBool(value *bool) bool {
	return value != nil && *value
}

func dereferenceInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }
