package users

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
)

var ErrVolunteerInvalid = errors.New("invalid volunteer request")
var ErrVolunteerConflict = errors.New("volunteer request changed; refresh before reviewing")

type VolunteerRequest struct {
	UserID        string     `json:"userId"`
	RealName      string     `json:"realName"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	ReviewedAt    *time.Time `json:"reviewedAt"`
	ScannerAccess bool       `json:"scannerAccess"`
	Email         string     `json:"email,omitempty"`
}

func normalizeVolunteerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return "", ErrVolunteerInvalid
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrVolunteerInvalid
		}
	}
	return name, nil
}

const volunteerSelect = `SELECT r.user_id, r.real_name, r.status, r.created_at, r.reviewed_at,
    EXISTS (SELECT 1 FROM ats.user_roles ur WHERE ur.user_id=r.user_id AND ur.role='scanner'), u.primary_email
    FROM ats.volunteer_requests r JOIN ats.users u ON u.id=r.user_id`

func scanVolunteer(row pgx.Row) (VolunteerRequest, error) {
	var r VolunteerRequest
	err := row.Scan(&r.UserID, &r.RealName, &r.Status, &r.CreatedAt, &r.ReviewedAt, &r.ScannerAccess, &r.Email)
	return r, err
}

// GetVolunteerRequest returns only the authenticated subject's request.
func (s *Service) GetVolunteerRequest(ctx context.Context, actor User) (*VolunteerRequest, error) {
	if actor.ID == "" {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	r, err := scanVolunteer(s.pool.QueryRow(ctx, volunteerSelect+` WHERE r.user_id=$1`, actor.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Email = ""
	r.ScannerAccess = actor.HasRole(RoleScanner)
	return &r, nil
}

// One immutable request per account: repeats cannot spam the queue or replace a
// reviewed identity. Applicants and volunteers use the same verified sign-in.
func (s *Service) RequestVolunteerAccess(ctx context.Context, actor User, name string) (*VolunteerRequest, error) {
	if actor.ID == "" {
		return nil, ErrForbidden
	}
	name, err := normalizeVolunteerName(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	_, err = s.pool.Exec(ctx, `INSERT INTO ats.volunteer_requests(user_id, real_name) VALUES ($1,$2) ON CONFLICT (user_id) DO NOTHING`, actor.ID, name)
	if err != nil {
		return nil, err
	}
	return s.GetVolunteerRequest(ctx, actor)
}

// Bounded, paginated admin-only queue; names are claims, never authorization.
func (s *Service) ListVolunteerRequests(ctx context.Context, actor User, offset int) ([]VolunteerRequest, error) {
	if !actor.HasRole(RoleAdmin) {
		return nil, ErrForbidden
	}
	if offset < 0 || offset > 100000 {
		return nil, ErrVolunteerInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, volunteerSelect+` ORDER BY (r.status='pending') DESC, r.created_at, r.user_id LIMIT 50 OFFSET $1`, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []VolunteerRequest{}
	for rows.Next() {
		r, err := scanVolunteer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func validVolunteerReview(status, expected string) bool {
	switch status {
	case "approved":
		return expected == "pending"
	case "rejected":
		return expected == "pending"
	case "revoked":
		return expected == "approved"
	}
	return false
}

// The request decision, scanner-only grant/revocation and audits commit together.
// Expected status rejects stale approvals, including a replay after revocation.
func (s *Service) ReviewVolunteerRequest(ctx context.Context, actor User, target, status, expected string) error {
	if !actor.HasRole(RoleAdmin) || actor.ID == target {
		return ErrForbidden
	}
	if !validVolunteerReview(status, expected) {
		return ErrVolunteerInvalid
	}
	targetID, err := parseUUID(target)
	if err != nil {
		return ErrNotFound
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM ats.volunteer_requests WHERE user_id=$1 FOR UPDATE`, targetID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != expected {
		return ErrVolunteerConflict
	}
	// Do not imply that removing a scanner row removes inherited admin access.
	var isAdmin bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ats.admin_email_allowlist a JOIN ats.users u ON lower(u.primary_email)=a.normalized_email WHERE u.id=$1)`, targetID).Scan(&isAdmin)
	if err != nil {
		return err
	}
	if isAdmin {
		return ErrForbidden
	}
	q := sqlc.New(tx)
	var changed bool
	event := "scanner_role_assigned"
	if status == "approved" {
		changed, err = q.GrantScannerRole(ctx, sqlc.GrantScannerRoleParams{UserID: targetID, CreatedBy: actorID})
	}
	if status == "revoked" {
		event = "scanner_role_revoked"
		changed, err = q.RevokeScannerRole(ctx, targetID)
	}
	if err != nil {
		return err
	}
	if changed {
		if err = q.InsertScannerRoleAudit(ctx, sqlc.InsertScannerRoleAuditParams{ActorUserID: actorID, EventType: event, SubjectID: targetID}); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE ats.volunteer_requests SET status=$2, reviewed_by=$3, reviewed_at=clock_timestamp() WHERE user_id=$1`, targetID, status, actorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ats.audit_events(actor_user_id,event_type,subject_type,subject_id,metadata_json) VALUES($1,'volunteer_request_reviewed','user',$2,jsonb_build_object('status',$3::text))`, actorID, targetID, status)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
