// Package sqlite provides SQLite persistence with embedded migrations and transparent encryption.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers "sqlite" driver
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Options configures DB connection.
type Options struct {
	Path        string
	BusyTimeout time.Duration
	Logger      *slog.Logger
}

// DB wraps an active SQLite connection.
type DB struct {
	sql  *sql.DB
	path string
	log  *slog.Logger
}

// Open opens an SQLite database and executes pending embedded migrations.
func Open(ctx context.Context, o Options) (*DB, error) {
	if o.Path == "" {
		return nil, errors.New("sqlite: path is required")
	}
	if o.BusyTimeout <= 0 {
		o.BusyTimeout = 5 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}

	if o.Path != ":memory:" && !strings.HasPrefix(o.Path, "file:") {
		if err := os.MkdirAll(filepath.Dir(o.Path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create directory for %s: %w", o.Path, err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		o.Path, o.BusyTimeout.Milliseconds())
	if o.Path == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)"
	}

	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", o.Path, err)
	}

	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	raw.SetConnMaxLifetime(0)

	if err := raw.PingContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("sqlite: ping %s: %w", o.Path, err)
	}

	db := &DB{
		sql:  raw,
		path: o.Path,
		log:  o.Logger,
	}

	if err := db.migrate(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	return db, nil
}

// Raw returns the underlying *sql.DB for advanced queries or testing.
func (db *DB) Raw() *sql.DB {
	return db.sql
}

// Close closes the database connection.
func (db *DB) Close() error {
	if db.path != ":memory:" {
		_, _ = db.sql.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	}
	return db.sql.Close()
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return fmt.Errorf("create _schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		name := entry.Name()
		var applied int
		row := db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM _schema_migrations WHERE name = ?", name)
		if err := row.Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", name, err)
		}

		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx: %w", err)
		}

		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO _schema_migrations (name) VALUES (?)", name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}

		db.log.Info("sqlite: applied migration", "name", name)
	}

	return nil
}
