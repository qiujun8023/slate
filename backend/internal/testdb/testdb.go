// Package testdb gives each test its own migrated PostgreSQL schema.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"slate/migrations"
)

// URL creates a schema in SLATE_TEST_DATABASE_URL, applies the migrations there and
// returns a connection string whose sessions use it; the schema is dropped when the test
// ends. Tests that call it are skipped when the variable is unset. Only CREATE on the
// database is needed, not CREATEDB.
func URL(t testing.TB) string {
	t.Helper()
	base := os.Getenv("SLATE_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set SLATE_TEST_DATABASE_URL to run database tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	// citext lives in a shared schema so every test schema can resolve the type.
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS citext"); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "test_" + hex.EncodeToString(b)
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if c, err := pgx.Connect(ctx, base); err == nil {
			_, _ = c.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
			_ = c.Close(ctx)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	cfg, err := pgx.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["timezone"] = "UTC"
	if err := migrations.Apply(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	return u.String()
}
