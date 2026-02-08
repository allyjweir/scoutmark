package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/lo"
)

// Migrate runs all .up.sql files from the given directory in order.
func (d *DB) Migrate(ctx context.Context, migrationsDir string) error {
	// Create migrations tracking table
	_, err := d.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Get applied migrations
	rows, err := d.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("querying applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("scanning migration version: %w", err)
		}
		applied[version] = true
	}

	// Find migration files
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	upFiles := lo.Filter(entries, func(e os.DirEntry, _ int) bool {
		return strings.HasSuffix(e.Name(), ".up.sql")
	})

	sort.Slice(upFiles, func(i, j int) bool {
		return upFiles[i].Name() < upFiles[j].Name()
	})

	for _, f := range upFiles {
		version := strings.TrimSuffix(f.Name(), ".up.sql")
		if applied[version] {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, f.Name()))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.Name(), err)
		}

		// Split on semicolons and execute each statement
		statements := lo.Filter(
			strings.Split(string(content), ";"),
			func(s string, _ int) bool {
				return strings.TrimSpace(s) != ""
			},
		)

		for _, stmt := range statements {
			if _, err := d.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("executing migration %s: %w\nStatement: %s", version, err, stmt)
			}
		}

		if _, err := d.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			return fmt.Errorf("recording migration %s: %w", version, err)
		}

		fmt.Printf("Applied migration: %s\n", version)
	}

	return nil
}
