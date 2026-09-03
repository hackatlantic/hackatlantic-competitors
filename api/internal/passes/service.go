// Package passes owns issuance and revocation of one-time bearer credentials.
package passes

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden    = errors.New("organizer or attendee access is forbidden")
	ErrNotFound     = errors.New("pass or eligible attendee not found")
	ErrActivePass   = errors.New("attendee already has an active pass")
	ErrRSVPRequired = errors.New("confirmed RSVP required before pass release")
	ErrInvalidID    = errors.New("invalid identifier")
	ErrInvalidCred  = errors.New("invalid credential")
)

const (
	defaultQueryTimeout       = 5 * time.Second
	defaultTransactionTimeout = 15 * time.Second
	credentialEntropyBytes    = 32
	minimumPepperBytes        = 32
	qrCredentialPrefix        = "qr_v1."
	claimCredentialPrefix     = "claim_v1."
)

// Config contains the server-only credential hashing material. The peppers are
// never returned, stored in PostgreSQL, or included in errors.
type Config struct {
	QRTokenPepper    string
	ClaimTokenPepper string
	AppBaseURL       string
}

// Pass is the safe pass projection. It intentionally has no bearer credential
// or credential hash fields.
type Pass struct {
	ID          string     `json:"id"`
	AttendeeID  string     `json:"attendeeId"`
	DisplayName string     `json:"displayName"`
	Status      string     `json:"status"`
	IssuedAt    time.Time  `json:"issuedAt"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

// Issuance exposes fresh bearer credentials after issue or reissue. The claim
// credential is unrecoverable after this response; the QR credential may be
// re-derived only for the authenticated owner of the active pass.
type Issuance struct {
	Pass
	QRToken    string `json:"qrToken"`
	ClaimToken string `json:"claimToken"`
	ClaimURL   string `json:"claimUrl,omitempty"`
}

// WebPass exposes the derived QR credential only to the accepted attendee who
// owns the active pass. The value is re-derived from server-held pepper and the
// pass ID, never recovered from database storage.
type WebPass struct {
	Pass
	QRToken string `json:"qrToken"`
}

// ClaimPass is the deliberately minimal public projection resolved by a claim
// credential. It contains neither attendee contact data nor a credential.
type ClaimPass struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Status      string    `json:"status"`
	IssuedAt    time.Time `json:"issuedAt"`
}

// OrganizerSummary allows the organizer application view to target an accepted
// attendee without exposing bearer values or stored hashes.
type OrganizerSummary struct {
	AttendeeID string `json:"attendeeId"`
	Pass       *Pass  `json:"pass"`
}

type Service struct {
	pool               *pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
	qrTokenPepper      []byte
	claimTokenPepper   []byte
	appBaseURL         *url.URL
}

// NewService validates credential configuration before accepting pass traffic.
func NewService(pool *pgxpool.Pool, queryTimeout, transactionTimeout time.Duration, config Config) (*Service, error) {
	qrPepper, err := decodePepper("QR_TOKEN_PEPPER", config.QRTokenPepper)
	if err != nil {
		return nil, err
	}
	claimPepper, err := decodePepper("CLAIM_TOKEN_PEPPER", config.ClaimTokenPepper)
	if err != nil {
		return nil, err
	}
	if hmac.Equal(qrPepper, claimPepper) {
		return nil, errors.New("QR_TOKEN_PEPPER and CLAIM_TOKEN_PEPPER must differ")
	}
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	if transactionTimeout <= 0 {
		transactionTimeout = defaultTransactionTimeout
	}
	var baseURL *url.URL
	if raw := strings.TrimSpace(config.AppBaseURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, errors.New("APP_BASE_URL must be an absolute HTTP URL")
		}
		baseURL = parsed
	}
	return &Service{
		pool:               pool,
		queryTimeout:       queryTimeout,
		transactionTimeout: transactionTimeout,
		qrTokenPepper:      qrPepper,
		claimTokenPepper:   claimPepper,
		appBaseURL:         baseURL,
	}, nil
}

func decodePepper(name, encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) < minimumPepperBytes {
		return nil, fmt.Errorf("%s must be standard base64 encoding at least %d random bytes", name, minimumPepperBytes)
	}
	return decoded, nil
}

// Issue is an explicit organizer action for a released, RSVP-confirmed acceptance.
func (s *Service) Issue(ctx context.Context, actor users.User, attendeeID string) (Issuance, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Issuance{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Issuance{}, fmt.Errorf("parse organizer ID: %w", err)
	}
	attendeeUUID, err := parseUUID(attendeeID)
	if err != nil {
		return Issuance{}, ErrNotFound
	}
	newPassID, err := randomUUID()
	if err != nil {
		return Issuance{}, err
	}
	credentials, err := s.newCredentials(newPassID)
	if err != nil {
		return Issuance{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Issuance{}, fmt.Errorf("begin pass issue transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attendee, err := lockAcceptedAttendee(ctx, tx, attendeeUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Issuance{}, ErrNotFound
	}
	if err != nil {
		return Issuance{}, fmt.Errorf("lock accepted attendee: %w", err)
	}
	var existingID pgtype.UUID
	if err := requireConfirmedRSVP(ctx, tx, attendee.id); err != nil {
		return Issuance{}, err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM ats.passes WHERE attendee_id = $1 AND status = 'active' FOR UPDATE`, attendee.id).Scan(&existingID)
	if err == nil {
		return Issuance{}, ErrActivePass
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Issuance{}, fmt.Errorf("check active pass: %w", err)
	}
	created, err := insertPass(ctx, tx, newPassID, attendee.id, credentials.qrHash, credentials.claimHash)
	if err != nil {
		return Issuance{}, fmt.Errorf("insert pass: %w", err)
	}
	if err := insertPassAudit(ctx, tx, "pass_issued", actorID, created.id, attendee.id, pgtype.UUID{}); err != nil {
		return Issuance{}, fmt.Errorf("audit pass issue: %w", err)
	}
	if err := s.queuePassLink(ctx, tx, attendee, created.id); err != nil {
		return Issuance{}, fmt.Errorf("queue pass link: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Issuance{}, fmt.Errorf("commit pass issue transaction: %w", err)
	}
	return s.issuanceFrom(created, attendee.displayName, credentials), nil
}

// Reissue replaces an active accepted-attendee pass atomically and returns only
// the new bearer values.
func (s *Service) Reissue(ctx context.Context, actor users.User, passID string) (Issuance, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Issuance{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Issuance{}, fmt.Errorf("parse organizer ID: %w", err)
	}
	passUUID, err := parseUUID(passID)
	if err != nil {
		return Issuance{}, ErrNotFound
	}
	newPassID, err := randomUUID()
	if err != nil {
		return Issuance{}, err
	}
	credentials, err := s.newCredentials(newPassID)
	if err != nil {
		return Issuance{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Issuance{}, fmt.Errorf("begin pass reissue transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attendeeID, err := passAttendeeID(ctx, tx, passUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Issuance{}, ErrNotFound
	}
	if err != nil {
		return Issuance{}, fmt.Errorf("load pass attendee: %w", err)
	}
	if _, err := lockAcceptedAttendee(ctx, tx, attendeeID); errors.Is(err, pgx.ErrNoRows) {
		return Issuance{}, ErrNotFound
	} else if err != nil {
		return Issuance{}, fmt.Errorf("lock accepted attendee: %w", err)
	}
	if err := requireConfirmedRSVP(ctx, tx, attendeeID); err != nil {
		return Issuance{}, err
	}
	current, err := lockActivePass(ctx, tx, passUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Issuance{}, ErrNotFound
	}
	if err != nil {
		return Issuance{}, fmt.Errorf("lock active pass: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ats.passes SET status = 'replaced', revoked_at = CURRENT_TIMESTAMP, replaced_by_pass_id = $2 WHERE id = $1 AND status = 'active'`, current.id, newPassID); err != nil {
		return Issuance{}, fmt.Errorf("replace current pass: %w", err)
	}
	created, err := insertPass(ctx, tx, newPassID, current.attendeeID, credentials.qrHash, credentials.claimHash)
	if err != nil {
		return Issuance{}, fmt.Errorf("insert replacement pass: %w", err)
	}
	if err := insertPassAudit(ctx, tx, "pass_reissued", actorID, current.id, current.attendeeID, created.id); err != nil {
		return Issuance{}, fmt.Errorf("audit pass reissue: %w", err)
	}
	if err := s.queuePassLink(ctx, tx, current.attendee, created.id); err != nil {
		return Issuance{}, fmt.Errorf("queue replacement pass link: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Issuance{}, fmt.Errorf("commit pass reissue transaction: %w", err)
	}
	return s.issuanceFrom(created, current.attendee.displayName, credentials), nil
}

// Revoke transitions an active pass once. Reissue and revoke are serialized by
// the same row lock and active-status predicate.
func (s *Service) Revoke(ctx context.Context, actor users.User, passID string) (Pass, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return Pass{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Pass{}, fmt.Errorf("parse organizer ID: %w", err)
	}
	passUUID, err := parseUUID(passID)
	if err != nil {
		return Pass{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.transactionTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Pass{}, fmt.Errorf("begin pass revoke transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attendeeID, err := passAttendeeID(ctx, tx, passUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pass{}, ErrNotFound
	}
	if err != nil {
		return Pass{}, fmt.Errorf("load pass attendee: %w", err)
	}
	if _, err := lockAcceptedAttendee(ctx, tx, attendeeID); errors.Is(err, pgx.ErrNoRows) {
		return Pass{}, ErrNotFound
	} else if err != nil {
		return Pass{}, fmt.Errorf("lock accepted attendee: %w", err)
	}
	current, err := lockActivePass(ctx, tx, passUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pass{}, ErrNotFound
	}
	if err != nil {
		return Pass{}, fmt.Errorf("lock active pass: %w", err)
	}
	var revokedAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `UPDATE ats.passes SET status = 'revoked', revoked_at = CURRENT_TIMESTAMP WHERE id = $1 AND status = 'active' RETURNING revoked_at`, current.id).Scan(&revokedAt); err != nil {
		return Pass{}, fmt.Errorf("revoke pass: %w", err)
	}
	if err := insertPassAudit(ctx, tx, "pass_revoked", actorID, current.id, current.attendeeID, pgtype.UUID{}); err != nil {
		return Pass{}, fmt.Errorf("audit pass revoke: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Pass{}, fmt.Errorf("commit pass revoke transaction: %w", err)
	}
	return passFrom(current.id, current.attendeeID, current.attendee.displayName, "revoked", current.issuedAt, revokedAt), nil
}

// WebPass returns the active accepted attendee's safe metadata and a QR bearer
// value deterministically derived from the pass ID and server-only QR pepper.
// Claim credentials remain random and unrecoverable after issuance.
func (s *Service) WebPass(ctx context.Context, actor users.User) (WebPass, error) {
	if !actor.HasRole(users.RoleApplicant) {
		return WebPass{}, ErrForbidden
	}
	userID, err := parseUUID(actor.ID)
	if err != nil {
		return WebPass{}, fmt.Errorf("parse attendee user ID: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var row safePassRow
	err = s.pool.QueryRow(ctx, `SELECT passes.id, passes.attendee_id, attendees.display_name, passes.status, passes.issued_at, passes.revoked_at FROM ats.passes JOIN ats.attendees ON attendees.id = passes.attendee_id JOIN ats.applications ON applications.id = attendees.application_id WHERE attendees.user_id = $1 AND passes.status = 'active' AND applications.status = 'accepted' AND applications.decision_released_at IS NOT NULL`, userID).Scan(&row.id, &row.attendeeID, &row.displayName, &row.status, &row.issuedAt, &row.revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebPass{}, ErrNotFound
	}
	if err != nil {
		return WebPass{}, fmt.Errorf("load attendee web pass: %w", err)
	}
	return WebPass{
		Pass:    passFrom(row.id, row.attendeeID, row.displayName, row.status, row.issuedAt, row.revokedAt),
		QRToken: s.derivedQRCredential(row.id),
	}, nil
}

// QRTokenHash validates a QR credential and returns the exact purpose-separated
// HMAC stored on ats.passes. It is intentionally the only cross-package QR
// verification primitive so scanners cannot accidentally use claim hashing.
// The returned bytes are zeroed when the credential is malformed or belongs to
// another credential family; callers may persist that sentinel only for
// idempotency identity and must never return it.
func (s *Service) QRTokenHash(qrToken string) ([]byte, bool) {
	if !validCredential(qrToken, qrCredentialPrefix) {
		return make([]byte, credentialEntropyBytes), false
	}
	return credentialHash(s.qrTokenPepper, "qr", qrToken), true
}

// ResolveClaim resolves only a claim credential. QR values are deliberately
// rejected before the claim HMAC is computed, preserving credential separation.
func (s *Service) ResolveClaim(ctx context.Context, claimToken string) (ClaimPass, error) {
	if !validCredential(claimToken, claimCredentialPrefix) {
		return ClaimPass{}, ErrNotFound
	}
	claimHash := credentialHash(s.claimTokenPepper, "claim", claimToken)
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var id pgtype.UUID
	var displayName, status string
	var issuedAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, `SELECT passes.id, attendees.display_name, passes.status, passes.issued_at FROM ats.passes JOIN ats.attendees ON attendees.id = passes.attendee_id JOIN ats.applications ON applications.id = attendees.application_id WHERE passes.claim_token_hash = $1 AND passes.status = 'active' AND applications.status = 'accepted' AND applications.decision_released_at IS NOT NULL`, claimHash).Scan(&id, &displayName, &status, &issuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaimPass{}, ErrNotFound
	}
	if err != nil {
		return ClaimPass{}, fmt.Errorf("resolve claim pass: %w", err)
	}
	return ClaimPass{ID: id.String(), DisplayName: displayName, Status: status, IssuedAt: issuedAt.Time.UTC()}, nil
}

// SummaryForApplication is organizer-only and contains no credential material.
func (s *Service) SummaryForApplication(ctx context.Context, actor users.User, applicationID string) (OrganizerSummary, error) {
	if !actor.HasRole(users.RoleOrganizer) {
		return OrganizerSummary{}, ErrForbidden
	}
	id, err := parseUUID(applicationID)
	if err != nil {
		return OrganizerSummary{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	var attendeeID, passID pgtype.UUID
	var displayName string
	var status pgtype.Text
	var issuedAt, revokedAt pgtype.Timestamptz
	err = s.pool.QueryRow(ctx, `SELECT attendees.id, passes.id, attendees.display_name, passes.status, passes.issued_at, passes.revoked_at FROM ats.attendees LEFT JOIN ats.passes ON passes.attendee_id = attendees.id AND passes.status = 'active' WHERE attendees.application_id = $1`, id).Scan(&attendeeID, &passID, &displayName, &status, &issuedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizerSummary{}, ErrNotFound
	}
	if err != nil {
		return OrganizerSummary{}, fmt.Errorf("load organizer pass summary: %w", err)
	}
	summary := OrganizerSummary{AttendeeID: attendeeID.String()}
	if passID.Valid {
		pass := passFrom(passID, attendeeID, displayName, status.String, issuedAt, revokedAt)
		summary.Pass = &pass
	}
	return summary, nil
}

type attendeeRow struct {
	id          pgtype.UUID
	userID      pgtype.UUID
	displayName string
	email       string
}

type lockedPass struct {
	id         pgtype.UUID
	attendeeID pgtype.UUID
	issuedAt   pgtype.Timestamptz
	attendee   attendeeRow
}

type insertedPass struct {
	id         pgtype.UUID
	attendeeID pgtype.UUID
	status     string
	issuedAt   pgtype.Timestamptz
	revokedAt  pgtype.Timestamptz
}

type safePassRow struct {
	id          pgtype.UUID
	attendeeID  pgtype.UUID
	displayName string
	status      string
	issuedAt    pgtype.Timestamptz
	revokedAt   pgtype.Timestamptz
}

type credentials struct {
	qr        string
	claim     string
	qrHash    []byte
	claimHash []byte
}

func (s *Service) newCredentials(passID pgtype.UUID) (credentials, error) {
	claim, err := generateCredential(claimCredentialPrefix)
	if err != nil {
		return credentials{}, fmt.Errorf("generate claim credential: %w", err)
	}
	qr := s.derivedQRCredential(passID)
	return credentials{
		qr: qr, claim: claim,
		qrHash:    credentialHash(s.qrTokenPepper, "qr", qr),
		claimHash: credentialHash(s.claimTokenPepper, "claim", claim),
	}, nil
}

func (s *Service) derivedQRCredential(passID pgtype.UUID) string {
	mac := hmac.New(sha256.New, s.qrTokenPepper)
	_, _ = mac.Write([]byte("hackatlantic:passes:qr-derived:v1\x00"))
	_, _ = mac.Write(passID.Bytes[:])
	return qrCredentialPrefix + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) issuanceFrom(created insertedPass, displayName string, credentials credentials) Issuance {
	result := Issuance{
		Pass:       passFrom(created.id, created.attendeeID, displayName, created.status, created.issuedAt, created.revokedAt),
		QRToken:    credentials.qr,
		ClaimToken: credentials.claim,
	}
	if s.appBaseURL != nil {
		claimURL := *s.appBaseURL
		claimURL.Path = strings.TrimRight(claimURL.Path, "/") + "/claim/" + url.PathEscape(credentials.claim)
		result.ClaimURL = claimURL.String()
	}
	return result
}

func lockAcceptedAttendee(ctx context.Context, tx pgx.Tx, attendeeID pgtype.UUID) (attendeeRow, error) {
	var attendee attendeeRow
	err := tx.QueryRow(ctx, `SELECT attendees.id, attendees.user_id, attendees.display_name, attendees.email FROM ats.attendees JOIN ats.applications ON applications.id = attendees.application_id WHERE attendees.id = $1 AND applications.status = 'accepted' AND applications.decision_released_at IS NOT NULL FOR UPDATE OF attendees, applications`, attendeeID).Scan(&attendee.id, &attendee.userID, &attendee.displayName, &attendee.email)
	return attendee, err
}

func passAttendeeID(ctx context.Context, tx pgx.Tx, passID pgtype.UUID) (pgtype.UUID, error) {
	var attendeeID pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT attendee_id FROM ats.passes WHERE id = $1`, passID).Scan(&attendeeID)
	return attendeeID, err
}

// Call only after locking the attendee and application. RSVP changes use the
// same application lock; this separate read sees changes committed while waiting.
func requireConfirmedRSVP(ctx context.Context, tx pgx.Tx, attendeeID pgtype.UUID) error {
	var confirmed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
      SELECT 1 FROM ats.attendees attendee
      JOIN ats.applications application ON application.id = attendee.application_id
      JOIN ats.decisions decision ON decision.id = application.current_decision_id
        AND decision.application_id = application.id
      JOIN ats.attendance_responses response ON response.decision_id = decision.id
      WHERE attendee.id = $1 AND decision.outcome = 'accepted'
        AND decision.released_at IS NOT NULL AND response.status = 'confirmed'
    )`, attendeeID).Scan(&confirmed)
	if err != nil {
		return fmt.Errorf("check pass RSVP eligibility: %w", err)
	}
	if !confirmed {
		return ErrRSVPRequired
	}
	return nil
}

func lockActivePass(ctx context.Context, tx pgx.Tx, passID pgtype.UUID) (lockedPass, error) {
	var pass lockedPass
	err := tx.QueryRow(ctx, `SELECT passes.id, passes.attendee_id, passes.issued_at, attendees.id, attendees.user_id, attendees.display_name, attendees.email FROM ats.passes JOIN ats.attendees ON attendees.id = passes.attendee_id JOIN ats.applications ON applications.id = attendees.application_id WHERE passes.id = $1 AND passes.status = 'active' AND applications.status = 'accepted' AND applications.decision_released_at IS NOT NULL FOR UPDATE OF passes, attendees, applications`, passID).Scan(&pass.id, &pass.attendeeID, &pass.issuedAt, &pass.attendee.id, &pass.attendee.userID, &pass.attendee.displayName, &pass.attendee.email)
	return pass, err
}

func insertPass(ctx context.Context, tx pgx.Tx, id, attendeeID pgtype.UUID, qrHash, claimHash []byte) (insertedPass, error) {
	var pass insertedPass
	var err error
	if id.Valid {
		err = tx.QueryRow(ctx, `INSERT INTO ats.passes (id, attendee_id, qr_token_hash, claim_token_hash) VALUES ($1, $2, $3, $4) RETURNING id, attendee_id, status, issued_at, revoked_at`, id, attendeeID, qrHash, claimHash).Scan(&pass.id, &pass.attendeeID, &pass.status, &pass.issuedAt, &pass.revokedAt)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO ats.passes (attendee_id, qr_token_hash, claim_token_hash) VALUES ($1, $2, $3) RETURNING id, attendee_id, status, issued_at, revoked_at`, attendeeID, qrHash, claimHash).Scan(&pass.id, &pass.attendeeID, &pass.status, &pass.issuedAt, &pass.revokedAt)
	}
	return pass, err
}

func insertPassAudit(ctx context.Context, tx pgx.Tx, eventType string, actorID, subjectID, attendeeID, replacementID pgtype.UUID) error {
	var metadata []byte
	var err error
	switch eventType {
	case "pass_issued", "pass_revoked":
		metadata, err = json.Marshal(struct {
			AttendeeID string `json:"attendeeId"`
		}{AttendeeID: attendeeID.String()})
	case "pass_reissued":
		metadata, err = json.Marshal(struct {
			AttendeeID        string `json:"attendeeId"`
			ReplacementPassID string `json:"replacementPassId"`
		}{AttendeeID: attendeeID.String(), ReplacementPassID: replacementID.String()})
	default:
		return errors.New("unsupported pass audit event")
	}
	if err != nil {
		return fmt.Errorf("encode pass audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json) VALUES ($1, $2, 'pass', $3, $4)`, actorID, eventType, subjectID, metadata)
	return err
}

func (s *Service) queuePassLink(ctx context.Context, tx pgx.Tx, attendee attendeeRow, passID pgtype.UUID) error {
	webPassURL := ""
	if s.appBaseURL != nil {
		value := *s.appBaseURL
		value.Path = strings.TrimRight(value.Path, "/") + "/attendee/pass"
		webPassURL = value.String()
	}
	templateData, err := json.Marshal(struct {
		PassID     string `json:"passId"`
		WebPassURL string `json:"webPassUrl,omitempty"`
	}{PassID: passID.String(), WebPassURL: webPassURL})
	if err != nil {
		return fmt.Errorf("encode pass link email: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO ats.email_outbox (event_type, recipient_user_id, recipient_email, template_key, template_data_json, dedupe_key) VALUES ('pass_link', $1, $2, 'pass_link', $3, $4) ON CONFLICT (dedupe_key) DO NOTHING`, attendee.userID, attendee.email, templateData, "pass_link:"+passID.String())
	return err
}

func passFrom(id, attendeeID pgtype.UUID, displayName, status string, issuedAt, revokedAt pgtype.Timestamptz) Pass {
	result := Pass{ID: id.String(), AttendeeID: attendeeID.String(), DisplayName: displayName, Status: status, IssuedAt: issuedAt.Time.UTC()}
	if revokedAt.Valid {
		value := revokedAt.Time.UTC()
		result.RevokedAt = &value
	}
	return result
}

func generateCredential(prefix string) (string, error) {
	entropy := make([]byte, credentialEntropyBytes)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(entropy), nil
}

func credentialHash(pepper []byte, purpose, credential string) []byte {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte("hackatlantic:passes:" + purpose + ":v1\x00"))
	_, _ = mac.Write([]byte(credential))
	return mac.Sum(nil)
}

func validCredential(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+base64.RawURLEncoding.EncodedLen(credentialEntropyBytes) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(decoded) == credentialEntropyBytes
}

func parseUUID(value string) (pgtype.UUID, error) {
	var result pgtype.UUID
	if err := result.Scan(value); err != nil || !result.Valid {
		return pgtype.UUID{}, ErrInvalidID
	}
	return result, nil
}

func randomUUID() (pgtype.UUID, error) {
	var id pgtype.UUID
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		return pgtype.UUID{}, fmt.Errorf("generate pass ID: %w", err)
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	id.Valid = true
	return id, nil
}
