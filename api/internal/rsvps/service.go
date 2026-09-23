// Package rsvps records attendance intentions without changing admissions or passes.
package rsvps

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden = errors.New("RSVP access forbidden")
	ErrNotFound  = errors.New("released acceptance not found")
	ErrInvalid   = errors.New("invalid RSVP")
	ErrConflict  = errors.New("RSVP or decision changed")
)

type Response struct {
	ApplicationID string     `json:"applicationId"`
	DecisionID    string     `json:"decisionId"`
	Status        string     `json:"status"`
	LockVersion   int32      `json:"lockVersion"`
	RespondedAt   *time.Time `json:"respondedAt,omitempty"`
}

type Input struct {
	ApplicationID string
	DecisionID    string
	Status        string
	LockVersion   int32
}

type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	if transactionTimeout <= 0 {
		transactionTimeout = 15 * time.Second
	}
	return &Service{pool, queryTimeout, transactionTimeout}
}

// No row means pending. Only the current, released acceptance is projected.
const responseQuery = `SELECT a.id::text, a.current_decision_id::text,
    COALESCE(r.status, 'pending'), COALESCE(r.lock_version, 0), r.responded_at
  FROM ats.applications a
  JOIN ats.decisions d ON d.id = a.current_decision_id AND d.application_id = a.id
  LEFT JOIN ats.attendance_responses r ON r.decision_id = d.id
  WHERE a.status = 'accepted' AND a.decision_released_at IS NOT NULL
    AND d.outcome = 'accepted' AND d.released_at IS NOT NULL`

func (s *Service) GetForApplicant(ctx context.Context, actor users.User, applicationID string) (Response, error) {
	if !actor.HasRole(users.RoleApplicant) {
		return Response{}, ErrForbidden
	}
	id, err := uuid(applicationID)
	if err != nil {
		return Response{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var response Response
	err = s.pool.QueryRow(ctx, responseQuery+` AND a.id = $1 AND a.applicant_user_id = $2`, id, actor.ID).Scan(
		&response.ApplicationID, &response.DecisionID, &response.Status, &response.LockVersion, &response.RespondedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, ErrNotFound
	}
	if err != nil {
		return Response{}, fmt.Errorf("read RSVP: %w", err)
	}
	return response, nil
}

// ForOrganizer batches the existing queue's IDs, avoiding a query per applicant.
func (s *Service) ForOrganizer(ctx context.Context, actor users.User, applicationIDs []string) (map[string]Response, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	result := make(map[string]Response)
	if len(applicationIDs) == 0 {
		return result, nil
	}
	ids := make([]pgtype.UUID, len(applicationIDs))
	for i, value := range applicationIDs {
		id, err := uuid(value)
		if err != nil {
			return nil, ErrNotFound
		}
		ids[i] = id
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, responseQuery+` AND a.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("list RSVPs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var response Response
		if err := rows.Scan(&response.ApplicationID, &response.DecisionID, &response.Status, &response.LockVersion, &response.RespondedAt); err != nil {
			return nil, err
		}
		result[response.ApplicationID] = response
	}
	return result, rows.Err()
}

func (s *Service) Respond(ctx context.Context, actor users.User, input Input) (Response, error) {
	if !actor.HasRole(users.RoleApplicant) {
		return Response{}, ErrForbidden
	}
	if !ValidStatus(input.Status) || input.LockVersion < 0 {
		return Response{}, ErrInvalid
	}
	id, err := uuid(input.ApplicationID)
	if err != nil {
		return Response{}, ErrNotFound
	}
	decisionID, err := uuid(input.DecisionID)
	if err != nil {
		return Response{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Decisions use the same application-row lock. Re-read the response in a
	// separate statement after acquiring it, so a waiting writer sees fresh state.
	var current pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT current_decision_id FROM ats.applications
      WHERE id = $1 AND applicant_user_id = $2 AND status = 'accepted'
        AND decision_released_at IS NOT NULL FOR UPDATE`, id, actor.ID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, ErrNotFound
	}
	if err != nil {
		return Response{}, fmt.Errorf("lock RSVP application: %w", err)
	}
	if current != decisionID {
		return Response{}, ErrConflict
	}
	var response Response
	err = tx.QueryRow(ctx, responseQuery+` AND a.id = $1`, id).Scan(
		&response.ApplicationID, &response.DecisionID, &response.Status, &response.LockVersion, &response.RespondedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, ErrNotFound
	}
	if err != nil {
		return Response{}, err
	}
	// Repeating an already-applied response is safe, even if its version is old.
	if response.Status == input.Status {
		return response, tx.Commit(ctx)
	}
	if response.LockVersion != input.LockVersion {
		return Response{}, ErrConflict
	}
	err = tx.QueryRow(ctx, `INSERT INTO ats.attendance_responses (decision_id, status, responded_by)
      VALUES ($1, $2, $3) ON CONFLICT (decision_id) DO UPDATE
      SET status = EXCLUDED.status, lock_version = ats.attendance_responses.lock_version + 1,
          responded_by = EXCLUDED.responded_by, responded_at = clock_timestamp()
      RETURNING status, lock_version, responded_at`, decisionID, input.Status, actor.ID).Scan(
		&response.Status, &response.LockVersion, &response.RespondedAt)
	if err != nil {
		return Response{}, fmt.Errorf("save RSVP: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
      VALUES ($1, 'attendance.rsvp_changed', 'application', $2,
        jsonb_build_object('decisionId', $3::text, 'status', $4::text, 'lockVersion', $5::integer))`,
		actor.ID, id, response.DecisionID, response.Status, response.LockVersion)
	if err != nil {
		return Response{}, fmt.Errorf("audit RSVP: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Response{}, err
	}
	return response, nil
}

func ValidStatus(status string) bool { return status == "confirmed" || status == "declined" }
func ValidFilter(status string) bool {
	return status == "" || status == "pending" || ValidStatus(status)
}
func uuid(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	if err == nil && !id.Valid {
		err = ErrInvalid
	}
	return id, err
}
