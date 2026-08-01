// Package migrations applies the repository's forward-only PostgreSQL schema migrations.
package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
)

//go:embed *.sql
var files embed.FS

const advisoryLockID int64 = 740159346018

type transactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Apply brings a database from zero to the latest checked-in schema version.
// It serializes application, stores migration checksums, and rejects an edited
// migration that has already been recorded.
func Apply(ctx context.Context, db transactionBeginner) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_catalog.pg_advisory_xact_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if err := initializeLedger(ctx, tx); err != nil {
		return err
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version := entry.Name()
		source, err := files.ReadFile(version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		checksum := migrationChecksum(source)

		ledger, err := ledgerTable(ctx, tx)
		if err != nil {
			return err
		}
		var storedChecksum *string
		err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT checksum FROM %s WHERE version = $1`, ledger), version).Scan(&storedChecksum)
		if err == nil {
			if storedChecksum == nil {
				return fmt.Errorf("migration %s has no recorded checksum; attest the legacy migration before applying further changes", version)
			}
			if *storedChecksum != checksum {
				return fmt.Errorf("migration %s checksum does not match the recorded migration", version)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(source)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		ledger, err = ledgerTable(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (version, checksum) VALUES ($1, $2)`, ledger), version, checksum); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func initializeLedger(ctx context.Context, tx pgx.Tx) error {
	var publicLedger bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&publicLedger); err != nil {
		return fmt.Errorf("locate migration ledger: %w", err)
	}
	if publicLedger {
		if _, err := tx.Exec(ctx, `ALTER TABLE public.schema_migrations ADD COLUMN IF NOT EXISTS checksum text`); err != nil {
			return fmt.Errorf("upgrade legacy migration ledger: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS ats`); err != nil {
		return fmt.Errorf("create ATS schema: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS ats.schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	return nil
}

func ledgerTable(ctx context.Context, tx pgx.Tx) (string, error) {
	var atsLedger bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('ats.schema_migrations') IS NOT NULL`).Scan(&atsLedger); err != nil {
		return "", fmt.Errorf("locate ATS migration ledger: %w", err)
	}
	if atsLedger {
		return "ats.schema_migrations", nil
	}
	var publicLedger bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&publicLedger); err != nil {
		return "", fmt.Errorf("locate legacy migration ledger: %w", err)
	}
	if publicLedger {
		return "public.schema_migrations", nil
	}
	return "", fmt.Errorf("migration ledger does not exist")
}

func migrationChecksum(source []byte) string {
	normalized := bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	hash := sha256.Sum256(normalized)
	return hex.EncodeToString(hash[:])
}
