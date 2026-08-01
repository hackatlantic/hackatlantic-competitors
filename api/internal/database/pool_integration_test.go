package database

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRoleAssumptionIntegration(t *testing.T) {
	url := os.Getenv("DATABASE_INTEGRATION_URL")
	role := os.Getenv("DATABASE_INTEGRATION_ROLE")
	if url == "" || role == "" {
		t.Skip("DATABASE_INTEGRATION_URL and DATABASE_INTEGRATION_ROLE are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := Open(ctx, Config{URL: url, Role: role, MaxConns: 1})
	if err != nil {
		t.Fatalf("open role-scoped pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping role-scoped pool: %v", err)
	}
	var currentUser string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&currentUser); err != nil {
		t.Fatalf("query current role: %v", err)
	}
	if currentUser != role {
		t.Fatalf("expected current_user %q, got %q", role, currentUser)
	}
}
