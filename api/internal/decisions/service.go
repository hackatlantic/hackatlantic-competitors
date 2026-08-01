// Package decisions owns organizer decision recording, release, and applicant-safe reads.
package decisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/attendees"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden         = errors.New("organizer access is forbidden")
	ErrNotFound          = errors.New("application or decision not found")
	ErrNotSubmitted      = errors.New("application is not submitted")
	ErrInvalidOutcome    = errors.New("invalid decision outcome")
	ErrNoCurrentDecision = errors.New("application has no current decision")
)

const (
	defaultQueryTimeout       = 5 * time.Second
	defaultTransactionTimeout = 15 * time.Second
)

// Decision is an organizer-only decision projection. Applicant handlers never return it.
type Decision struct {
	ID             string     `json:"id"`
	ApplicationID  string     `json:"applicationId"`
	Outcome        string     `json:"outcome"`
	InternalReason *string    `json:"internalReason,omitempty"`
	DecidedBy      string     `json:"decidedBy"`
	DecidedAt      time.Time  `json:"decidedAt"`
	SupersedesID   *string    `json:"supersedesId,omitempty"`
	ReleasedBy     *string    `json:"releasedBy,omitempty"`
	ReleasedAt     *time.Time `json:"releasedAt,omitempty"`
}

// ApplicantDecision intentionally exposes only an already-released outcome.
type ApplicantDecision struct {
	ApplicationID string    `json:"applicationId"`
	Outcome       string    `json:"outcome"`
	ReleasedAt    time.Time `json:"releasedAt"`
}

// RecordInput is the organizer-controlled internal decision payload.
type RecordInput struct {
	ApplicationID  string
	Outcome        string
	InternalReason *string
}

// Service owns all decision lifecycle database transactions.
type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

// NewService creates a bounded decision lifecycle service.
func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	return &Service{pool: pool, queryTimeout: queryTimeout, transactionTimeout: transactionTimeout}
}

// Record appends a decision, updates the current application cache, and converts accepted applicants atomically.
func (s *Service) Record(ctx context.Context, actor users.User, input RecordInput) (Decision, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Decision{}, ErrForbidden
	}
	if !validOutcome(input.Outcome) {
		return Decision{}, ErrInvalidOutcome
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Decision{}, fmt.Errorf("parse organizer ID: %w", err)
	}
	applicationID, err := parseApplicationID(input.ApplicationID)
	if err != nil {
		return Decision{}, err
	}
	internalReason := optionalText(input.InternalReason)

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Decision{}, fmt.Errorf("begin decision record transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	application, err := queries.LockApplicationForDecision(ctx, applicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, ErrNotFound
	}
	if err != nil {
		return Decision{}, fmt.Errorf("lock decision application: %w", err)
	}
	if !decisionEligibleStatus(application.Status) {
		return Decision{}, ErrNotSubmitted
	}

	prior, err := queries.GetLatestDecisionForApplication(ctx, applicationID)
	var supersedesID pgtype.UUID
	switch {
	case err == nil:
		supersedesID = prior.ID
	case errors.Is(err, pgx.ErrNoRows):
		// The first decision has no predecessor.
	default:
		return Decision{}, fmt.Errorf("load current decision: %w", err)
	}

	recorded, err := queries.InsertDecision(ctx, sqlc.InsertDecisionParams{
		ApplicationID:  applicationID,
		Outcome:        input.Outcome,
		InternalReason: internalReason,
		DecidedBy:      actorID,
		SupersedesID:   supersedesID,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("insert decision: %w", err)
	}
	if err := queries.UpdateApplicationDecisionCache(ctx, sqlc.UpdateApplicationDecisionCacheParams{
		ID: applicationID, Status: input.Outcome, CurrentDecisionID: recorded.ID,
	}); err != nil {
		return Decision{}, fmt.Errorf("update application decision cache: %w", err)
	}

	if input.Outcome == "accepted" {
		attendee, err := queries.UpsertAcceptedAttendee(ctx, sqlc.UpsertAcceptedAttendeeParams{
			CycleID:       application.CycleID,
			ApplicationID: applicationID,
			UserID:        application.ApplicantUserID,
			DisplayName:   application.ApplicantDisplayName,
			Email:         application.ApplicantEmail,
		})
		if err != nil {
			return Decision{}, fmt.Errorf("upsert accepted attendee: %w", err)
		}
		if err := queries.SeedAttendeeHackerRole(ctx, attendee.ID); err != nil {
			return Decision{}, fmt.Errorf("seed accepted attendee %s role: %w", attendees.RoleHacker, err)
		}
		if attendee.AttendeeCreated {
			if err := queries.InsertAttendeeCreatedAudit(ctx, sqlc.InsertAttendeeCreatedAuditParams{
				ActorUserID: actorID, SubjectID: attendee.ID, ApplicationID: applicationID.String(),
			}); err != nil {
				return Decision{}, fmt.Errorf("audit attendee creation: %w", err)
			}
		}
	}

	if err := queries.InsertDecisionRecordedAudit(ctx, sqlc.InsertDecisionRecordedAuditParams{
		ActorUserID: actorID, SubjectID: recorded.ID, ApplicationID: applicationID.String(), Outcome: input.Outcome,
	}); err != nil {
		return Decision{}, fmt.Errorf("audit decision record: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return Decision{}, fmt.Errorf("commit decision record transaction: %w", err)
	}
	return decisionFromRow(recorded), nil
}

// Release marks the current decision applicant-visible and creates one matching outbox event atomically.
func (s *Service) Release(ctx context.Context, actor users.User, decisionID string) (Decision, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Decision{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Decision{}, fmt.Errorf("parse organizer ID: %w", err)
	}
	id, err := parseDecisionID(decisionID)
	if err != nil {
		return Decision{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Decision{}, fmt.Errorf("begin decision release transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	decision, err := queries.LockCurrentDecisionForRelease(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, ErrNotFound
	}
	if err != nil {
		return Decision{}, fmt.Errorf("lock current decision for release: %w", err)
	}
	if decision.ReleasedAt.Valid {
		if err := transaction.Commit(ctx); err != nil {
			return Decision{}, fmt.Errorf("commit repeated decision release transaction: %w", err)
		}
		return decisionFromReleaseRow(decision), nil
	}

	releasedAt, err := queries.ReleaseDecision(ctx, sqlc.ReleaseDecisionParams{ID: id, ReleasedBy: actorID})
	if err != nil {
		return Decision{}, fmt.Errorf("release decision: %w", err)
	}
	if err := queries.UpdateApplicationDecisionRelease(ctx, sqlc.UpdateApplicationDecisionReleaseParams{
		ID: decision.ApplicationID, DecisionReleasedAt: releasedAt,
	}); err != nil {
		return Decision{}, fmt.Errorf("update application release cache: %w", err)
	}
	templateData, err := json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		Outcome       string `json:"outcome"`
	}{ApplicationID: decision.ApplicationID.String(), Outcome: decision.Outcome})
	if err != nil {
		return Decision{}, fmt.Errorf("encode decision release email: %w", err)
	}
	if err := queries.InsertDecisionReleaseEmail(ctx, sqlc.InsertDecisionReleaseEmailParams{
		RecipientUserID:  decision.ApplicantUserID,
		RecipientEmail:   decision.ApplicantEmail,
		TemplateDataJson: templateData,
		DedupeKey:        "decision_release:" + decision.ID.String(),
	}); err != nil {
		return Decision{}, fmt.Errorf("queue decision release email: %w", err)
	}
	if err := queries.InsertDecisionReleasedAudit(ctx, sqlc.InsertDecisionReleasedAuditParams{
		ActorUserID: actorID, SubjectID: decision.ID, ApplicationID: decision.ApplicationID.String(), Outcome: decision.Outcome,
	}); err != nil {
		return Decision{}, fmt.Errorf("audit decision release: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return Decision{}, fmt.Errorf("commit decision release transaction: %w", err)
	}

	decision.ReleasedBy = actorID
	decision.ReleasedAt = releasedAt
	return decisionFromReleaseRow(decision), nil
}

// CurrentForOrganizer returns the latest internal decision for the organizer detail projection.
func (s *Service) CurrentForOrganizer(ctx context.Context, actor users.User, applicationID string) (Decision, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Decision{}, ErrForbidden
	}
	id, err := parseApplicationID(applicationID)
	if err != nil {
		return Decision{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	decision, err := sqlc.New(s.pool).GetLatestDecisionForApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, ErrNoCurrentDecision
	}
	if err != nil {
		return Decision{}, fmt.Errorf("load current decision: %w", err)
	}
	return decisionFromRow(decision), nil
}

// GetReleasedForApplicant returns an applicant-safe outcome only once its current decision is released.
func (s *Service) GetReleasedForApplicant(ctx context.Context, actor users.User, applicationID string) (ApplicantDecision, error) {
	if !actor.HasRole(users.RoleApplicant) {
		return ApplicantDecision{}, ErrForbidden
	}
	applicantID, err := parseUUID(actor.ID)
	if err != nil {
		return ApplicantDecision{}, fmt.Errorf("parse applicant ID: %w", err)
	}
	id, err := parseApplicationID(applicationID)
	if err != nil {
		return ApplicantDecision{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	decision, err := sqlc.New(s.pool).GetReleasedDecisionForApplicant(ctx, sqlc.GetReleasedDecisionForApplicantParams{
		ID: id, ApplicantUserID: applicantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicantDecision{}, ErrNotFound
	}
	if err != nil {
		return ApplicantDecision{}, fmt.Errorf("load released applicant decision: %w", err)
	}
	if !decision.ReleasedAt.Valid {
		return ApplicantDecision{}, fmt.Errorf("released decision has no release timestamp")
	}
	return ApplicantDecision{
		ApplicationID: decision.ApplicationID.String(),
		Outcome:       decision.Outcome,
		ReleasedAt:    decision.ReleasedAt.Time.UTC(),
	}, nil
}

func decisionFromRow(row sqlc.AtsDecision) Decision {
	return Decision{
		ID:             row.ID.String(),
		ApplicationID:  row.ApplicationID.String(),
		Outcome:        row.Outcome,
		InternalReason: optionalString(row.InternalReason),
		DecidedBy:      row.DecidedBy.String(),
		DecidedAt:      row.DecidedAt.Time.UTC(),
		SupersedesID:   optionalUUID(row.SupersedesID),
		ReleasedBy:     optionalUUID(row.ReleasedBy),
		ReleasedAt:     optionalTime(row.ReleasedAt),
	}
}

func decisionFromReleaseRow(row sqlc.LockCurrentDecisionForReleaseRow) Decision {
	return Decision{
		ID:             row.ID.String(),
		ApplicationID:  row.ApplicationID.String(),
		Outcome:        row.Outcome,
		InternalReason: optionalString(row.InternalReason),
		DecidedBy:      row.DecidedBy.String(),
		DecidedAt:      row.DecidedAt.Time.UTC(),
		SupersedesID:   optionalUUID(row.SupersedesID),
		ReleasedBy:     optionalUUID(row.ReleasedBy),
		ReleasedAt:     optionalTime(row.ReleasedAt),
	}
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: normalized, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalUUID(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	result := value.String()
	return &result
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func validOutcome(value string) bool {
	switch value {
	case "accepted", "waitlisted", "rejected":
		return true
	default:
		return false
	}
}

func decisionEligibleStatus(value string) bool {
	switch value {
	case "submitted", "accepted", "waitlisted", "rejected":
		return true
	default:
		return false
	}
}

func parseApplicationID(value string) (pgtype.UUID, error) {
	id, err := parseUUID(value)
	if err != nil {
		return pgtype.UUID{}, ErrNotFound
	}
	return id, nil
}

func parseDecisionID(value string) (pgtype.UUID, error) {
	id, err := parseUUID(value)
	if err != nil {
		return pgtype.UUID{}, ErrNotFound
	}
	return id, nil
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
