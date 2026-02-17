package persistence

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

var (
	//go:embed migrations/0001_init.up.sql
	migration0001Up string
	//go:embed migrations/0002_resources.up.sql
	migration0002Up string
)

func MigratePostgres(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("nil database handle")
	}
	// Serialize migration DDL across concurrent processes/tests.
	// This avoids catalog races when multiple callers run bootstrap simultaneously.
	if _, err := db.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, int64(64250423391944124)); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(64250423391944124))
	}()

	if _, err := db.ExecContext(ctx, migration0001Up); err != nil {
		return fmt.Errorf("apply migration 0001_init.up.sql: %w", err)
	}
	if _, err := db.ExecContext(ctx, migration0002Up); err != nil {
		return fmt.Errorf("apply migration 0002_resources.up.sql: %w", err)
	}
	return nil
}
