package db

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var safeIdent = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func Connect(ctx context.Context, databaseURL, searchPath string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	var quoted string
	if searchPath != "" {
		if !safeIdent.MatchString(searchPath) {
			return nil, fmt.Errorf("invalid DB_SEARCH_PATH")
		}
		// Neon pooler (transaction mode) drops session SET between checkouts.
		// Re-apply search_path on every acquire.
		quoted = pgx.Identifier{searchPath}.Sanitize()
		setSQL := "SET search_path TO " + quoted + ", public"
		cfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
			if _, err := conn.Exec(ctx, setSQL); err != nil {
				slog.Error("set search_path failed", "err", err, "schema", searchPath)
				return false, err
			}
			return true, nil
		}
		cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, setSQL)
			return err
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if quoted != "" {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoted); err != nil {
			pool.Close()
			return nil, fmt.Errorf("create schema: %w", err)
		}
	}
	return pool, nil
}

// EnsureAppTables fails fast when migrations/search_path are wrong.
func EnsureAppTables(ctx context.Context, pool *pgxpool.Pool) error {
	var sessions, tokens *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('sessions')::text, to_regclass('api_tokens')::text`).
		Scan(&sessions, &tokens); err != nil {
		return fmt.Errorf("check tables: %w", err)
	}
	if sessions == nil || tokens == nil {
		return fmt.Errorf("required tables missing in search_path (sessions=%v api_tokens=%v); check DB_SEARCH_PATH and migrations", sessions, tokens)
	}
	return nil
}
