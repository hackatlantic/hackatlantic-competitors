// Package operations owns organizer-only event administration and operational reporting.
package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

var (
	ErrForbidden    = errors.New("organizer access is forbidden")
	ErrInvalidInput = errors.New("invalid event operations input")
	ErrNotFound     = errors.New("event operations resource not found")
	ErrConflict     = errors.New("event operations resource conflicts with existing state")
	ErrInUse        = errors.New("event operations resource is in use")
)

const (
	defaultQueryTimeout       = 5 * time.Second
	defaultTransactionTimeout = 15 * time.Second
	defaultReportLimit        = 100
	maximumReportLimit        = 500
	maximumRedemptionValue    = 1<<31 - 1
)

// Service separates organizer administration and reporting from the scanner
// redemption path. In particular, it does not participate in redemption
// idempotency, capacity counting, or outcome recording.
type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

// NewService creates the organizer operations service.
func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	return &Service{pool: pool, queryTimeout: queryTimeout, transactionTimeout: transactionTimeout}
}

// Activity is schedule metadata that can be attached to a checkpoint but is not
// inherently redeemable.
type Activity struct {
	ID        string     `json:"id"`
	CycleID   string     `json:"cycleId"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	StartsAt  *time.Time `json:"startsAt,omitempty"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// ActivityInput replaces the mutable fields of an activity. CycleID is required
// when creating an activity and ignored on update because cycle membership is
// immutable.
type ActivityInput struct {
	CycleID  string
	Slug     string
	Name     string
	StartsAt *time.Time
	EndsAt   *time.Time
}

// Checkpoint is an organizer projection. The scanner retains its deliberately
// smaller id/name projection in the checkpoints package.
type Checkpoint struct {
	ID                    string     `json:"id"`
	CycleID               string     `json:"cycleId"`
	ActivityID            *string    `json:"activityId,omitempty"`
	Slug                  string     `json:"slug"`
	Name                  string     `json:"name"`
	OpensAt               *time.Time `json:"opensAt,omitempty"`
	ClosesAt              *time.Time `json:"closesAt,omitempty"`
	DefaultAllowed        bool       `json:"defaultAllowed"`
	DefaultMaxRedemptions int        `json:"defaultMaxRedemptions"`
	Active                bool       `json:"active"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// CheckpointInput replaces the administrative fields of a checkpoint. CycleID
// is required on create and immutable afterwards. A nil ActivityID unlinks any
// activity.
type CheckpointInput struct {
	CycleID               string
	ActivityID            *string
	Slug                  string
	Name                  string
	OpensAt               *time.Time
	ClosesAt              *time.Time
	DefaultAllowed        bool
	DefaultMaxRedemptions int
	Active                bool
}

// Entitlement is the complete override rule. It intentionally replaces both
// checkpoint defaults; callers must not treat it as an additive grant.
type Entitlement struct {
	AttendeeID     string    `json:"attendeeId"`
	CheckpointID   string    `json:"checkpointId"`
	Allowed        bool      `json:"allowed"`
	MaxRedemptions int       `json:"maxRedemptions"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// EntitlementInput is the complete override stored for one attendee/checkpoint
// pair.
type EntitlementInput struct {
	Allowed        bool
	MaxRedemptions int
}

// CheckpointCount is a count of successful immutable redemptions only.
type CheckpointCount struct {
	CheckpointID     string     `json:"checkpointId"`
	CheckpointName   string     `json:"checkpointName"`
	TotalRedemptions int64      `json:"totalRedemptions"`
	LastRedeemedAt   *time.Time `json:"lastRedeemedAt,omitempty"`
}

// Redemption is an operational, privacy-minimized redemption projection. It
// intentionally excludes contact details, credentials, application answers,
// reviews, and decisions.
type Redemption struct {
	ID            string               `json:"id"`
	RedeemedAt    time.Time            `json:"redeemedAt"`
	Ordinal       int                  `json:"ordinal"`
	Checkpoint    RedemptionCheckpoint `json:"checkpoint"`
	Attendee      RedemptionAttendee   `json:"attendee"`
	Pass          RedemptionPass       `json:"pass"`
	ScannerUserID string               `json:"scannerUserId"`
}

type RedemptionCheckpoint struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type RedemptionAttendee struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type RedemptionPass struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ExportKind selects one of the explicitly minimal CSV projections.
type ExportKind string

const (
	ExportAttendance     ExportKind = "attendance"
	ExportReconciliation ExportKind = "reconciliation"
)

// ListActivities returns organizer schedule metadata for every cycle.
func (s *Service) ListActivities(ctx context.Context, actor users.User) ([]Activity, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, `SELECT id, cycle_id, slug, name, starts_at, ends_at, created_at, updated_at
		FROM ats.activities
		ORDER BY cycle_id, starts_at NULLS LAST, name, id`)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	activities := make([]Activity, 0)
	for rows.Next() {
		activity, err := scanActivity(rows)
		if err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activities: %w", err)
	}
	return activities, nil
}

// CreateActivity stores schedule metadata and records its audit event in the
// same transaction.
func (s *Service) CreateActivity(ctx context.Context, actor users.User, input ActivityInput) (Activity, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Activity{}, err
	}
	cycleID, err := parseID(input.CycleID)
	if err != nil {
		return Activity{}, ErrInvalidInput
	}
	fields, err := validateActivityInput(input)
	if err != nil {
		return Activity{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Activity{}, fmt.Errorf("begin activity creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockCycle(ctx, tx, cycleID); errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrNotFound
	} else if err != nil {
		return Activity{}, fmt.Errorf("lock activity cycle: %w", err)
	}

	created, err := queryActivity(tx.QueryRow(ctx, `INSERT INTO ats.activities (cycle_id, slug, name, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, cycle_id, slug, name, starts_at, ends_at, created_at, updated_at`, cycleID, fields.slug, fields.name, fields.startsAt, fields.endsAt))
	if isUniqueViolation(err) {
		return Activity{}, ErrConflict
	}
	if err != nil {
		return Activity{}, fmt.Errorf("create activity: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "activity_created", "activity", created.ID, map[string]any{
		"cycleId": created.CycleID, "slug": created.Slug,
	}); err != nil {
		return Activity{}, fmt.Errorf("audit activity creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Activity{}, fmt.Errorf("commit activity creation: %w", err)
	}
	return created, nil
}

// UpdateActivity replaces the mutable metadata of an activity and audits the
// exact successful change.
func (s *Service) UpdateActivity(ctx context.Context, actor users.User, activityID string, input ActivityInput) (Activity, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Activity{}, err
	}
	id, err := parseID(activityID)
	if err != nil {
		return Activity{}, ErrNotFound
	}
	fields, err := validateActivityInput(input)
	if err != nil {
		return Activity{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Activity{}, fmt.Errorf("begin activity update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := queryActivity(tx.QueryRow(ctx, `UPDATE ats.activities
		SET slug = $2, name = $3, starts_at = $4, ends_at = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING id, cycle_id, slug, name, starts_at, ends_at, created_at, updated_at`, id, fields.slug, fields.name, fields.startsAt, fields.endsAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Activity{}, ErrConflict
	}
	if err != nil {
		return Activity{}, fmt.Errorf("update activity: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "activity_updated", "activity", updated.ID, map[string]any{
		"cycleId": updated.CycleID, "slug": updated.Slug,
	}); err != nil {
		return Activity{}, fmt.Errorf("audit activity update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Activity{}, fmt.Errorf("commit activity update: %w", err)
	}
	return updated, nil
}

// DeleteActivity removes only an unreferenced activity. Checkpoints are never
// silently rewritten or deleted as a side effect.
func (s *Service) DeleteActivity(ctx context.Context, actor users.User, activityID string) error {
	actorID, err := organizerID(actor)
	if err != nil {
		return err
	}
	id, err := parseID(activityID)
	if err != nil {
		return ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin activity deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	activity, err := queryActivity(tx.QueryRow(ctx, `SELECT id, cycle_id, slug, name, starts_at, ends_at, created_at, updated_at
		FROM ats.activities WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock activity deletion: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ats.activities WHERE id = $1`, id); isForeignKeyViolation(err) {
		return ErrInUse
	} else if err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "activity_deleted", "activity", activity.ID, map[string]any{
		"cycleId": activity.CycleID, "slug": activity.Slug,
	}); err != nil {
		return fmt.Errorf("audit activity deletion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit activity deletion: %w", err)
	}
	return nil
}

// ListCheckpoints returns the complete organizer configuration while retaining
// the scanner's separate minimal checkpoint response.
func (s *Service) ListCheckpoints(ctx context.Context, actor users.User) ([]Checkpoint, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT id, cycle_id, activity_id, slug, name, opens_at, closes_at,
		default_allowed, default_max_redemptions, active, created_at, updated_at
		FROM ats.checkpoints
		ORDER BY cycle_id, name, id`)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()
	checkpoints := make([]Checkpoint, 0)
	for rows.Next() {
		checkpoint, err := scanCheckpoint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoints: %w", err)
	}
	return checkpoints, nil
}

// CreateCheckpoint creates a scannable policy configuration and audits it in
// the same transaction. Referenced activities must belong to the same cycle.
func (s *Service) CreateCheckpoint(ctx context.Context, actor users.User, input CheckpointInput) (Checkpoint, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Checkpoint{}, err
	}
	cycleID, err := parseID(input.CycleID)
	if err != nil {
		return Checkpoint{}, ErrInvalidInput
	}
	fields, err := validateCheckpointInput(input)
	if err != nil {
		return Checkpoint{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("begin checkpoint creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockCycle(ctx, tx, cycleID); errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, ErrNotFound
	} else if err != nil {
		return Checkpoint{}, fmt.Errorf("lock checkpoint cycle: %w", err)
	}
	if fields.activityID.Valid {
		if err := lockActivityInCycle(ctx, tx, fields.activityID, cycleID); errors.Is(err, pgx.ErrNoRows) {
			return Checkpoint{}, ErrNotFound
		} else if err != nil {
			return Checkpoint{}, fmt.Errorf("lock checkpoint activity: %w", err)
		}
	}
	created, err := queryCheckpoint(tx.QueryRow(ctx, `INSERT INTO ats.checkpoints
		(cycle_id, activity_id, slug, name, opens_at, closes_at, default_allowed, default_max_redemptions, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, cycle_id, activity_id, slug, name, opens_at, closes_at, default_allowed,
		default_max_redemptions, active, created_at, updated_at`, cycleID, fields.activityID, fields.slug, fields.name,
		fields.opensAt, fields.closesAt, fields.defaultAllowed, fields.defaultMaxRedemptions, fields.active))
	if isUniqueViolation(err) {
		return Checkpoint{}, ErrConflict
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("create checkpoint: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "checkpoint_created", "checkpoint", created.ID, checkpointAuditMetadata(created)); err != nil {
		return Checkpoint{}, fmt.Errorf("audit checkpoint creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Checkpoint{}, fmt.Errorf("commit checkpoint creation: %w", err)
	}
	return created, nil
}

// UpdateCheckpoint validates and locks the referenced activity before taking
// the checkpoint row lock used by redemption. This preserves a single
// activity-to-checkpoint order while still serializing policy changes with scans.
func (s *Service) UpdateCheckpoint(ctx context.Context, actor users.User, checkpointID string, input CheckpointInput) (Checkpoint, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Checkpoint{}, err
	}
	id, err := parseID(checkpointID)
	if err != nil {
		return Checkpoint{}, ErrNotFound
	}
	fields, err := validateCheckpointInput(input)
	if err != nil {
		return Checkpoint{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("begin checkpoint update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var cycleID pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT cycle_id FROM ats.checkpoints WHERE id = $1`, id).Scan(&cycleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("load checkpoint update cycle: %w", err)
	}
	if fields.activityID.Valid {
		if err := lockActivityInCycle(ctx, tx, fields.activityID, cycleID); errors.Is(err, pgx.ErrNoRows) {
			return Checkpoint{}, ErrNotFound
		} else if err != nil {
			return Checkpoint{}, fmt.Errorf("lock updated checkpoint activity: %w", err)
		}
	}
	var lockedID pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM ats.checkpoints WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checkpoint{}, ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("lock checkpoint update: %w", err)
	}
	updated, err := queryCheckpoint(tx.QueryRow(ctx, `UPDATE ats.checkpoints
		SET activity_id = $2, slug = $3, name = $4, opens_at = $5, closes_at = $6,
		default_allowed = $7, default_max_redemptions = $8, active = $9, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING id, cycle_id, activity_id, slug, name, opens_at, closes_at, default_allowed,
		default_max_redemptions, active, created_at, updated_at`, id, fields.activityID, fields.slug, fields.name,
		fields.opensAt, fields.closesAt, fields.defaultAllowed, fields.defaultMaxRedemptions, fields.active))
	if isUniqueViolation(err) {
		return Checkpoint{}, ErrConflict
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("update checkpoint: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "checkpoint_updated", "checkpoint", updated.ID, checkpointAuditMetadata(updated)); err != nil {
		return Checkpoint{}, fmt.Errorf("audit checkpoint update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Checkpoint{}, fmt.Errorf("commit checkpoint update: %w", err)
	}
	return updated, nil
}

// DeleteCheckpoint removes only a checkpoint without redemption or entitlement
// history. It never cascades through immutable redemption facts.
func (s *Service) DeleteCheckpoint(ctx context.Context, actor users.User, checkpointID string) error {
	actorID, err := organizerID(actor)
	if err != nil {
		return err
	}
	id, err := parseID(checkpointID)
	if err != nil {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin checkpoint deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	checkpoint, err := queryCheckpoint(tx.QueryRow(ctx, `SELECT id, cycle_id, activity_id, slug, name, opens_at, closes_at,
		default_allowed, default_max_redemptions, active, created_at, updated_at
		FROM ats.checkpoints WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock checkpoint deletion: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ats.checkpoints WHERE id = $1`, id); isForeignKeyViolation(err) {
		return ErrInUse
	} else if err != nil {
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "checkpoint_deleted", "checkpoint", checkpoint.ID, checkpointAuditMetadata(checkpoint)); err != nil {
		return fmt.Errorf("audit checkpoint deletion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit checkpoint deletion: %w", err)
	}
	return nil
}

// GetEntitlement returns an override only for an accepted attendee whose
// acceptance has been released. A missing override is represented by nil,
// allowing clients to distinguish it from explicit allowed=false.
func (s *Service) GetEntitlement(ctx context.Context, actor users.User, attendeeID, checkpointID string) (*Entitlement, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	attendee, err := parseID(attendeeID)
	if err != nil {
		return nil, ErrNotFound
	}
	checkpoint, err := parseID(checkpointID)
	if err != nil {
		return nil, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var storedAttendeeID, storedCheckpointID pgtype.UUID
	var allowed pgtype.Bool
	var maximum pgtype.Int4
	var createdAt, updatedAt pgtype.Timestamptz
	err = s.pool.QueryRow(ctx, `SELECT entitlement.attendee_id, entitlement.checkpoint_id, entitlement.allowed,
		entitlement.max_redemptions, entitlement.created_at, entitlement.updated_at
		FROM ats.attendees attendee
		JOIN ats.applications application ON application.id = attendee.application_id
		JOIN ats.checkpoints checkpoint ON checkpoint.id = $2 AND checkpoint.cycle_id = attendee.cycle_id
		LEFT JOIN ats.attendee_entitlements entitlement
		ON entitlement.attendee_id = attendee.id AND entitlement.checkpoint_id = checkpoint.id
		WHERE attendee.id = $1 AND application.status = 'accepted' AND application.decision_released_at IS NOT NULL`, attendee, checkpoint).Scan(
		&storedAttendeeID, &storedCheckpointID, &allowed, &maximum, &createdAt, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attendee entitlement: %w", err)
	}
	if !storedAttendeeID.Valid {
		return nil, nil
	}
	return &Entitlement{
		AttendeeID: storedAttendeeID.String(), CheckpointID: storedCheckpointID.String(), Allowed: allowed.Bool,
		MaxRedemptions: int(maximum.Int32), CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}, nil
}

// PutEntitlement atomically locks checkpoint then attendee (matching the
// redemption checkpoint-first order), validates cycle consistency, replaces the
// whole override rule, and records an audit event before commit.
func (s *Service) PutEntitlement(ctx context.Context, actor users.User, attendeeID, checkpointID string, input EntitlementInput) (Entitlement, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return Entitlement{}, err
	}
	attendee, err := parseID(attendeeID)
	if err != nil {
		return Entitlement{}, ErrNotFound
	}
	checkpoint, err := parseID(checkpointID)
	if err != nil {
		return Entitlement{}, ErrNotFound
	}
	if input.MaxRedemptions < 0 || input.MaxRedemptions > maximumRedemptionValue {
		return Entitlement{}, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Entitlement{}, fmt.Errorf("begin entitlement update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	checkpointCycle, err := lockCheckpointCycle(ctx, tx, checkpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entitlement{}, ErrNotFound
	}
	if err != nil {
		return Entitlement{}, fmt.Errorf("lock entitlement checkpoint: %w", err)
	}
	attendeeCycle, err := lockVisibleAttendeeCycle(ctx, tx, attendee)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entitlement{}, ErrNotFound
	}
	if err != nil {
		return Entitlement{}, fmt.Errorf("lock entitlement attendee: %w", err)
	}
	if attendeeCycle != checkpointCycle {
		return Entitlement{}, ErrNotFound
	}

	var storedAttendeeID, storedCheckpointID pgtype.UUID
	var maximum pgtype.Int4
	var allowed bool
	var createdAt, updatedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `INSERT INTO ats.attendee_entitlements
		(attendee_id, checkpoint_id, cycle_id, allowed, max_redemptions)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (attendee_id, checkpoint_id) DO UPDATE
		SET allowed = EXCLUDED.allowed, max_redemptions = EXCLUDED.max_redemptions, updated_at = CURRENT_TIMESTAMP
		RETURNING attendee_id, checkpoint_id, allowed, max_redemptions, created_at, updated_at`,
		attendee, checkpoint, attendeeCycle, input.Allowed, input.MaxRedemptions).Scan(
		&storedAttendeeID, &storedCheckpointID, &allowed, &maximum, &createdAt, &updatedAt,
	)
	result := Entitlement{
		AttendeeID: storedAttendeeID.String(), CheckpointID: storedCheckpointID.String(), Allowed: allowed,
		MaxRedemptions: int(maximum.Int32), CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}
	if err != nil {
		return Entitlement{}, fmt.Errorf("upsert attendee entitlement: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "entitlement_overridden", "attendee", result.AttendeeID, map[string]any{
		"checkpointId": result.CheckpointID, "allowed": result.Allowed, "maxRedemptions": result.MaxRedemptions,
	}); err != nil {
		return Entitlement{}, fmt.Errorf("audit entitlement override: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Entitlement{}, fmt.Errorf("commit entitlement update: %w", err)
	}
	return result, nil
}

// DeleteEntitlement removes a stored override so the checkpoint defaults again
// apply. Deleting an absent override is idempotent and does not create an audit
// event because no state changed.
func (s *Service) DeleteEntitlement(ctx context.Context, actor users.User, attendeeID, checkpointID string) error {
	actorID, err := organizerID(actor)
	if err != nil {
		return err
	}
	attendee, err := parseID(attendeeID)
	if err != nil {
		return ErrNotFound
	}
	checkpoint, err := parseID(checkpointID)
	if err != nil {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin entitlement deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	checkpointCycle, err := lockCheckpointCycle(ctx, tx, checkpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock entitlement checkpoint deletion: %w", err)
	}
	attendeeCycle, err := lockVisibleAttendeeCycle(ctx, tx, attendee)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock entitlement attendee deletion: %w", err)
	}
	if attendeeCycle != checkpointCycle {
		return ErrNotFound
	}
	var storedAttendeeID, storedCheckpointID pgtype.UUID
	var maximum pgtype.Int4
	var createdAt, updatedAt pgtype.Timestamptz
	var removedAllowed bool
	err = tx.QueryRow(ctx, `DELETE FROM ats.attendee_entitlements
		WHERE attendee_id = $1 AND checkpoint_id = $2
		RETURNING attendee_id, checkpoint_id, allowed, max_redemptions, created_at, updated_at`, attendee, checkpoint).Scan(
		&storedAttendeeID, &storedCheckpointID, &removedAllowed, &maximum, &createdAt, &updatedAt,
	)
	removed := Entitlement{
		AttendeeID: storedAttendeeID.String(), CheckpointID: storedCheckpointID.String(), Allowed: removedAllowed,
		MaxRedemptions: int(maximum.Int32), CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("delete attendee entitlement: %w", err)
	}
	if err == nil {
		if err := insertAudit(ctx, tx, actorID, "entitlement_removed", "attendee", removed.AttendeeID, map[string]any{
			"checkpointId": removed.CheckpointID,
		}); err != nil {
			return fmt.Errorf("audit entitlement removal: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit entitlement deletion: %w", err)
	}
	return nil
}

// ListCheckpointCounts reports successful immutable redemptions; rejected scan
// attempts are deliberately not mixed into attendance counts.
func (s *Service) ListCheckpointCounts(ctx context.Context, actor users.User) ([]CheckpointCount, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT checkpoint.id, checkpoint.name, COUNT(redemption.id), MAX(redemption.redeemed_at)
		FROM ats.checkpoints checkpoint
		LEFT JOIN ats.redemptions redemption ON redemption.checkpoint_id = checkpoint.id
		GROUP BY checkpoint.id, checkpoint.name
		ORDER BY checkpoint.name, checkpoint.id`)
	if err != nil {
		return nil, fmt.Errorf("list checkpoint counts: %w", err)
	}
	defer rows.Close()
	counts := make([]CheckpointCount, 0)
	for rows.Next() {
		var id pgtype.UUID
		var count int64
		var redeemedAt pgtype.Timestamptz
		var countItem CheckpointCount
		if err := rows.Scan(&id, &countItem.CheckpointName, &count, &redeemedAt); err != nil {
			return nil, fmt.Errorf("scan checkpoint count: %w", err)
		}
		countItem.CheckpointID = id.String()
		countItem.TotalRedemptions = count
		countItem.LastRedeemedAt = optionalTime(redeemedAt)
		counts = append(counts, countItem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint counts: %w", err)
	}
	return counts, nil
}

// ListRedemptions returns recent successful redemptions in chronological order.
// The optional checkpoint filter is validated before use and no credential,
// application, review, or decision columns are selected.
func (s *Service) ListRedemptions(ctx context.Context, actor users.User, checkpointID *string, limit int) ([]Redemption, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	filter, err := parseOptionalID(checkpointID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if limit == 0 {
		limit = defaultReportLimit
	}
	if limit < 1 || limit > maximumReportLimit {
		return nil, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.listRedemptions(ctx, filter, limit)
}

// ExportRedemptions loads the appropriate minimal projection and then records
// the organizer's export generation. The audit contains only kind, optional
// checkpoint id, and the emitted record count.
func (s *Service) ExportRedemptions(ctx context.Context, actor users.User, kind ExportKind, checkpointID *string) ([]Redemption, error) {
	actorID, err := organizerID(actor)
	if err != nil {
		return nil, err
	}
	if kind != ExportAttendance && kind != ExportReconciliation {
		return nil, ErrInvalidInput
	}
	filter, err := parseOptionalID(checkpointID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	rows, err := s.listRedemptions(ctx, filter, -1)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin redemption export audit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	metadata := map[string]any{"kind": string(kind), "recordCount": len(rows)}
	if filter.Valid {
		metadata["checkpointId"] = filter.String()
	}
	if err := insertAudit(ctx, tx, actorID, "redemption_exported", "user", actorID.String(), metadata); err != nil {
		return nil, fmt.Errorf("audit redemption export: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit redemption export audit: %w", err)
	}
	return rows, nil
}

type activityFields struct {
	slug, name       string
	startsAt, endsAt *time.Time
}

type checkpointFields struct {
	activityID            pgtype.UUID
	slug, name            string
	opensAt, closesAt     *time.Time
	defaultAllowed        bool
	defaultMaxRedemptions int
	active                bool
}

type rowScanner interface {
	Scan(...any) error
}

func validateActivityInput(input ActivityInput) (activityFields, error) {
	fields := activityFields{slug: strings.TrimSpace(input.Slug), name: strings.TrimSpace(input.Name), startsAt: normalizedTime(input.StartsAt), endsAt: normalizedTime(input.EndsAt)}
	if fields.slug == "" || fields.name == "" || invalidTimeWindow(fields.startsAt, fields.endsAt) {
		return activityFields{}, ErrInvalidInput
	}
	return fields, nil
}

func validateCheckpointInput(input CheckpointInput) (checkpointFields, error) {
	activityID, err := parseOptionalID(input.ActivityID)
	if err != nil {
		return checkpointFields{}, ErrInvalidInput
	}
	fields := checkpointFields{
		activityID: activityID, slug: strings.TrimSpace(input.Slug), name: strings.TrimSpace(input.Name),
		opensAt: normalizedTime(input.OpensAt), closesAt: normalizedTime(input.ClosesAt), defaultAllowed: input.DefaultAllowed,
		defaultMaxRedemptions: input.DefaultMaxRedemptions, active: input.Active,
	}
	if fields.slug == "" || fields.name == "" || fields.defaultMaxRedemptions < 0 || fields.defaultMaxRedemptions > maximumRedemptionValue || invalidTimeWindow(fields.opensAt, fields.closesAt) {
		return checkpointFields{}, ErrInvalidInput
	}
	return fields, nil
}

func organizerID(actor users.User) (pgtype.UUID, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return pgtype.UUID{}, ErrForbidden
	}
	id, err := parseID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse organizer id: %w", err)
	}
	return id, nil
}

func parseID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(value)); err != nil || !id.Valid {
		return pgtype.UUID{}, ErrInvalidInput
	}
	return id, nil
}

func parseOptionalID(value *string) (pgtype.UUID, error) {
	if value == nil {
		return pgtype.UUID{}, nil
	}
	return parseID(*value)
}

func normalizedTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	if value.IsZero() {
		return value
	}
	result := value.UTC()
	return &result
}

func invalidTimeWindow(start, end *time.Time) bool {
	return (start != nil && start.IsZero()) || (end != nil && end.IsZero()) || (start != nil && end != nil && !start.Before(*end))
}

func lockCycle(ctx context.Context, tx pgx.Tx, cycleID pgtype.UUID) error {
	var id pgtype.UUID
	return tx.QueryRow(ctx, `SELECT id FROM ats.application_cycles WHERE id = $1 FOR KEY SHARE`, cycleID).Scan(&id)
}

func lockActivityInCycle(ctx context.Context, tx pgx.Tx, activityID, cycleID pgtype.UUID) error {
	var id pgtype.UUID
	return tx.QueryRow(ctx, `SELECT id FROM ats.activities WHERE id = $1 AND cycle_id = $2 FOR KEY SHARE`, activityID, cycleID).Scan(&id)
}

func lockCheckpointCycle(ctx context.Context, tx pgx.Tx, checkpointID pgtype.UUID) (pgtype.UUID, error) {
	var cycleID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT cycle_id FROM ats.checkpoints WHERE id = $1 FOR UPDATE`, checkpointID).Scan(&cycleID)
	return cycleID, err
}

func lockVisibleAttendeeCycle(ctx context.Context, tx pgx.Tx, attendeeID pgtype.UUID) (pgtype.UUID, error) {
	var cycleID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT attendee.cycle_id
		FROM ats.attendees attendee
		JOIN ats.applications application ON application.id = attendee.application_id
		WHERE attendee.id = $1 AND application.status = 'accepted' AND application.decision_released_at IS NOT NULL
		FOR UPDATE OF attendee, application`, attendeeID).Scan(&cycleID)
	return cycleID, err
}

func scanActivity(row rowScanner) (Activity, error) {
	var id, cycleID pgtype.UUID
	var startsAt, endsAt, createdAt, updatedAt pgtype.Timestamptz
	var activity Activity
	err := row.Scan(&id, &cycleID, &activity.Slug, &activity.Name, &startsAt, &endsAt, &createdAt, &updatedAt)
	if err != nil {
		return Activity{}, err
	}
	activity.ID = id.String()
	activity.CycleID = cycleID.String()
	activity.StartsAt = optionalTime(startsAt)
	activity.EndsAt = optionalTime(endsAt)
	activity.CreatedAt = createdAt.Time.UTC()
	activity.UpdatedAt = updatedAt.Time.UTC()
	return activity, nil
}

func queryActivity(row rowScanner) (Activity, error) { return scanActivity(row) }

func scanCheckpoint(row rowScanner) (Checkpoint, error) {
	var id, cycleID, activityID pgtype.UUID
	var opensAt, closesAt, createdAt, updatedAt pgtype.Timestamptz
	var maximum pgtype.Int4
	var checkpoint Checkpoint
	err := row.Scan(&id, &cycleID, &activityID, &checkpoint.Slug, &checkpoint.Name, &opensAt, &closesAt,
		&checkpoint.DefaultAllowed, &maximum, &checkpoint.Active, &createdAt, &updatedAt)
	if err != nil {
		return Checkpoint{}, err
	}
	checkpoint.ID = id.String()
	checkpoint.CycleID = cycleID.String()
	if activityID.Valid {
		value := activityID.String()
		checkpoint.ActivityID = &value
	}
	checkpoint.OpensAt = optionalTime(opensAt)
	checkpoint.ClosesAt = optionalTime(closesAt)
	checkpoint.DefaultMaxRedemptions = int(maximum.Int32)
	checkpoint.CreatedAt = createdAt.Time.UTC()
	checkpoint.UpdatedAt = updatedAt.Time.UTC()
	return checkpoint, nil
}

func queryCheckpoint(row rowScanner) (Checkpoint, error) { return scanCheckpoint(row) }

func (s *Service) listRedemptions(ctx context.Context, checkpointID pgtype.UUID, limit int) ([]Redemption, error) {
	query := `SELECT redemption.id, redemption.redeemed_at, redemption.ordinal,
		checkpoint.id, checkpoint.slug, checkpoint.name,
		attendee.id, attendee.display_name, pass.id, pass.status, redemption.scanner_user_id
		FROM ats.redemptions redemption
		JOIN ats.checkpoints checkpoint ON checkpoint.id = redemption.checkpoint_id
		JOIN ats.attendees attendee ON attendee.id = redemption.attendee_id
		JOIN ats.passes pass ON pass.id = redemption.pass_id AND pass.attendee_id = attendee.id
		WHERE ($1::uuid IS NULL OR redemption.checkpoint_id = $1)
		ORDER BY redemption.redeemed_at DESC, redemption.id DESC`
	var rows pgx.Rows
	var err error
	if limit > 0 {
		rows, err = s.pool.Query(ctx, query+` LIMIT $2`, checkpointID, limit)
	} else {
		rows, err = s.pool.Query(ctx, query, checkpointID)
	}
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}
	defer rows.Close()
	result := make([]Redemption, 0)
	for rows.Next() {
		var id, checkpoint, attendee, pass, scanner pgtype.UUID
		var redeemedAt pgtype.Timestamptz
		var ordinal pgtype.Int4
		var item Redemption
		if err := rows.Scan(&id, &redeemedAt, &ordinal, &checkpoint, &item.Checkpoint.Slug, &item.Checkpoint.Name,
			&attendee, &item.Attendee.DisplayName, &pass, &item.Pass.Status, &scanner); err != nil {
			return nil, fmt.Errorf("scan redemption: %w", err)
		}
		item.ID = id.String()
		item.RedeemedAt = redeemedAt.Time.UTC()
		item.Ordinal = int(ordinal.Int32)
		item.Checkpoint.ID = checkpoint.String()
		item.Attendee.ID = attendee.String()
		item.Pass.ID = pass.String()
		item.ScannerUserID = scanner.String()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate redemptions: %w", err)
	}
	return result, nil
}

func checkpointAuditMetadata(checkpoint Checkpoint) map[string]any {
	metadata := map[string]any{
		"cycleId": checkpoint.CycleID, "slug": checkpoint.Slug, "active": checkpoint.Active,
		"defaultAllowed": checkpoint.DefaultAllowed, "defaultMaxRedemptions": checkpoint.DefaultMaxRedemptions,
	}
	if checkpoint.ActivityID != nil {
		metadata["activityId"] = *checkpoint.ActivityID
	}
	return metadata
}

func insertAudit(ctx context.Context, tx pgx.Tx, actorID pgtype.UUID, eventType, subjectType, subjectID string, metadata any) error {
	id, err := parseID(subjectID)
	if err != nil {
		return fmt.Errorf("parse audit subject: %w", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
		VALUES ($1, $2, $3, $4, $5)`, actorID, eventType, subjectType, id, encoded)
	return err
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func isUniqueViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23503"
}
