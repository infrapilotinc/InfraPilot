package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations runs all SQL migrations in order
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) error {
	// Create migrations tracking table
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Check if this is an existing database (has organizations table but no migration records)
	var hasOrgs bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'organizations'
		)
	`).Scan(&hasOrgs)
	if err != nil {
		return fmt.Errorf("failed to check existing tables: %w", err)
	}

	var hasMigrations bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations LIMIT 1)").Scan(&hasMigrations)
	if err != nil {
		return fmt.Errorf("failed to check migrations: %w", err)
	}

	// If database exists but no migrations tracked, mark base migrations as applied
	if hasOrgs && !hasMigrations {
		logger.Info("Existing database detected, marking base migrations as applied")
		baseMigrations := []string{
			"001_initial_schema.sql",
			"002_network_attachments.sql",
			"003_proxy_settings.sql",
			"004_mfa_tokens.sql",
			"005_seed_data.sql",
			"006_system_settings.sql",
		}
		for _, m := range baseMigrations {
			_, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", m)
			if err != nil {
				return fmt.Errorf("failed to mark migration %s: %w", m, err)
			}
		}
	}

	// Get list of migration files
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Sort migrations by name (they should be numbered like 001_, 002_, etc.)
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Run each migration
	for _, filename := range migrationFiles {
		// Check if already applied
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", filename).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if exists {
			logger.Debug("Migration already applied", zap.String("file", filename))
			continue
		}

		// Read migration file
		content, err := fs.ReadFile(migrationsFS, filepath.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filename, err)
		}

		// Execute migration
		logger.Info("Running migration", zap.String("file", filename))
		_, err = pool.Exec(ctx, string(content))
		if err != nil {
			return fmt.Errorf("failed to run migration %s: %w", filename, err)
		}

		// Mark as applied
		_, err = pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", filename)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		logger.Info("Migration applied", zap.String("file", filename))
	}

	logger.Info("All migrations complete", zap.Int("total", len(migrationFiles)))
	return nil
}
