// Package migrate provides automatic database migration with version tracking.
//
// Migrations are embedded in the binary at compile time, ensuring the application
// always has access to its required schema changes without external file dependencies.
//
// # Usage
//
// Call Run() after establishing a database connection but before starting services:
//
//	pool, _ := pgxpool.New(ctx, databaseURL)
//	if err := migrate.Run(ctx, pool, logger); err != nil {
//	    log.Fatal("migration failed:", err)
//	}
//
// # Migration Files
//
// Migrations are SQL files in the db/migrations directory with the format:
//
//	NNN_descriptive_name.sql
//
// Where NNN is a zero-padded version number (001, 002, ..., 021).
// Migrations are applied in version order and each is run in a transaction.
//
// # Version Tracking
//
// Applied migrations are tracked in the schema_migrations table:
//
//	CREATE TABLE schema_migrations (
//	    version INTEGER PRIMARY KEY,
//	    name TEXT NOT NULL,
//	    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//
// This ensures migrations are only applied once and allows rollback tracking.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Record represents a completed migration in the database.
type Record struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// Status contains information about the current migration state.
type Status struct {
	Applied []Record `json:"applied"`
	Pending []string `json:"pending"`
}

// Run executes all pending database migrations.
//
// It creates the schema_migrations table if it doesn't exist,
// then applies any migrations that haven't been run yet.
// Each migration runs in its own transaction.
//
// This function should be called after database connection but before
// any application services start.
func Run(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	logger.Info("checking database migrations")

	// Ensure schema_migrations table exists
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Get list of applied migrations
	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("getting applied migrations: %w", err)
	}
	appliedSet := make(map[int]bool)
	for _, m := range applied {
		appliedSet[m.Version] = true
	}

	// Get list of available migrations
	available, err := getAvailableMigrations()
	if err != nil {
		return fmt.Errorf("reading migration files: %w", err)
	}

	// Apply pending migrations in order
	pendingCount := 0
	for _, mig := range available {
		if appliedSet[mig.version] {
			continue
		}

		logger.Info("applying migration",
			"version", mig.version,
			"name", mig.name,
		)

		if err := applyMigration(ctx, pool, mig); err != nil {
			return fmt.Errorf("applying migration %03d_%s: %w", mig.version, mig.name, err)
		}

		pendingCount++
		logger.Info("migration applied successfully",
			"version", mig.version,
			"name", mig.name,
		)
	}

	if pendingCount == 0 {
		logger.Info("database schema is up to date",
			"version", len(applied),
		)
	} else {
		logger.Info("migrations complete",
			"applied", pendingCount,
			"total", len(applied)+pendingCount,
		)
	}

	return nil
}

// GetStatus returns the current migration status for diagnostics.
func GetStatus(ctx context.Context, pool *pgxpool.Pool) (*Status, error) {
	// Check if migrations table exists
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'schema_migrations'
		)
	`).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("checking migrations table: %w", err)
	}

	status := &Status{}

	if exists {
		status.Applied, err = getAppliedMigrations(ctx, pool)
		if err != nil {
			return nil, err
		}
	}

	appliedSet := make(map[int]bool)
	for _, m := range status.Applied {
		appliedSet[m.Version] = true
	}

	available, err := getAvailableMigrations()
	if err != nil {
		return nil, err
	}

	for _, m := range available {
		if !appliedSet[m.version] {
			status.Pending = append(status.Pending, fmt.Sprintf("%03d_%s", m.version, m.name))
		}
	}

	return status, nil
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

// getAppliedMigrations returns all migrations that have been applied.
func getAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) ([]Record, error) {
	rows, err := pool.Query(ctx, `
		SELECT version, name, applied_at
		FROM schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []Record
	for rows.Next() {
		var m Record
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}

	return migrations, rows.Err()
}

// migration represents a migration file to be applied.
type migration struct {
	version int
	name    string
	sql     string
}

// getAvailableMigrations reads all migration files from the embedded filesystem.
func getAvailableMigrations() ([]migration, error) {
	var migrations []migration

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, err := parseMigrationFilename(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parsing migration filename %s: %w", entry.Name(), err)
		}

		content, err := fs.ReadFile(migrationsFS, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    name,
			sql:     string(content),
		})
	}

	// Sort by version number
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// parseMigrationFilename extracts version and name from a migration filename.
// Expected format: NNN_name.sql (e.g., "001_initial_schema.sql")
func parseMigrationFilename(filename string) (int, string, error) {
	// Remove .sql extension
	base := strings.TrimSuffix(filename, ".sql")

	// Split on first underscore
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration filename format: %s (expected NNN_name.sql)", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid version number in %s: %w", filename, err)
	}

	return version, parts[1], nil
}

// applyMigration executes a single migration within a transaction.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, mig migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx) // No-op if committed

	// Execute the migration SQL
	// Note: We use Exec which handles multiple statements
	if _, err := tx.Exec(ctx, mig.sql); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	// Record the migration
	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (version, name) VALUES ($1, $2)
	`, mig.version, mig.name); err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// Rollback removes the last applied migration from the tracking table.
// Note: This does NOT undo the migration SQL - that must be done manually.
// This is primarily for development/testing purposes.
func Rollback(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	var version int
	var name string

	err := pool.QueryRow(ctx, `
		SELECT version, name FROM schema_migrations
		ORDER BY version DESC LIMIT 1
	`).Scan(&version, &name)
	if err == pgx.ErrNoRows {
		logger.Info("no migrations to rollback")
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting last migration: %w", err)
	}

	_, err = pool.Exec(ctx, `
		DELETE FROM schema_migrations WHERE version = $1
	`, version)
	if err != nil {
		return fmt.Errorf("removing migration record: %w", err)
	}

	logger.Info("migration record removed (SQL not reverted)",
		"version", version,
		"name", name,
	)

	return nil
}
