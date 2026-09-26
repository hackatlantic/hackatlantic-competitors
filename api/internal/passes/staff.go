package passes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// StaffPass is owner-only. The credential never grants application access or meals.
type StaffPass struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	QRToken     string    `json:"qrToken"`
}

// Eligibility is re-evaluated from the database, not a cached session role.
const staffEligibility = `SELECT u.id,
    CASE WHEN a.normalized_email IS NOT NULL THEN 'organizer' ELSE 'volunteer' END AS kind,
    CASE WHEN a.normalized_email IS NOT NULL THEN COALESCE(NULLIF(trim(u.display_name), ''), u.primary_email) ELSE r.real_name END AS display_name
    FROM ats.users u
    LEFT JOIN ats.admin_email_allowlist a ON a.normalized_email=lower(u.primary_email)
    LEFT JOIN ats.volunteer_requests r ON r.user_id=u.id
    WHERE a.normalized_email IS NOT NULL OR (r.status='approved' AND EXISTS (
        SELECT 1 FROM ats.user_roles ur WHERE ur.user_id=u.id AND ur.role='scanner'))`

// EnsureStaffPass is an idempotent owner-only issuance, independent of intake/RSVP.
// A unique user/event constraint makes concurrent page loads share one credential.
func (s *Service) EnsureStaffPass(ctx context.Context, actor users.User) (StaffPass, error) {
	userID, err := parseUUID(actor.ID)
	if err != nil {
		return StaffPass{}, ErrNotFound
	}
	id, err := randomUUID()
	if err != nil {
		return StaffPass{}, err
	}
	token := s.derivedQRCredential(id)
	hash, _ := s.QRTokenHash(token)
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	// INSERT ... SELECT makes the eligibility test part of the issuance statement.
	_, err = s.pool.Exec(ctx, `WITH issued AS (INSERT INTO ats.staff_passes(id,user_id,event_key,qr_token_hash,expires_at)
        SELECT $1,e.id,'hackatlantic-2026',$3,$4 FROM (`+staffEligibility+`) e
        WHERE e.id=$2 AND now()<$4
        ON CONFLICT (user_id,event_key) DO NOTHING RETURNING id,user_id)
        INSERT INTO ats.audit_events(actor_user_id,event_type,subject_type,subject_id,metadata_json)
        SELECT user_id,'staff_pass_issued','staff_pass',id,'{}'::jsonb FROM issued`, id, userID, hash, s.staffPassExpiresAt)
	if err != nil {
		return StaffPass{}, fmt.Errorf("ensure staff pass: %w", err)
	}
	var pass StaffPass
	var storedID pgtype.UUID
	err = s.pool.QueryRow(ctx, `SELECT p.id,e.display_name,e.kind,p.issued_at,p.expires_at
        FROM ats.staff_passes p JOIN (`+staffEligibility+`) e ON e.id=p.user_id
        WHERE p.user_id=$1 AND p.event_key='hackatlantic-2026' AND p.expires_at>now()`, userID).
		Scan(&storedID, &pass.DisplayName, &pass.Kind, &pass.IssuedAt, &pass.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffPass{}, ErrNotFound
	}
	if err != nil {
		return StaffPass{}, fmt.Errorf("load staff pass: %w", err)
	}
	pass.ID = storedID.String()
	pass.Status = "active"
	pass.QRToken = s.derivedQRCredential(storedID)
	return pass, nil
}

// VerifyStaffPass returns no credential or contact data. Removed approval/role or
// admin allowlisting and expired passes fail closed, including saved screenshots.
func (s *Service) VerifyStaffPass(ctx context.Context, hash []byte) (name, kind string, err error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	err = s.pool.QueryRow(ctx, `SELECT e.display_name,e.kind FROM ats.staff_passes p
        JOIN (`+staffEligibility+`) e ON e.id=p.user_id
        WHERE p.qr_token_hash=$1 AND p.event_key='hackatlantic-2026' AND p.expires_at>now()`, hash).Scan(&name, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return
}
