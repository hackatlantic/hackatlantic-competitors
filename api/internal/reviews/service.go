// Package reviews owns organizer and reviewer workflows for submitted applications.
package reviews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden        = errors.New("staff access is forbidden")
	ErrNotFound         = errors.New("application not found")
	ErrInvalidFilter    = errors.New("invalid application filter")
	ErrInvalidReview    = errors.New("invalid review")
	ErrInvalidReviewer  = errors.New("target user is not a reviewer")
	ErrNotSubmitted     = errors.New("application is not submitted")
	ErrReviewNotStarted = errors.New("review draft does not exist")
	ErrReviewConflict   = errors.New("review state conflict")
)

const (
	defaultQueryTimeout       = 5 * time.Second
	defaultTransactionTimeout = 15 * time.Second
)

// ConflictError exposes the current review version without exposing another reviewer's work.
type ConflictError struct {
	LockVersion int32
}

func (e *ConflictError) Error() string { return ErrReviewConflict.Error() }

func (e *ConflictError) Is(target error) bool { return target == ErrReviewConflict }

// Applicant is the organizer/reviewer projection of the application owner.
type Applicant struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName"`
}

// Application is an internal staff projection and is never used by applicant endpoints.
type Application struct {
	ID          string                     `json:"id"`
	CycleID     string                     `json:"cycleId"`
	FormID      string                     `json:"formId"`
	FormVersion int32                      `json:"formVersion"`
	Status      string                     `json:"status"`
	SubmittedAt *time.Time                 `json:"submittedAt,omitempty"`
	Applicant   Applicant                  `json:"applicant"`
	Answers     map[string]json.RawMessage `json:"answers"`
	CreatedAt   time.Time                  `json:"createdAt"`
	UpdatedAt   time.Time                  `json:"updatedAt"`
}

// Assignment marks a reviewer's organizer-assigned queue metadata. It does not gate reviewer access.
type Assignment struct {
	AssignedBy string    `json:"assignedBy"`
	AssignedAt time.Time `json:"assignedAt"`
}

// Review is the authenticated reviewer's private review of an application.
type Review struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Score          int32      `json:"score"`
	Recommendation string     `json:"recommendation"`
	InternalNotes  *string    `json:"internalNotes,omitempty"`
	LockVersion    int32      `json:"lockVersion"`
	SubmittedAt    *time.Time `json:"submittedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// ReviewerApplication combines a submitted staff projection with only the caller's review and queue metadata.
type ReviewerApplication struct {
	Application
	Assignment *Assignment `json:"assignment,omitempty"`
	Review     *Review     `json:"review,omitempty"`
}

// ListFilter controls server-side organizer filtering.
type ListFilter struct {
	Status string
	Search string
}

// SaveDraftInput is a complete review draft replacement guarded by lockVersion.
type SaveDraftInput struct {
	ApplicationID  string
	LockVersion    int32
	Score          int32
	Recommendation string
	InternalNotes  *string
}

// SubmitInput submits a previously saved draft guarded by lockVersion.
type SubmitInput struct {
	ApplicationID string
	LockVersion   int32
}

// Service authorizes and persists organizer/reviewer operations.
type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

// NewService creates a bounded workflow service.
func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	return &Service{pool: pool, queryTimeout: queryTimeout, transactionTimeout: transactionTimeout}
}

// ListOrganizerApplications returns an organizer-authorized, server-filtered internal projection.
func (s *Service) ListOrganizerApplications(ctx context.Context, actor users.User, filter ListFilter) ([]Application, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return nil, ErrForbidden
	}
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && !validApplicationStatus(filter.Status) {
		return nil, ErrInvalidFilter
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := sqlc.New(s.pool).ListOrganizerApplications(ctx, sqlc.ListOrganizerApplicationsParams{
		Status: filter.Status,
		Search: filter.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list organizer applications: %w", err)
	}
	items := make([]Application, 0, len(rows))
	for _, row := range rows {
		application, err := applicationFromOrganizerListRow(row)
		if err != nil {
			return nil, err
		}
		items = append(items, application)
	}
	return items, nil
}

// GetOrganizerApplication returns an organizer-authorized internal application projection.
func (s *Service) GetOrganizerApplication(ctx context.Context, actor users.User, applicationID string) (Application, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Application{}, ErrForbidden
	}
	id, err := parseApplicationID(applicationID)
	if err != nil {
		return Application{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	row, err := sqlc.New(s.pool).GetOrganizerApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, fmt.Errorf("load organizer application: %w", err)
	}
	return applicationFromOrganizerRow(row)
}

// GrantReviewerRole adds the reviewer role once and writes its audit record in the same transaction.
func (s *Service) GrantReviewerRole(ctx context.Context, actor users.User, targetUserID string) error {
	if !actor.HasRole(users.RoleOrganizer) || actor.ID == targetUserID {
		return ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return fmt.Errorf("parse organizer ID: %w", err)
	}
	targetID, err := parseUUID(targetUserID)
	if err != nil {
		return ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin reviewer role transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)
	exists, err := queries.UserExists(ctx, targetID)
	if err != nil {
		return fmt.Errorf("check reviewer role target: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	assigned, err := queries.GrantReviewerRole(ctx, sqlc.GrantReviewerRoleParams{UserID: targetID, CreatedBy: actorID})
	if err != nil {
		return fmt.Errorf("grant reviewer role: %w", err)
	}
	if assigned {
		if err := queries.InsertReviewerRoleAudit(ctx, sqlc.InsertReviewerRoleAuditParams{ActorUserID: actorID, SubjectID: targetID}); err != nil {
			return fmt.Errorf("audit reviewer role assignment: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit reviewer role transaction: %w", err)
	}
	return nil
}

// AssignReviewer records organizer queue metadata for a submitted application and audits a new assignment atomically.
func (s *Service) AssignReviewer(ctx context.Context, actor users.User, applicationID, reviewerUserID string) error {
	if !actor.HasRole(users.RoleOrganizer) {
		return ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return fmt.Errorf("parse organizer ID: %w", err)
	}
	applicationUUID, err := parseApplicationID(applicationID)
	if err != nil {
		return err
	}
	reviewerID, err := parseUUID(reviewerUserID)
	if err != nil {
		return ErrInvalidReviewer
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin reviewer assignment transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)
	application, err := queries.GetOrganizerApplication(ctx, applicationUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load assignment application: %w", err)
	}
	if application.Status != "submitted" {
		return ErrNotSubmitted
	}
	reviewer, err := queries.UserHasReviewerRole(ctx, reviewerID)
	if err != nil {
		return fmt.Errorf("check reviewer role: %w", err)
	}
	if !reviewer {
		return ErrInvalidReviewer
	}
	assigned, err := queries.AssignReviewer(ctx, sqlc.AssignReviewerParams{
		ApplicationID:  applicationUUID,
		ReviewerUserID: reviewerID,
		AssignedBy:     actorID,
	})
	if err != nil {
		return fmt.Errorf("assign reviewer: %w", err)
	}
	if assigned {
		if err := queries.InsertReviewAssignmentAudit(ctx, sqlc.InsertReviewAssignmentAuditParams{
			ActorUserID: actorID, SubjectID: applicationUUID, ReviewerUserID: reviewerUserID,
		}); err != nil {
			return fmt.Errorf("audit reviewer assignment: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit reviewer assignment transaction: %w", err)
	}
	return nil
}

// ListReviewerApplications returns every submitted application. Assignments only order and annotate the queue.
func (s *Service) ListReviewerApplications(ctx context.Context, actor users.User) ([]ReviewerApplication, error) {
	if !actor.HasRole(users.RoleReviewer) {
		return nil, ErrForbidden
	}
	reviewerID, err := parseUUID(actor.ID)
	if err != nil {
		return nil, fmt.Errorf("parse reviewer ID: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := sqlc.New(s.pool).ListReviewerApplications(ctx, reviewerID)
	if err != nil {
		return nil, fmt.Errorf("list reviewer applications: %w", err)
	}
	items := make([]ReviewerApplication, 0, len(rows))
	for _, row := range rows {
		application, err := reviewerApplicationFromListRow(row)
		if err != nil {
			return nil, err
		}
		items = append(items, application)
	}
	return items, nil
}

// GetReviewerApplication returns a submitted application and only the caller's review.
func (s *Service) GetReviewerApplication(ctx context.Context, actor users.User, applicationID string) (ReviewerApplication, error) {
	if !actor.HasRole(users.RoleReviewer) {
		return ReviewerApplication{}, ErrForbidden
	}
	id, err := parseApplicationID(applicationID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	reviewerID, err := parseUUID(actor.ID)
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("parse reviewer ID: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.reviewerApplication(ctx, sqlc.New(s.pool), id, reviewerID)
}

// SaveDraft atomically creates or updates a caller-owned review draft when its lockVersion matches.
func (s *Service) SaveDraft(ctx context.Context, actor users.User, input SaveDraftInput) (ReviewerApplication, error) {
	if !actor.HasRole(users.RoleReviewer) {
		return ReviewerApplication{}, ErrForbidden
	}
	if input.LockVersion < 0 || input.Score < 1 || input.Score > 5 || !validRecommendation(input.Recommendation) {
		return ReviewerApplication{}, ErrInvalidReview
	}
	applicationID, err := parseApplicationID(input.ApplicationID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	reviewerID, err := parseUUID(actor.ID)
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("parse reviewer ID: %w", err)
	}
	scoreJSON, err := json.Marshal(struct {
		Score int32 `json:"score"`
	}{Score: input.Score})
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("encode review score: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("begin review draft transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)
	before, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if before.Review != nil {
		if before.Review.Status != "draft" || before.Review.LockVersion != input.LockVersion {
			return ReviewerApplication{}, &ConflictError{LockVersion: before.Review.LockVersion}
		}
	} else if input.LockVersion != 0 {
		return ReviewerApplication{}, &ConflictError{LockVersion: 0}
	}
	internalNotes := ""
	if input.InternalNotes != nil {
		internalNotes = *input.InternalNotes
	}
	if _, err := queries.UpdateReviewDraft(ctx, sqlc.UpdateReviewDraftParams{
		ApplicationID: applicationID, ReviewerUserID: reviewerID, ScoreJson: scoreJSON,
		Recommendation: input.Recommendation, InternalNotes: internalNotes, LockVersion: input.LockVersion,
	}); errors.Is(err, pgx.ErrNoRows) {
		return ReviewerApplication{}, s.reviewConflict(ctx, queries, applicationID, reviewerID)
	} else if err != nil {
		return ReviewerApplication{}, fmt.Errorf("save review draft: %w", err)
	}
	updated, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return ReviewerApplication{}, fmt.Errorf("commit review draft transaction: %w", err)
	}
	return updated, nil
}

// Submit transitions a matching draft to submitted and records the immutable transition in the audit log.
func (s *Service) Submit(ctx context.Context, actor users.User, input SubmitInput) (ReviewerApplication, error) {
	if !actor.HasRole(users.RoleReviewer) {
		return ReviewerApplication{}, ErrForbidden
	}
	if input.LockVersion < 0 {
		return ReviewerApplication{}, ErrInvalidReview
	}
	applicationID, err := parseApplicationID(input.ApplicationID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	reviewerID, err := parseUUID(actor.ID)
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("parse reviewer ID: %w", err)
	}
	actorID := reviewerID
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("begin review submission transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)
	before, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if before.Review == nil {
		return ReviewerApplication{}, ErrReviewNotStarted
	}
	if before.Review.Status != "draft" {
		return ReviewerApplication{}, &ConflictError{LockVersion: before.Review.LockVersion}
	}
	if before.Review.LockVersion != input.LockVersion {
		return ReviewerApplication{}, &ConflictError{LockVersion: before.Review.LockVersion}
	}
	reviewID, err := queries.SubmitReview(ctx, sqlc.SubmitReviewParams{
		ApplicationID: applicationID, ReviewerUserID: reviewerID, LockVersion: input.LockVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.submissionConflict(ctx, queries, applicationID, reviewerID)
	}
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("submit review: %w", err)
	}
	if err := queries.InsertReviewSubmissionAudit(ctx, sqlc.InsertReviewSubmissionAuditParams{
		ActorUserID: actorID, SubjectID: reviewID, ApplicationID: input.ApplicationID,
	}); err != nil {
		return ReviewerApplication{}, fmt.Errorf("audit review submission: %w", err)
	}
	updated, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return ReviewerApplication{}, fmt.Errorf("commit review submission transaction: %w", err)
	}
	return updated, nil
}

func (s *Service) reviewerApplication(ctx context.Context, queries *sqlc.Queries, applicationID, reviewerID pgtype.UUID) (ReviewerApplication, error) {
	row, err := queries.GetReviewerApplication(ctx, sqlc.GetReviewerApplicationParams{ID: applicationID, ReviewerUserID: reviewerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewerApplication{}, ErrNotFound
	}
	if err != nil {
		return ReviewerApplication{}, fmt.Errorf("load reviewer application: %w", err)
	}
	return reviewerApplicationFromRow(row)
}

func (s *Service) reviewConflict(ctx context.Context, queries *sqlc.Queries, applicationID, reviewerID pgtype.UUID) error {
	current, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return err
	}
	if current.Review == nil {
		return &ConflictError{LockVersion: 0}
	}
	return &ConflictError{LockVersion: current.Review.LockVersion}
}

func (s *Service) submissionConflict(ctx context.Context, queries *sqlc.Queries, applicationID, reviewerID pgtype.UUID) (ReviewerApplication, error) {
	current, err := s.reviewerApplication(ctx, queries, applicationID, reviewerID)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if current.Review == nil {
		return ReviewerApplication{}, ErrReviewNotStarted
	}
	return ReviewerApplication{}, &ConflictError{LockVersion: current.Review.LockVersion}
}

func applicationFromOrganizerRow(row sqlc.GetOrganizerApplicationRow) (Application, error) {
	return applicationFromValues(row.ID, row.CycleID, row.FormID, row.FormVersion, row.Status, row.SubmittedAt, row.ApplicantID, row.ApplicantEmail, row.ApplicantDisplayName, row.AnswersJson, row.CreatedAt, row.UpdatedAt)
}

func applicationFromOrganizerListRow(row sqlc.ListOrganizerApplicationsRow) (Application, error) {
	return applicationFromValues(row.ID, row.CycleID, row.FormID, row.FormVersion, row.Status, row.SubmittedAt, row.ApplicantID, row.ApplicantEmail, row.ApplicantDisplayName, row.AnswersJson, row.CreatedAt, row.UpdatedAt)
}

func reviewerApplicationFromRow(row sqlc.GetReviewerApplicationRow) (ReviewerApplication, error) {
	application, err := applicationFromValues(row.ID, row.CycleID, row.FormID, row.FormVersion, row.Status, row.SubmittedAt, row.ApplicantID, row.ApplicantEmail, row.ApplicantDisplayName, row.AnswersJson, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return ReviewerApplication{}, err
	}
	return reviewerApplicationFromValues(application, row.AssignedBy, row.AssignedAt, row.ReviewID, row.ReviewStatus, row.ReviewScoreJson, row.ReviewRecommendation, row.ReviewInternalNotes, row.ReviewLockVersion, row.ReviewSubmittedAt, row.ReviewCreatedAt, row.ReviewUpdatedAt)
}

func reviewerApplicationFromListRow(row sqlc.ListReviewerApplicationsRow) (ReviewerApplication, error) {
	application, err := applicationFromValues(row.ID, row.CycleID, row.FormID, row.FormVersion, row.Status, row.SubmittedAt, row.ApplicantID, row.ApplicantEmail, row.ApplicantDisplayName, row.AnswersJson, row.CreatedAt, row.UpdatedAt)
	if err != nil {
		return ReviewerApplication{}, err
	}
	return reviewerApplicationFromValues(application, row.AssignedBy, row.AssignedAt, row.ReviewID, row.ReviewStatus, row.ReviewScoreJson, row.ReviewRecommendation, row.ReviewInternalNotes, row.ReviewLockVersion, row.ReviewSubmittedAt, row.ReviewCreatedAt, row.ReviewUpdatedAt)
}

func applicationFromValues(id, cycleID, formID pgtype.UUID, formVersion int32, status string, submittedAt pgtype.Timestamptz, applicantID pgtype.UUID, applicantEmail string, applicantDisplayName pgtype.Text, answersJSON string, createdAt, updatedAt pgtype.Timestamptz) (Application, error) {
	answers := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(answersJSON), &answers); err != nil || answers == nil {
		return Application{}, fmt.Errorf("decode staff application answers: %w", err)
	}
	var displayName *string
	if applicantDisplayName.Valid {
		displayName = &applicantDisplayName.String
	}
	return Application{
		ID: id.String(), CycleID: cycleID.String(), FormID: formID.String(), FormVersion: formVersion,
		Status: status, SubmittedAt: nullableTime(submittedAt),
		Applicant: Applicant{ID: applicantID.String(), Email: applicantEmail, DisplayName: displayName},
		Answers:   answers, CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}, nil
}

func reviewerApplicationFromValues(application Application, assignedBy pgtype.UUID, assignedAt pgtype.Timestamptz, reviewID pgtype.UUID, reviewStatus pgtype.Text, reviewScoreJSON string, reviewRecommendation, reviewInternalNotes pgtype.Text, reviewLockVersion pgtype.Int4, reviewSubmittedAt, reviewCreatedAt, reviewUpdatedAt pgtype.Timestamptz) (ReviewerApplication, error) {
	result := ReviewerApplication{Application: application}
	if assignedAt.Valid {
		result.Assignment = &Assignment{AssignedBy: assignedBy.String(), AssignedAt: assignedAt.Time.UTC()}
	}
	if !reviewID.Valid {
		return result, nil
	}
	score, err := decodeScore(reviewScoreJSON)
	if err != nil {
		return ReviewerApplication{}, err
	}
	if !reviewStatus.Valid || !reviewRecommendation.Valid || !reviewLockVersion.Valid || !reviewCreatedAt.Valid || !reviewUpdatedAt.Valid {
		return ReviewerApplication{}, errors.New("invalid persisted review")
	}
	var internalNotes *string
	if reviewInternalNotes.Valid {
		internalNotes = &reviewInternalNotes.String
	}
	result.Review = &Review{
		ID: reviewID.String(), Status: reviewStatus.String, Score: score, Recommendation: reviewRecommendation.String,
		InternalNotes: internalNotes, LockVersion: reviewLockVersion.Int32, SubmittedAt: nullableTime(reviewSubmittedAt),
		CreatedAt: reviewCreatedAt.Time.UTC(), UpdatedAt: reviewUpdatedAt.Time.UTC(),
	}
	return result, nil
}

func decodeScore(raw string) (int32, error) {
	var document struct {
		Score *int32 `json:"score"`
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.Score == nil || *document.Score < 1 || *document.Score > 5 {
		return 0, errors.New("invalid persisted review score")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return 0, errors.New("invalid persisted review score")
	}
	return *document.Score, nil
}

func validApplicationStatus(status string) bool {
	switch status {
	case "submitted", "accepted", "waitlisted", "rejected":
		return true
	default:
		return false
	}
}

func validRecommendation(value string) bool {
	switch value {
	case "strong_yes", "yes", "neutral", "no", "strong_no":
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

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}
