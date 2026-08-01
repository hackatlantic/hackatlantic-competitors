// Package resumes owns private PDF resume validation, storage, and access.
package resumes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxPDFBytes int64 = 5 * 1024 * 1024

var (
	ErrForbidden  = errors.New("resume access forbidden")
	ErrNotFound   = errors.New("resume not found")
	ErrInvalidPDF = errors.New("resume must be a valid PDF")
)

type Store interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type Metadata struct {
	ApplicationID    string    `json:"applicationId"`
	OriginalFilename string    `json:"originalFilename"`
	MediaType        string    `json:"mediaType"`
	ByteSize         int64     `json:"byteSize"`
	UploadedAt       time.Time `json:"uploadedAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type PDF struct {
	Metadata Metadata
	Content  []byte
}

type Service struct {
	pool         *pgxpool.Pool
	store        Store
	queryTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, store Store, queryTimeout time.Duration) *Service {
	return &Service{pool: pool, store: store, queryTimeout: queryTimeout}
}

func (s *Service) Upload(ctx context.Context, actor users.User, applicationID, filename string, content []byte) (Metadata, error) {
	if !actor.HasRole(users.RoleApplicant) {
		return Metadata{}, ErrForbidden
	}
	if !validPDF(filename, content) {
		return Metadata{}, ErrInvalidPDF
	}
	applicationUUID, actorUUID, err := parseIDs(applicationID, actor.ID)
	if err != nil {
		return Metadata{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	queries := sqlc.New(s.pool)
	if _, err := queries.GetDraftApplicationResumeTarget(ctx, sqlc.GetDraftApplicationResumeTargetParams{ID: applicationUUID, ApplicantUserID: actorUUID}); errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, ErrNotFound
	} else if err != nil {
		return Metadata{}, fmt.Errorf("load resume upload target: %w", err)
	}
	objectKey := "applications/" + applicationID + "/" + fmt.Sprintf("%d.pdf", time.Now().UTC().UnixNano())
	if err := s.store.Put(ctx, objectKey, content); err != nil {
		return Metadata{}, fmt.Errorf("store resume PDF: %w", err)
	}
	digest := sha256.Sum256(content)
	row, err := queries.UpsertApplicationResume(ctx, sqlc.UpsertApplicationResumeParams{
		ApplicationID: applicationUUID, ObjectKey: objectKey,
		OriginalFilename: filepath.Base(filename), ByteSize: int64(len(content)), Sha256: digest[:],
	})
	if err != nil {
		return Metadata{}, fmt.Errorf("save resume metadata: %w", err)
	}
	return metadata(row), nil
}

func (s *Service) ForApplicant(ctx context.Context, actor users.User, applicationID string) (Metadata, error) {
	applicationUUID, actorUUID, err := parseIDs(applicationID, actor.ID)
	if err != nil {
		return Metadata{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	row, err := sqlc.New(s.pool).GetApplicationResumeForApplicant(ctx, sqlc.GetApplicationResumeForApplicantParams{ApplicationID: applicationUUID, ApplicantUserID: actorUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("load applicant resume: %w", err)
	}
	return metadata(row), nil
}

func (s *Service) ForAdmin(ctx context.Context, actor users.User, applicationID string) (PDF, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return PDF{}, ErrForbidden
	}
	applicationUUID, _, err := parseIDs(applicationID, actor.ID)
	if err != nil {
		return PDF{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	row, err := sqlc.New(s.pool).GetApplicationResumeForAdmin(ctx, applicationUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PDF{}, ErrNotFound
	}
	if err != nil {
		return PDF{}, fmt.Errorf("load admin resume metadata: %w", err)
	}
	content, err := s.store.Get(ctx, row.ObjectKey)
	if err != nil {
		return PDF{}, fmt.Errorf("load resume PDF: %w", err)
	}
	return PDF{Metadata: metadata(row), Content: content}, nil
}

func validPDF(filename string, content []byte) bool {
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") || int64(len(content)) < 8 || int64(len(content)) > MaxPDFBytes {
		return false
	}
	headerEnd := len(content)
	if headerEnd > 1024 {
		headerEnd = 1024
	}
	tailStart := len(content) - 1024
	if tailStart < 0 {
		tailStart = 0
	}
	return bytes.Contains(content[:headerEnd], []byte("%PDF-")) && bytes.Contains(content[tailStart:], []byte("%%EOF"))
}

func parseIDs(applicationID, actorID string) (pgtype.UUID, pgtype.UUID, error) {
	var applicationUUID, actorUUID pgtype.UUID
	if err := applicationUUID.Scan(applicationID); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	if err := actorUUID.Scan(actorID); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	return applicationUUID, actorUUID, nil
}

func metadata(row sqlc.AtsApplicationResume) Metadata {
	return Metadata{ApplicationID: row.ApplicationID.String(), OriginalFilename: row.OriginalFilename, MediaType: row.MediaType, ByteSize: row.ByteSize, UploadedAt: row.UploadedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}
