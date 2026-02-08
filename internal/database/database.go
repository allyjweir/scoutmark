package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyjweir/scoutmark/internal/tracing"
)

// DB wraps *sql.DB with traced query methods.
type DB struct {
	*sql.DB
}

// Connect opens a PostgreSQL connection pool using DATABASE_URL from the environment.
func Connect(ctx context.Context) (*DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://scoutmark:scoutmark@localhost:5432/scoutmark?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB{DB: db}, nil
}

// QueryContext executes a traced query.
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := tracing.Tracer().Start(ctx, "db.query")
	defer span.End()

	span.SetAttributes(
		attribute.String("db.statement", truncateQuery(query)),
		attribute.Int("db.args_count", len(args)),
	)

	rows, err := d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
	}
	return rows, err
}

// QueryRowContext executes a traced query returning a single row.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, span := tracing.Tracer().Start(ctx, "db.query_row")
	defer span.End()

	span.SetAttributes(
		attribute.String("db.statement", truncateQuery(query)),
		attribute.Int("db.args_count", len(args)),
	)

	return d.DB.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a traced statement.
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, span := tracing.Tracer().Start(ctx, "db.exec")
	defer span.End()

	span.SetAttributes(
		attribute.String("db.statement", truncateQuery(query)),
		attribute.Int("db.args_count", len(args)),
	)

	result, err := d.DB.ExecContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
	}
	return result, err
}

// InTx runs a function within a database transaction, with tracing.
func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	ctx, span := tracing.Tracer().Start(ctx, "db.transaction")
	defer span.End()

	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("beginning transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		span.RecordError(err)
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func truncateQuery(q string) string {
	if len(q) > 500 {
		return q[:500] + "..."
	}
	return q
}
