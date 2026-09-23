package operations

import (
	"context"
	"errors"
	"fmt"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
)

// AttendanceSummary does not depend on checkpoints or an open application form.
type AttendanceSummary struct {
	CycleID        string `json:"cycleId"`
	CycleName      string `json:"cycleName"`
	ConfirmedRSVPs int64  `json:"confirmedRsvps"`
}

func (s *Service) GetAttendanceSummary(ctx context.Context, actor users.User) (AttendanceSummary, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return AttendanceSummary{}, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var summary AttendanceSummary
	err := s.pool.QueryRow(ctx, `SELECT cycle.id::text, cycle.name,
		(SELECT COUNT(*) FROM ats.applications application
		 JOIN ats.attendance_responses response ON response.decision_id = application.current_decision_id
		 WHERE application.cycle_id = cycle.id AND application.status = 'accepted'
		 AND application.decision_released_at IS NOT NULL AND response.status = 'confirmed')
		FROM ats.application_cycles cycle WHERE cycle.active`).Scan(&summary.CycleID, &summary.CycleName, &summary.ConfirmedRSVPs)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceSummary{}, ErrNotFound
	}
	if err != nil {
		return AttendanceSummary{}, fmt.Errorf("load attendance summary: %w", err)
	}
	return summary, nil
}

// EnableEntrance creates a default entrance only for an otherwise unconfigured
// active event. A cycle lock serializes concurrent setup and cycle changes.
// Replays return the existing entrance verbatim; they never reset its rules.
func (s *Service) EnableEntrance(ctx context.Context, actor users.User, cycle string) (Checkpoint, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Checkpoint{}, err
	}
	cycleID, err := parseID(cycle)
	if err != nil {
		return Checkpoint{}, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Checkpoint{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var active bool
	err = tx.QueryRow(ctx, `SELECT active FROM ats.application_cycles WHERE id = $1 FOR UPDATE`, cycleID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, err
	}
	if !active {
		return Checkpoint{}, ErrConflict
	}
	existing, err := queryCheckpoint(tx.QueryRow(ctx, `SELECT id, cycle_id, activity_id, slug, name, opens_at, closes_at,
		default_allowed, default_max_redemptions, active, created_at, updated_at
		FROM ats.checkpoints WHERE cycle_id = $1 AND slug = 'main-entrance' FOR UPDATE`, cycleID))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Checkpoint{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, err
	}
	var configured bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ats.checkpoints WHERE cycle_id = $1)`, cycleID).Scan(&configured); err != nil {
		return Checkpoint{}, err
	}
	if configured {
		return Checkpoint{}, ErrConflict
	}
	created, err := queryCheckpoint(tx.QueryRow(ctx, `INSERT INTO ats.checkpoints
		(cycle_id, slug, name, default_allowed, default_max_redemptions, active)
		VALUES ($1, 'main-entrance', 'Main entrance', true, 1, true)
		RETURNING id, cycle_id, activity_id, slug, name, opens_at, closes_at,
		default_allowed, default_max_redemptions, active, created_at, updated_at`, cycleID))
	if err != nil {
		return Checkpoint{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "checkpoint_created", "checkpoint", created.ID, checkpointAuditMetadata(created)); err != nil {
		return Checkpoint{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Checkpoint{}, err
	}
	return created, nil
}
