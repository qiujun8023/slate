// Package migrations embeds the schema history and applies new files in name order.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed *.sql
var files embed.FS

// lockID serializes migrators sharing a database.
const lockID = 0x534c4154

// Apply records each file in schema_migrations. Applied files are verified by
// checksum and must never be edited; schema changes go into a new file.
func Apply(ctx context.Context, cfg *pgx.ConnConfig) error {
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err = conn.Exec(ctx, "SET lock_timeout = '10s'"); err != nil {
		return err
	}
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		checksum TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	for _, file := range names {
		name := strings.TrimSuffix(file, ".sql")
		sql, err := files.ReadFile(file)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(sql)
		checksum := hex.EncodeToString(sum[:])
		var stored string
		err = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name = $1", name).Scan(&stored)
		if err == nil {
			if stored != checksum {
				return fmt.Errorf("migration %s checksum mismatch", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (name, checksum) VALUES ($1, $2)", name, checksum)
			return err
		})
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		slog.Info("migration applied", "name", name)
	}
	return nil
}
