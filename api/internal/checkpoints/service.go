// Package checkpoints exposes scanner-safe active checkpoint projections.
package checkpoints

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrForbidden = errors.New("scanner access is forbidden")

const defaultQueryTimeout = 5 * time.Second

// Checkpoint deliberately omits configuration and entitlement defaults. A
// scanner needs an identifier and human label, not operational policy details.
type Checkpoint struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Service struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, queryTimeout time.Duration) *Service {
	if queryTimeout <= 0 {
		queryTimeout = defaultQueryTimeout
	}
	return &Service{pool: pool, queryTimeout: queryTimeout}
}

// ListActive returns every active checkpoint to any application-global scanner.
// Checkpoint-scoped scanner grants intentionally do not exist.
func (s *Service) ListActive(ctx context.Context, actor users.User) ([]Checkpoint, error) {
	if !actor.HasRole(users.RoleScanner) {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, `SELECT id, name FROM ats.checkpoints WHERE active ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list active checkpoints: %w", err)
	}
	defer rows.Close()

	result := make([]Checkpoint, 0)
	for rows.Next() {
		var id pgtype.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan active checkpoint: %w", err)
		}
		result = append(result, Checkpoint{ID: id.String(), Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active checkpoints: %w", err)
	}
	return result, nil
}
