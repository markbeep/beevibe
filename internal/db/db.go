// Package db opens the SQLite database and applies the embedded goose
// migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mark/beevibe/internal/db/gen"
	"github.com/mark/beevibe/internal/db/migrations"

	_ "modernc.org/sqlite"
)

// Open opens (creating if needed) the SQLite database at path, applies every
// pending migration and returns a ready pool.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}
	// One writer at a time, a few concurrent readers (WAL).
	handle.SetMaxOpenConns(4)
	if err := handle.PingContext(ctx); err != nil {
		handle.Close()
		return nil, fmt.Errorf("db: ping %s: %w", path, err)
	}
	if err := migrations.Up(ctx, handle); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

// Queries returns the sqlc query set bound to the given handle.
func Queries(handle *sql.DB) *dbgen.Queries {
	return dbgen.New(handle)
}
