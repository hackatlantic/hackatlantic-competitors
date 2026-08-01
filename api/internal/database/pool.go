// Package database owns PostgreSQL connectivity and database-level time limits.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMaxConns           int32 = 10
	defaultQueryTimeout             = 5 * time.Second
	defaultTransactionTimeout       = 15 * time.Second
)

// Config contains bounded PostgreSQL connection and operation settings.
type Config struct {
	URL                string
	MaxConns           int32
	QueryTimeout       time.Duration
	TransactionTimeout time.Duration
}

// Pool wraps a pgx pool with its authoritative operation deadlines.
type Pool struct {
	*pgxpool.Pool
	queryTimeout       time.Duration
	transactionTimeout time.Duration
}

// Open constructs a connection pool. It does not contact PostgreSQL; readiness
// and migrations make deadline-bound calls explicitly.
func Open(ctx context.Context, config Config) (*Pool, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if config.MaxConns == 0 {
		config.MaxConns = defaultMaxConns
	}
	if config.MaxConns < 1 {
		return nil, fmt.Errorf("database max connections must be positive")
	}
	if config.QueryTimeout == 0 {
		config.QueryTimeout = defaultQueryTimeout
	}
	if config.TransactionTimeout == 0 {
		config.TransactionTimeout = defaultTransactionTimeout
	}
	if config.QueryTimeout <= 0 || config.TransactionTimeout <= 0 {
		return nil, fmt.Errorf("database timeouts must be positive")
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", config.QueryTimeout.Milliseconds())
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", config.QueryTimeout.Milliseconds())

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	return &Pool{Pool: pool, queryTimeout: config.QueryTimeout, transactionTimeout: config.TransactionTimeout}, nil
}

// Ping verifies that PostgreSQL is currently available within the query timeout.
func (p *Pool) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, p.queryTimeout)
	defer cancel()
	return p.Pool.Ping(ctx)
}

// QueryContext applies the configured deadline to a single database operation.
func (p *Pool) QueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.queryTimeout)
}

// TransactionContext applies the configured deadline to a transaction.
func (p *Pool) TransactionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.transactionTimeout)
}
