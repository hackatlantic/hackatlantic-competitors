package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrganizerAlreadyExists = errors.New("an organizer already exists")

// BootstrapFirstOrganizer assigns organizer once to an existing local user.
// It returns true only when this invocation created the initial assignment.
func BootstrapFirstOrganizer(ctx context.Context, pool *pgxpool.Pool, queryTimeout time.Duration, clerkUserID string) (bool, error) {
	if clerkUserID == "" {
		return false, fmt.Errorf("missing Clerk user ID")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("begin organizer bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlc.New(tx)

	if err := queries.AcquireOrganizerBootstrapLock(ctx); err != nil {
		return false, fmt.Errorf("acquire organizer bootstrap lock: %w", err)
	}
	targetID, err := queries.GetUserIDByClerkUserID(ctx, clerkUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("local Clerk user %q does not exist", clerkUserID)
	}
	if err != nil {
		return false, fmt.Errorf("find local user: %w", err)
	}
	organizers, err := queries.CountOrganizerRoles(ctx)
	if err != nil {
		return false, fmt.Errorf("count organizers: %w", err)
	}
	if organizers > 0 {
		isOrganizer, err := queries.UserHasOrganizerRole(ctx, targetID)
		if err != nil {
			return false, fmt.Errorf("check target organizer role: %w", err)
		}
		if !isOrganizer {
			return false, ErrOrganizerAlreadyExists
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit idempotent organizer bootstrap: %w", err)
		}
		return false, nil
	}
	if err := queries.AssignUserRole(ctx, sqlc.AssignUserRoleParams{UserID: targetID, Role: string(RoleOrganizer)}); err != nil {
		return false, fmt.Errorf("assign organizer role: %w", err)
	}
	if err := queries.InsertBootstrapOrganizerAuditEvent(ctx, targetID); err != nil {
		return false, fmt.Errorf("write organizer bootstrap audit event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit organizer bootstrap: %w", err)
	}
	return true, nil
}
