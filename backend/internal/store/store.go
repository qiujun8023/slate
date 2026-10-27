package store

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f ../../sqlc.yaml

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"slate/internal/store/db"
)

type Store struct {
	Q    *db.Queries
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{db.New(p), p}, nil
}

// Tx runs f in a transaction with all deferrable constraints deferred, so sort orders
// can be rewritten in one statement.
func (s *Store) Tx(ctx context.Context, f func(*db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
		return err
	}
	if err = f(s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func Missing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// Unique reports a unique violation, optionally on the named constraint.
func Unique(err error, constraint ...string) bool {
	var e *pgconn.PgError
	if !errors.As(err, &e) || e.Code != "23505" {
		return false
	}
	return len(constraint) == 0 || e.ConstraintName == constraint[0]
}
