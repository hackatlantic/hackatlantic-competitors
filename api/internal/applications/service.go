package applications

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoCurrentForm           = errors.New("no current application form")
	ErrNotFound                = errors.New("application not found")
	ErrConflict                = errors.New("application state conflict")
	ErrApplicationWindowClosed = errors.New("application window is closed")
	ErrInvalidAnswers          = errors.New("invalid application answers")
	ErrIncomplete              = errors.New("incomplete application")
	ErrInvalidPublishedForm    = errors.New("invalid published application form")
)

// ConflictError exposes the current version without exposing another user's application.
type ConflictError struct {
	LockVersion int32
}

func (e *ConflictError) Error() string {
	return ErrConflict.Error()
}

func (e *ConflictError) Is(target error) bool {
	return target == ErrConflict
}

const (
	defaultQueryTimeout       = 5 * time.Second
	defaultTransactionTimeout = 15 * time.Second
)

// Applicant is the authenticated local user allowed to act on an application.
type Applicant struct {
	ID    string
	Email string
}

// Question is one scalar prompt in a published application form.
type Question struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Type     string  `json:"type"`
	Required bool    `json:"required"`
	Help     *string `json:"help,omitempty"`
}

// Form is the application form that is currently accepting responses.
type Form struct {
	ID             string     `json:"id"`
	CycleID        string     `json:"cycleId"`
	Version        int32      `json:"version"`
	ResumeRequired bool       `json:"resumeRequired"`
	Questions      []Question `json:"questions"`
}

// Application is the applicant-safe representation of an application record.
type Application struct {
	ID          string                     `json:"id"`
	CycleID     string                     `json:"cycleId"`
	FormID      string                     `json:"formId"`
	FormVersion int32                      `json:"formVersion"`
	Status      string                     `json:"status"`
	Answers     map[string]json.RawMessage `json:"answers"`
	LockVersion int32                      `json:"lockVersion"`
	SubmittedAt *time.Time                 `json:"submittedAt,omitempty"`
	CreatedAt   time.Time                  `json:"createdAt"`
	UpdatedAt   time.Time                  `json:"updatedAt"`
}

// SaveDraftInput replaces all draft answers when lock version matches.
type SaveDraftInput struct {
	ApplicationID string
	LockVersion   int32
	Answers       map[string]json.RawMessage
}

// SubmitInput transitions a complete draft when lock version matches.
type SubmitInput struct {
	ApplicationID string
	LockVersion   int32
}

// Service owns application form, draft, and submission operations.
type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

// NewService creates an application service with bounded database operations.
func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	return &Service{
		pool:               pool,
		queryTimeout:       queryTimeout,
		transactionTimeout: transactionTimeout,
	}
}

// CurrentForm returns the active, currently-open published application form.
func (s *Service) CurrentForm(ctx context.Context) (Form, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	row, err := sqlc.New(s.pool).GetCurrentApplicationForm(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrNoCurrentForm
	}
	if err != nil {
		return Form{}, fmt.Errorf("load current application form: %w", err)
	}
	return formFromRow(row)
}

// Create starts an empty draft against the current immutable published form.
func (s *Service) Create(ctx context.Context, applicant Applicant) (Application, error) {
	applicantID, err := parseUUID(applicant.ID)
	if err != nil {
		return Application{}, fmt.Errorf("parse applicant ID: %w", err)
	}
	form, err := s.CurrentForm(ctx)
	if err != nil {
		return Application{}, err
	}
	formID, err := parseUUID(form.ID)
	if err != nil {
		return Application{}, fmt.Errorf("parse current form ID: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	queries := sqlc.New(s.pool)
	applicationID, err := queries.CreateApplication(ctx, sqlc.CreateApplicationParams{
		ApplicantUserID: applicantID,
		FormID:          formID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNoCurrentForm
	}
	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}
	row, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return Application{}, err
	}
	return applicationFromDetail(row)
}

// List returns every application owned by the authenticated applicant.
func (s *Service) List(ctx context.Context, applicant Applicant) ([]Application, error) {
	applicantID, err := parseUUID(applicant.ID)
	if err != nil {
		return nil, fmt.Errorf("parse applicant ID: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	rows, err := sqlc.New(s.pool).ListApplicationsForApplicant(ctx, applicantID)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	items := make([]Application, 0, len(rows))
	for _, row := range rows {
		item, err := applicationFromList(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// SaveDraft validates and atomically replaces the complete answer map.
func (s *Service) SaveDraft(ctx context.Context, applicant Applicant, input SaveDraftInput) (Application, error) {
	applicantID, err := parseUUID(applicant.ID)
	if err != nil {
		return Application{}, fmt.Errorf("parse applicant ID: %w", err)
	}
	applicationID, err := parseApplicationID(input.ApplicationID)
	if err != nil {
		return Application{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()

	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Application{}, fmt.Errorf("begin application draft transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	before, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return Application{}, err
	}
	if before.Status != "draft" || before.LockVersion != input.LockVersion {
		return Application{}, &ConflictError{LockVersion: before.LockVersion}
	}
	form, err := parsePublishedForm(before.SchemaJson)
	if err != nil {
		return Application{}, err
	}
	if err := validateAnswers(form, input.Answers, false); err != nil {
		return Application{}, err
	}
	answersJSON, err := json.Marshal(input.Answers)
	if err != nil {
		return Application{}, fmt.Errorf("encode application answers: %w", err)
	}
	if _, err := queries.UpdateApplicationDraft(ctx, sqlc.UpdateApplicationDraftParams{
		ID:              applicationID,
		ApplicantUserID: applicantID,
		LockVersion:     input.LockVersion,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Application{}, currentConflict(ctx, queries, applicationID, applicantID)
	} else if err != nil {
		return Application{}, fmt.Errorf("advance application draft version: %w", err)
	}
	if err := queries.DeleteApplicationAnswers(ctx, applicationID); err != nil {
		return Application{}, fmt.Errorf("delete application answers: %w", err)
	}
	if err := queries.ReplaceApplicationAnswers(ctx, sqlc.ReplaceApplicationAnswersParams{
		ApplicationID: applicationID,
		AnswersJson:   answersJSON,
	}); err != nil {
		return Application{}, fmt.Errorf("replace application answers: %w", err)
	}
	row, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return Application{}, err
	}
	application, err := applicationFromDetail(row)
	if err != nil {
		return Application{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Application{}, fmt.Errorf("commit application draft transaction: %w", err)
	}
	return application, nil
}

// Submit validates required answers and atomically records the confirmation event.
func (s *Service) Submit(ctx context.Context, applicant Applicant, input SubmitInput) (Application, error) {
	applicantID, err := parseUUID(applicant.ID)
	if err != nil {
		return Application{}, fmt.Errorf("parse applicant ID: %w", err)
	}
	applicationID, err := parseApplicationID(input.ApplicationID)
	if err != nil {
		return Application{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()

	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Application{}, fmt.Errorf("begin application submission transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	before, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return Application{}, err
	}
	if before.Status == "submitted" {
		return applicationFromDetail(before)
	}
	if before.Status != "draft" || before.LockVersion != input.LockVersion {
		return Application{}, &ConflictError{LockVersion: before.LockVersion}
	}
	form, err := parsePublishedForm(before.SchemaJson)
	if err != nil {
		return Application{}, err
	}
	answers, err := answersFromJSON(before.AnswersJson)
	if err != nil {
		return Application{}, fmt.Errorf("decode stored application answers: %w", err)
	}
	if err := validateAnswers(form, answers, true); err != nil {
		return Application{}, err
	}
	if form.ResumeRequired {
		hasResume, err := queries.ApplicationHasResume(ctx, applicationID)
		if err != nil {
			return Application{}, fmt.Errorf("check required resume: %w", err)
		}
		if !hasResume {
			return Application{}, ErrIncomplete
		}
	}
	if _, err := queries.UpdateApplicationSubmission(ctx, sqlc.UpdateApplicationSubmissionParams{
		ID:                     applicationID,
		ApplicantUserID:        applicantID,
		LockVersion:            input.LockVersion,
		ApplicantEmailSnapshot: pgtype.Text{String: applicant.Email, Valid: true},
	}); errors.Is(err, pgx.ErrNoRows) {
		return Application{}, submissionConflict(ctx, queries, applicationID, applicantID, input.LockVersion)
	} else if err != nil {
		return Application{}, fmt.Errorf("submit application: %w", err)
	}
	templateData, err := json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
	}{ApplicationID: applicationID.String()})
	if err != nil {
		return Application{}, fmt.Errorf("encode submission confirmation: %w", err)
	}
	if err := queries.InsertSubmissionConfirmation(ctx, sqlc.InsertSubmissionConfirmationParams{
		RecipientUserID:  applicantID,
		RecipientEmail:   applicant.Email,
		TemplateDataJson: templateData,
		DedupeKey:        "submission_confirmation:" + applicationID.String(),
	}); err != nil {
		return Application{}, fmt.Errorf("queue submission confirmation: %w", err)
	}
	row, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return Application{}, err
	}
	application, err := applicationFromDetail(row)
	if err != nil {
		return Application{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Application{}, fmt.Errorf("commit application submission transaction: %w", err)
	}
	return application, nil
}

func applicationForApplicant(ctx context.Context, queries *sqlc.Queries, applicationID, applicantID pgtype.UUID) (sqlc.GetApplicationForApplicantRow, error) {
	row, err := queries.GetApplicationForApplicant(ctx, sqlc.GetApplicationForApplicantParams{
		ID:              applicationID,
		ApplicantUserID: applicantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.GetApplicationForApplicantRow{}, ErrNotFound
	}
	if err != nil {
		return sqlc.GetApplicationForApplicantRow{}, fmt.Errorf("load application: %w", err)
	}
	return row, nil
}

func currentConflict(ctx context.Context, queries *sqlc.Queries, applicationID, applicantID pgtype.UUID) error {
	row, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err == nil {
		return &ConflictError{LockVersion: row.LockVersion}
	}
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func submissionConflict(ctx context.Context, queries *sqlc.Queries, applicationID, applicantID pgtype.UUID, expectedLockVersion int32) error {
	row, err := applicationForApplicant(ctx, queries, applicationID, applicantID)
	if err != nil {
		return err
	}
	if row.Status != "draft" || row.LockVersion != expectedLockVersion {
		return &ConflictError{LockVersion: row.LockVersion}
	}
	windowOpen, err := queries.IsApplicationSubmissionWindowOpen(ctx, sqlc.IsApplicationSubmissionWindowOpenParams{
		ID:              applicationID,
		ApplicantUserID: applicantID,
	})
	if err != nil {
		return fmt.Errorf("check application submission window: %w", err)
	}
	if !windowOpen {
		return ErrApplicationWindowClosed
	}
	return &ConflictError{LockVersion: row.LockVersion}
}

func formFromRow(row sqlc.GetCurrentApplicationFormRow) (Form, error) {
	form, err := parsePublishedForm(row.SchemaJson)
	if err != nil {
		return Form{}, err
	}
	return Form{
		ID:             row.FormID.String(),
		CycleID:        row.CycleID.String(),
		Version:        row.FormVersion,
		ResumeRequired: form.ResumeRequired,
		Questions:      form.Questions,
	}, nil
}

func applicationFromDetail(row sqlc.GetApplicationForApplicantRow) (Application, error) {
	answers, err := answersFromJSON(row.AnswersJson)
	if err != nil {
		return Application{}, fmt.Errorf("decode stored application answers: %w", err)
	}
	return applicationFromValues(
		row.ID,
		row.CycleID,
		row.FormID,
		row.FormVersion,
		row.Status,
		answers,
		row.LockVersion,
		row.SubmittedAt,
		row.CreatedAt,
		row.UpdatedAt,
	), nil
}

func applicationFromList(row sqlc.ListApplicationsForApplicantRow) (Application, error) {
	answers, err := answersFromJSON(row.AnswersJson)
	if err != nil {
		return Application{}, fmt.Errorf("decode stored application answers: %w", err)
	}
	return applicationFromValues(
		row.ID,
		row.CycleID,
		row.FormID,
		row.FormVersion,
		row.Status,
		answers,
		row.LockVersion,
		row.SubmittedAt,
		row.CreatedAt,
		row.UpdatedAt,
	), nil
}

func applicationFromValues(
	id, cycleID, formID pgtype.UUID,
	formVersion int32,
	status string,
	answers map[string]json.RawMessage,
	lockVersion int32,
	submittedAt, createdAt, updatedAt pgtype.Timestamptz,
) Application {
	return Application{
		ID:          id.String(),
		CycleID:     cycleID.String(),
		FormID:      formID.String(),
		FormVersion: formVersion,
		Status:      status,
		Answers:     answers,
		LockVersion: lockVersion,
		SubmittedAt: nullableTime(submittedAt),
		CreatedAt:   createdAt.Time.UTC(),
		UpdatedAt:   updatedAt.Time.UTC(),
	}
}

type publishedForm struct {
	ResumeRequired bool
	Questions      []Question
}

func parsePublishedForm(raw []byte) (publishedForm, error) {
	type questionDocument struct {
		Key      *string `json:"key"`
		Label    *string `json:"label"`
		Type     *string `json:"type"`
		Required *bool   `json:"required"`
		Help     *string `json:"help"`
	}
	type document struct {
		ResumeRequired bool                `json:"resumeRequired"`
		Questions      *[]questionDocument `json:"questions"`
	}

	var parsed document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return publishedForm{}, ErrInvalidPublishedForm
	}
	if err := consumeJSON(decoder); err != nil || parsed.Questions == nil || len(*parsed.Questions) == 0 {
		return publishedForm{}, ErrInvalidPublishedForm
	}
	questions := make([]Question, 0, len(*parsed.Questions))
	keys := make(map[string]struct{}, len(*parsed.Questions))
	for _, question := range *parsed.Questions {
		if question.Key == nil || question.Label == nil || question.Type == nil || question.Required == nil ||
			*question.Key == "" || strings.TrimSpace(*question.Key) != *question.Key || strings.TrimSpace(*question.Label) == "" {
			return publishedForm{}, ErrInvalidPublishedForm
		}
		if *question.Type != "string" && *question.Type != "number" && *question.Type != "boolean" {
			return publishedForm{}, ErrInvalidPublishedForm
		}
		if _, exists := keys[*question.Key]; exists {
			return publishedForm{}, ErrInvalidPublishedForm
		}
		keys[*question.Key] = struct{}{}
		questions = append(questions, Question{
			Key:      *question.Key,
			Label:    *question.Label,
			Type:     *question.Type,
			Required: *question.Required,
			Help:     question.Help,
		})
	}
	return publishedForm{ResumeRequired: parsed.ResumeRequired, Questions: questions}, nil
}

func validateAnswers(form publishedForm, answers map[string]json.RawMessage, requireRequired bool) error {
	questions := form.Questions
	if answers == nil {
		return ErrInvalidAnswers
	}
	byKey := make(map[string]Question, len(questions))
	for _, question := range questions {
		byKey[question.Key] = question
	}
	nonemptyStrings := make(map[string]bool, len(answers))
	for key, rawValue := range answers {
		question, exists := byKey[key]
		if !exists {
			return ErrInvalidAnswers
		}
		value, err := decodeScalar(rawValue)
		if err != nil {
			return ErrInvalidAnswers
		}
		switch question.Type {
		case "string":
			stringValue, ok := value.(string)
			if !ok {
				return ErrInvalidAnswers
			}
			nonemptyStrings[key] = strings.TrimSpace(stringValue) != ""
		case "number":
			if _, ok := value.(json.Number); !ok {
				return ErrInvalidAnswers
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return ErrInvalidAnswers
			}
		}
	}
	if requireRequired {
		for _, question := range questions {
			if !question.Required {
				continue
			}
			if _, exists := answers[question.Key]; !exists ||
				(question.Type == "string" && !nonemptyStrings[question.Key]) {
				return ErrIncomplete
			}
		}
	}
	return nil
}

func decodeScalar(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := consumeJSON(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func consumeJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func answersFromJSON(raw any) (map[string]json.RawMessage, error) {
	var encoded []byte
	switch value := raw.(type) {
	case string:
		encoded = []byte(value)
	case []byte:
		encoded = value
	default:
		return nil, fmt.Errorf("unexpected JSON storage type %T", raw)
	}
	answers := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &answers); err != nil {
		return nil, err
	}
	if answers == nil {
		return nil, errors.New("answers must be a JSON object")
	}
	return answers, nil
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
