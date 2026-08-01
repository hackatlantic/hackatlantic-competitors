// Package users owns local Clerk-linked identities and authorization roles.
package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden   = errors.New("forbidden")
	ErrInvalidRole = errors.New("invalid role")
	ErrNotFound    = errors.New("user not found")
)

type Role string

const (
	RoleApplicant Role = "applicant"
	RoleAdmin     Role = "admin"
	RoleReviewer  Role = "reviewer"
	RoleOrganizer Role = "organizer"
	RoleScanner   Role = "scanner"
)

// User is the local authorization subject returned to HTTP adapters.
type User struct {
	ID          string
	ClerkUserID string
	Email       string
	DisplayName *string
	Roles       map[Role]struct{}
}

func (u User) HasRole(role Role) bool {
	_, ok := u.Roles[role]
	if ok {
		return true
	}
	_, admin := u.Roles[RoleAdmin]
	return admin && (role == RoleReviewer || role == RoleOrganizer || role == RoleScanner)
}

// Service transactionally reconciles verified Clerk profiles with local users.
type Service struct {
	pool         *pgxpool.Pool
	profiles     ProfileSource
	queryTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, profiles ProfileSource, queryTimeout time.Duration) *Service {
	return &Service{pool: pool, profiles: profiles, queryTimeout: queryTimeout}
}

// Resolve synchronizes trusted Clerk identity data and ensures only applicant
// is granted automatically. Existing staff roles are additive and preserved.
func (s *Service) Resolve(ctx context.Context, clerkUserID string) (User, error) {
	if clerkUserID == "" {
		return User{}, fmt.Errorf("resolve local user: missing Clerk subject")
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	profile, err := s.profiles.Profile(ctx, clerkUserID)
	if err != nil {
		return User{}, fmt.Errorf("load verified Clerk profile: %w", err)
	}
	if profile.ClerkUserID != clerkUserID || profile.Email == "" {
		return User{}, fmt.Errorf("invalid verified Clerk profile")
	}

	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin user reconciliation: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	queries := sqlc.New(transaction)

	displayName := pgtype.Text{}
	if profile.DisplayName != nil {
		displayName = pgtype.Text{String: *profile.DisplayName, Valid: true}
	}
	record, err := queries.UpsertClerkUser(ctx, sqlc.UpsertClerkUserParams{
		ClerkUserID:  profile.ClerkUserID,
		PrimaryEmail: profile.Email,
		DisplayName:  displayName,
	})
	if err != nil {
		return User{}, fmt.Errorf("upsert local user: %w", err)
	}
	if err := queries.EnsureApplicantRole(ctx, record.ID); err != nil {
		return User{}, fmt.Errorf("ensure applicant role: %w", err)
	}
	roleValues, err := queries.ListUserRoles(ctx, record.ID)
	if err != nil {
		return User{}, fmt.Errorf("load user roles: %w", err)
	}
	roles := make(map[Role]struct{}, len(roleValues))
	for _, role := range roleValues {
		roles[Role(role)] = struct{}{}
	}
	isAdmin, err := queries.UserEmailIsAdmin(ctx, profile.Email)
	if err != nil {
		return User{}, fmt.Errorf("resolve admin access: %w", err)
	}
	// Admin is derived from PostgreSQL and Clerk's verified primary email on
	// every request. It is never accepted from the browser or a JWT role claim.
	if isAdmin {
		roles[RoleAdmin] = struct{}{}
	}
	if err := transaction.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit user reconciliation: %w", err)
	}

	var resolvedDisplayName *string
	if record.DisplayName.Valid {
		resolvedDisplayName = &record.DisplayName.String
	}
	return User{
		ID:          record.ID.String(),
		ClerkUserID: record.ClerkUserID,
		Email:       record.PrimaryEmail,
		DisplayName: resolvedDisplayName,
		Roles:       roles,
	}, nil
}

// AssignPrivilegedRole requires an existing organizer and rejects self-service
// assignments. It accepts only reviewer, organizer, or scanner assignments.
func (s *Service) AssignPrivilegedRole(ctx context.Context, actor User, targetUserID string, role Role) error {
	if !actor.HasRole(RoleOrganizer) || actor.ID == targetUserID {
		return ErrForbidden
	}
	if role != RoleReviewer && role != RoleOrganizer && role != RoleScanner {
		return ErrInvalidRole
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	targetID, err := parseUUID(targetUserID)
	if err != nil {
		return fmt.Errorf("parse target user ID: %w", err)
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return fmt.Errorf("parse actor user ID: %w", err)
	}
	if err := sqlc.New(s.pool).AssignUserRole(ctx, sqlc.AssignUserRoleParams{
		UserID:    targetID,
		Role:      string(role),
		CreatedBy: actorID,
	}); err != nil {
		return fmt.Errorf("assign privileged role: %w", err)
	}
	return nil
}

// GrantScannerRole gives an existing local user event-scanner access and
// records the authorization change atomically. Repeating the grant is a no-op.
func (s *Service) GrantScannerRole(ctx context.Context, actor User, targetUserID string) error {
	return s.changeScannerRole(ctx, actor, targetUserID, true)
}

// RevokeScannerRole removes event-scanner access and records the change
// atomically. Repeating the revocation is a no-op.
func (s *Service) RevokeScannerRole(ctx context.Context, actor User, targetUserID string) error {
	return s.changeScannerRole(ctx, actor, targetUserID, false)
}

func (s *Service) changeScannerRole(ctx context.Context, actor User, targetUserID string, grant bool) error {
	if !actor.HasRole(RoleOrganizer) || actor.ID == targetUserID {
		return ErrForbidden
	}
	targetID, err := parseUUID(targetUserID)
	if err != nil {
		return ErrNotFound
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return fmt.Errorf("parse actor user ID: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin scanner role transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlc.New(tx)
	exists, err := queries.IdentityUserExists(ctx, targetID)
	if err != nil {
		return fmt.Errorf("check scanner role target: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	var changed bool
	eventType := "scanner_role_revoked"
	if grant {
		changed, err = queries.GrantScannerRole(ctx, sqlc.GrantScannerRoleParams{UserID: targetID, CreatedBy: actorID})
		eventType = "scanner_role_assigned"
	} else {
		changed, err = queries.RevokeScannerRole(ctx, targetID)
	}
	if err != nil {
		return fmt.Errorf("change scanner role: %w", err)
	}
	if changed {
		if err := queries.InsertScannerRoleAudit(ctx, sqlc.InsertScannerRoleAuditParams{
			ActorUserID: actorID, EventType: eventType, SubjectID: targetID,
		}); err != nil {
			return fmt.Errorf("audit scanner role change: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit scanner role transaction: %w", err)
	}
	return nil
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
