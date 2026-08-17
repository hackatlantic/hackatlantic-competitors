// Package redemptions owns scanner QR resolution and append-only redemption recording.
package redemptions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/entitlements"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden           = errors.New("scanner access is forbidden")
	ErrNotFound            = errors.New("active pass not found")
	ErrInvalidInput        = errors.New("invalid redemption input")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to a different operation")
	ErrCheckpointNotFound  = errors.New("active checkpoint not found")
)

const (
	OutcomeRedeemed         = "redeemed"
	OutcomeAlreadyExhausted = "already_exhausted"
	OutcomeNotEntitled      = "not_entitled"
	OutcomeOutsideWindow    = "outside_window"
	OutcomeInvalidPass      = "invalid_pass"
	OutcomeRevokedPass      = "revoked_pass"

	defaultTransactionTimeout = 15 * time.Second
	transactionRetries        = 3
)

// QRTokenHasher is implemented by passes.Service. It retains ownership of the
// M5 QR prefix validation and purpose-separated HMAC used by ats.passes.
type QRTokenHasher interface {
	QRTokenHash(string) ([]byte, bool)
}

// Attendee and Pass are intentionally scanner-safe projections. They exclude
// application, review, decision, contact, and credential data.
type Attendee struct {
	DisplayName string `json:"displayName"`
}

type Pass struct {
	Status string `json:"status"`
}

type Checkpoint struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Lookup is the only QR-resolution projection. It exposes only active or
// revoked state for accepted, released attendees so scanners can stop a
// revoked credential before attempting a redemption.
type Lookup struct {
	Attendee Attendee `json:"attendee"`
	Pass     Pass     `json:"pass"`
}

type RedeemInput struct {
	QRToken        string
	CheckpointID   string
	IdempotencyKey string
}

// Result is persisted as a safe snapshot in the immutable idempotency ledger.
// The same request can therefore replay the original authoritative result.
type Result struct {
	Outcome      string     `json:"outcome"`
	Attendee     *Attendee  `json:"attendee,omitempty"`
	Pass         *Pass      `json:"pass,omitempty"`
	Checkpoint   Checkpoint `json:"checkpoint"`
	RedemptionID *string    `json:"redemptionId,omitempty"`
}

type Service struct {
	pool               *pgxpool.Pool
	transactionTimeout time.Duration
	qrTokenHasher      QRTokenHasher
}

func NewService(pool *pgxpool.Pool, transactionTimeout time.Duration, qrTokenHasher QRTokenHasher) *Service {
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	return &Service{pool: pool, transactionTimeout: transactionTimeout, qrTokenHasher: qrTokenHasher}
}

// Lookup resolves only a valid M5 QR credential to a visible active or revoked
// pass. Replaced credentials remain indistinguishable from invalid credentials.
// Claim credentials fail QRTokenHash validation before any database lookup.
func (s *Service) Lookup(ctx context.Context, actor users.User, qrToken string) (Lookup, error) {
	if !actor.HasRole(users.RoleScanner) {
		return Lookup{}, ErrForbidden
	}
	if s.qrTokenHasher == nil {
		return Lookup{}, fmt.Errorf("QR token hasher is unavailable")
	}
	qrHash, valid := s.qrTokenHasher.QRTokenHash(qrToken)
	if !valid {
		return Lookup{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()

	var displayName, status string
	err := s.pool.QueryRow(ctx, `SELECT attendee.display_name, pass.status
		FROM ats.passes AS pass
		JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
		JOIN ats.applications AS application ON application.id = attendee.application_id
		WHERE pass.qr_token_hash = $1
		  AND pass.status IN ('active', 'revoked')
		  AND application.status = 'accepted'
		  AND application.decision_released_at IS NOT NULL`, qrHash).Scan(&displayName, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lookup{}, ErrNotFound
	}
	if err != nil {
		return Lookup{}, fmt.Errorf("resolve QR pass: %w", err)
	}
	return Lookup{Attendee: Attendee{DisplayName: displayName}, Pass: Pass{Status: status}}, nil
}

// Redeem records a scanner request exactly once. Every decision, its safe
// snapshot, and any successful redemption are inserted in one transaction.
// Pass-row locks serialize the same attendee credential, the shared checkpoint
// lock prevents policy changes from racing, and database uniqueness protects
// idempotency. Retrying serialization/unique races re-reads the idempotency
// ledger rather than producing a second redemption.
func (s *Service) Redeem(ctx context.Context, actor users.User, input RedeemInput) (Result, error) {
	if !actor.HasRole(users.RoleScanner) {
		return Result{}, ErrForbidden
	}
	if s.qrTokenHasher == nil {
		return Result{}, fmt.Errorf("QR token hasher is unavailable")
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Result{}, fmt.Errorf("parse scanner ID: %w", err)
	}
	checkpointID, err := parseUUID(input.CheckpointID)
	if err != nil {
		return Result{}, ErrInvalidInput
	}
	idempotencyKey, err := parseUUID(input.IdempotencyKey)
	if err != nil {
		return Result{}, ErrInvalidInput
	}
	qrHash, validQR := s.qrTokenHasher.QRTokenHash(input.QRToken)
	if len(qrHash) != 32 {
		return Result{}, fmt.Errorf("QR token hasher returned an invalid hash length")
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	for range transactionRetries {
		result, retry, err := s.redeemAttempt(ctx, actorID, checkpointID, idempotencyKey, qrHash, validQR)
		if retry && ctx.Err() == nil {
			continue
		}
		return result, err
	}
	return Result{}, fmt.Errorf("redeem transaction did not complete after %d attempts", transactionRetries)
}

type redemptionRequestRow struct {
	checkpointID        pgtype.UUID
	qrTokenHash         []byte
	passID              pgtype.UUID
	attendeeID          pgtype.UUID
	outcome             string
	attendeeDisplayName pgtype.Text
	passStatus          pgtype.Text
	checkpointName      string
	redemptionID        pgtype.UUID
}

type lockedCheckpoint struct {
	id                    pgtype.UUID
	cycleID               pgtype.UUID
	name                  string
	opensAt               pgtype.Timestamptz
	closesAt              pgtype.Timestamptz
	defaultAllowed        bool
	defaultMaxRedemptions int32
	active                bool
}

type lockedPass struct {
	id          pgtype.UUID
	attendeeID  pgtype.UUID
	cycleID     pgtype.UUID
	displayName string
	status      string
	eligible    bool
}

func (s *Service) redeemAttempt(ctx context.Context, scannerID, checkpointID, idempotencyKey pgtype.UUID, qrHash []byte, validQR bool) (Result, bool, error) {
	// READ COMMITTED is deliberate. The stronger SERIALIZABLE level turns the
	// initial absent-idempotency and zero-redemption index probes into broad
	// predicate conflicts, so unrelated passes can repeatedly abort under a
	// normal check-in burst. The explicit pass/checkpoint row locks and unique
	// constraints below provide the required, narrower serialization boundaries.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Result{}, false, fmt.Errorf("begin redemption transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	request, err := findRedemptionRequest(ctx, tx, idempotencyKey)
	if err == nil {
		if request.checkpointID != checkpointID || !bytes.Equal(request.qrTokenHash, qrHash) {
			return Result{}, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return Result{}, isRetryableTransactionError(err), fmt.Errorf("commit idempotent redemption replay: %w", err)
		}
		return resultFromRequest(request), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("lock redemption idempotency request: %w", err)
	}

	checkpoint, err := lockCheckpoint(ctx, tx, checkpointID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, ErrCheckpointNotFound
	}
	if err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("lock checkpoint: %w", err)
	}

	if !validQR {
		result, err := recordOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, outcome: OutcomeInvalidPass,
		})
		if err != nil {
			return Result{}, isRetryableTransactionError(err), err
		}
		if err := tx.Commit(ctx); err != nil {
			return Result{}, isRetryableTransactionError(err), fmt.Errorf("commit invalid pass redemption request: %w", err)
		}
		return result, false, nil
	}

	pass, err := lockPassByQRHash(ctx, tx, qrHash)
	if errors.Is(err, pgx.ErrNoRows) {
		result, recordErr := recordOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, outcome: OutcomeInvalidPass,
		})
		if recordErr != nil {
			return Result{}, isRetryableTransactionError(recordErr), recordErr
		}
		if err := tx.Commit(ctx); err != nil {
			return Result{}, isRetryableTransactionError(err), fmt.Errorf("commit invalid pass redemption request: %w", err)
		}
		return result, false, nil
	}
	if err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("lock pass for redemption: %w", err)
	}
	if pass.status == "revoked" && pass.eligible {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeRevokedPass,
		})
	}
	if pass.status != "active" || !pass.eligible {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, outcome: OutcomeInvalidPass,
		})
	}
	if !checkpoint.active {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeOutsideWindow,
		})
	}
	if checkpoint.cycleID != pass.cycleID {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeNotEntitled,
		})
	}

	var databaseNow pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&databaseNow); err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("read redemption clock: %w", err)
	}
	if (checkpoint.opensAt.Valid && databaseNow.Time.Before(checkpoint.opensAt.Time)) || (checkpoint.closesAt.Valid && !databaseNow.Time.Before(checkpoint.closesAt.Time)) {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeOutsideWindow,
		})
	}

	effective, err := effectiveEntitlement(ctx, tx, pass, checkpoint)
	if err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("resolve attendee entitlement: %w", err)
	}
	if !effective.Allowed {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeNotEntitled,
		})
	}
	var committed int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM ats.redemptions WHERE attendee_id = $1 AND checkpoint_id = $2`, pass.attendeeID, checkpoint.id).Scan(&committed); err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("count committed redemptions: %w", err)
	}
	if committed >= int64(effective.MaxRedemptions) {
		return s.commitOutcome(ctx, tx, requestRecord{
			scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeAlreadyExhausted,
		})
	}
	ordinal := int32(committed + 1)
	return s.commitOutcome(ctx, tx, requestRecord{
		scannerID: scannerID, checkpoint: checkpoint, idempotencyKey: idempotencyKey, qrTokenHash: qrHash, pass: &pass, outcome: OutcomeRedeemed, ordinal: ordinal,
	})
}

func (s *Service) commitOutcome(ctx context.Context, tx pgx.Tx, record requestRecord) (Result, bool, error) {
	result, err := recordOutcome(ctx, tx, record)
	if err != nil {
		return Result{}, isRetryableTransactionError(err), err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, isRetryableTransactionError(err), fmt.Errorf("commit redemption request: %w", err)
	}
	return result, false, nil
}

type requestRecord struct {
	scannerID      pgtype.UUID
	checkpoint     lockedCheckpoint
	idempotencyKey pgtype.UUID
	qrTokenHash    []byte
	pass           *lockedPass
	outcome        string
	ordinal        int32
}

func findRedemptionRequest(ctx context.Context, tx pgx.Tx, idempotencyKey pgtype.UUID) (redemptionRequestRow, error) {
	var row redemptionRequestRow
	err := tx.QueryRow(ctx, `SELECT checkpoint_id, qr_token_hash, pass_id, attendee_id, outcome, attendee_display_name, pass_status, checkpoint_name, redemption_id
		FROM ats.redemption_requests WHERE idempotency_key = $1 FOR UPDATE`, idempotencyKey).Scan(
		&row.checkpointID, &row.qrTokenHash, &row.passID, &row.attendeeID, &row.outcome, &row.attendeeDisplayName, &row.passStatus, &row.checkpointName, &row.redemptionID,
	)
	return row, err
}

func lockCheckpoint(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (lockedCheckpoint, error) {
	var checkpoint lockedCheckpoint
	err := tx.QueryRow(ctx, `SELECT id, cycle_id, name, opens_at, closes_at, default_allowed, default_max_redemptions, active
		FROM ats.checkpoints WHERE id = $1 FOR SHARE`, id).Scan(
		&checkpoint.id, &checkpoint.cycleID, &checkpoint.name, &checkpoint.opensAt, &checkpoint.closesAt,
		&checkpoint.defaultAllowed, &checkpoint.defaultMaxRedemptions, &checkpoint.active,
	)
	return checkpoint, err
}

func lockPassByQRHash(ctx context.Context, tx pgx.Tx, qrHash []byte) (lockedPass, error) {
	var pass lockedPass
	err := tx.QueryRow(ctx, `SELECT pass.id, pass.attendee_id, attendee.cycle_id, attendee.display_name, pass.status,
		application.status = 'accepted' AND application.decision_released_at IS NOT NULL AS eligible
		FROM ats.passes AS pass
		JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
		JOIN ats.applications AS application ON application.id = attendee.application_id
		WHERE pass.qr_token_hash = $1
		FOR UPDATE OF pass, attendee, application`, qrHash).Scan(
		&pass.id, &pass.attendeeID, &pass.cycleID, &pass.displayName, &pass.status, &pass.eligible,
	)
	return pass, err
}

func effectiveEntitlement(ctx context.Context, tx pgx.Tx, pass lockedPass, checkpoint lockedCheckpoint) (entitlements.Effective, error) {
	var override entitlements.Override
	err := tx.QueryRow(ctx, `SELECT allowed, max_redemptions FROM ats.attendee_entitlements
		WHERE attendee_id = $1 AND checkpoint_id = $2 AND cycle_id = $3 FOR UPDATE`, pass.attendeeID, checkpoint.id, pass.cycleID).Scan(&override.Allowed, &override.MaxRedemptions)
	if errors.Is(err, pgx.ErrNoRows) {
		return entitlements.Resolve(checkpoint.defaultAllowed, checkpoint.defaultMaxRedemptions, nil), nil
	}
	if err != nil {
		return entitlements.Effective{}, err
	}
	return entitlements.Resolve(checkpoint.defaultAllowed, checkpoint.defaultMaxRedemptions, &override), nil
}

func recordOutcome(ctx context.Context, tx pgx.Tx, record requestRecord) (Result, error) {
	var passID, attendeeID pgtype.UUID
	var attendeeName, passStatus pgtype.Text
	result := Result{Outcome: record.outcome, Checkpoint: Checkpoint{ID: record.checkpoint.id.String(), Name: record.checkpoint.name}}
	if record.pass != nil && record.outcome != OutcomeInvalidPass {
		passID, attendeeID = record.pass.id, record.pass.attendeeID
		attendeeName = pgtype.Text{String: record.pass.displayName, Valid: true}
		passStatus = pgtype.Text{String: record.pass.status, Valid: true}
		result.Attendee = &Attendee{DisplayName: record.pass.displayName}
		result.Pass = &Pass{Status: record.pass.status}
	}

	var redemptionID pgtype.UUID
	if record.outcome == OutcomeRedeemed {
		var err error
		redemptionID, err = randomUUID()
		if err != nil {
			return Result{}, err
		}
		value := redemptionID.String()
		result.RedemptionID = &value
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ats.redemption_requests (
		idempotency_key, scanner_user_id, checkpoint_id, qr_token_hash, pass_id, attendee_id, outcome,
		attendee_display_name, pass_status, checkpoint_name, redemption_id
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		record.idempotencyKey, record.scannerID, record.checkpoint.id, record.qrTokenHash, passID, attendeeID,
		record.outcome, attendeeName, passStatus, record.checkpoint.name, redemptionID,
	); err != nil {
		return Result{}, fmt.Errorf("insert redemption idempotency request: %w", err)
	}
	if record.outcome == OutcomeRedeemed {
		if _, err := tx.Exec(ctx, `INSERT INTO ats.redemptions (
			id, pass_id, attendee_id, checkpoint_id, cycle_id, ordinal, scanner_user_id, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, redemptionID, passID, attendeeID, record.checkpoint.id,
			record.checkpoint.cycleID, record.ordinal, record.scannerID, record.idempotencyKey); err != nil {
			return Result{}, fmt.Errorf("insert redemption: %w", err)
		}
	}
	metadata, err := json.Marshal(struct {
		ScannerUserID  string `json:"scannerUserId"`
		CheckpointID   string `json:"checkpointId"`
		AttendeeID     string `json:"attendeeId,omitempty"`
		PassID         string `json:"passId,omitempty"`
		RedemptionID   string `json:"redemptionId,omitempty"`
		IdempotencyKey string `json:"idempotencyKey"`
		Outcome        string `json:"outcome"`
	}{
		ScannerUserID: record.scannerID.String(), CheckpointID: record.checkpoint.id.String(),
		AttendeeID: attendeeID.String(), PassID: passID.String(), RedemptionID: redemptionID.String(),
		IdempotencyKey: record.idempotencyKey.String(), Outcome: record.outcome,
	})
	if err != nil {
		return Result{}, fmt.Errorf("encode redemption audit: %w", err)
	}
	subjectID := record.checkpoint.id
	if redemptionID.Valid {
		subjectID = redemptionID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
		VALUES ($1, 'redemption_recorded', 'redemption', $2, $3)`, record.scannerID, subjectID, metadata); err != nil {
		return Result{}, fmt.Errorf("insert redemption audit: %w", err)
	}
	return result, nil
}

func resultFromRequest(row redemptionRequestRow) Result {
	result := Result{Outcome: row.outcome, Checkpoint: Checkpoint{ID: row.checkpointID.String(), Name: row.checkpointName}}
	if row.passID.Valid && row.attendeeDisplayName.Valid && row.passStatus.Valid {
		result.Attendee = &Attendee{DisplayName: row.attendeeDisplayName.String}
		result.Pass = &Pass{Status: row.passStatus.String}
	}
	if row.redemptionID.Valid {
		value := row.redemptionID.String()
		result.RedemptionID = &value
	}
	return result
}

func parseUUID(value string) (pgtype.UUID, error) {
	var result pgtype.UUID
	if err := result.Scan(value); err != nil || !result.Valid {
		return pgtype.UUID{}, ErrInvalidInput
	}
	return result, nil
}

func randomUUID() (pgtype.UUID, error) {
	var result pgtype.UUID
	if _, err := rand.Read(result.Bytes[:]); err != nil {
		return pgtype.UUID{}, fmt.Errorf("generate redemption ID: %w", err)
	}
	result.Bytes[6] = (result.Bytes[6] & 0x0f) | 0x40
	result.Bytes[8] = (result.Bytes[8] & 0x3f) | 0x80
	result.Valid = true
	return result, nil
}

func isRetryableTransactionError(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && (databaseError.Code == "40001" || databaseError.Code == "23505")
}
