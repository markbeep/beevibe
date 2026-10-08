// Package migrations embeds the goose migration files so the binary can bring
// any database up to date at startup.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// Up applies every pending migration from the embedded FS.
func Up(ctx context.Context, handle *sql.DB) error {
	goose.SetBaseFS(FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("migrations: set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, handle, "."); err != nil {
		return fmt.Errorf("migrations: up: %w", err)
	}
	return nil
}
