package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"sync"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"

	moderncsqlite "modernc.org/sqlite"
)

const timeLayout = "2006-01-02 15:04:05"

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db      *sql.DB
	queries *sqlcsqlite.Queries
}

var _ db.Store = (*Store)(nil)

var registerHookOnce sync.Once

func Open(ctx context.Context, dsn string) (*Store, error) {
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	d.SetMaxOpenConns(1)
	d.SetMaxIdleConns(1)

	registerHookOnce.Do(func() {
		moderncsqlite.RegisterConnectionHook(func(conn moderncsqlite.ExecQuerierContext, _ string) error {
			if _, err := conn.ExecContext(context.Background(), "PRAGMA journal_mode=WAL", nil); err != nil {
				return err
			}
			if _, err := conn.ExecContext(context.Background(), "PRAGMA busy_timeout=5000", nil); err != nil {
				return err
			}
			return nil
		})
	})

	if _, err := d.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite pragma WAL: %w", err)
	}
	if _, err := d.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite pragma busy_timeout: %w", err)
	}

	if _, err := d.ExecContext(ctx, schemaSQL); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite apply schema: %w", err)
	}

	if err := migrateSchema(ctx, d); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite migrate schema: %w", err)
	}

	return &Store{
		db:      d,
		queries: sqlcsqlite.New(d),
	}, nil
}

func (s *Store) Close() {
	if s == nil || s.db == nil {
		return
	}
	s.db.Close()
}

// migrateSchema applies incremental schema changes for existing databases.
// New databases already have the latest schema from schema.sql.
func migrateSchema(ctx context.Context, d *sql.DB) error {
	if err := addColumnIfNotExists(ctx, d, "codex", "reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS gemini_cli (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expired TEXT NOT NULL,
    email TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    plan_type TEXT NOT NULL DEFAULT 'free',
    reason TEXT NOT NULL DEFAULT '',
    throttled_until TEXT NOT NULL DEFAULT (datetime('now')),
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
)`); err != nil {
		return fmt.Errorf("create gemini_cli table: %w", err)
	}
	if err := addColumnIfNotExists(ctx, d, "models", "plan_types", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

// addColumnIfNotExists checks via PRAGMA table_info and adds the column if missing.
func addColumnIfNotExists(ctx context.Context, d *sql.DB, table, column, colDef string) error {
	rows, err := d.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if name == column {
			return nil // column already exists
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = d.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef))
	if err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}

func (s *Store) SaveSettings(ctx context.Context, settings []db.UpsertSettingParams) error {
	if s == nil {
		return nil
	}
	if len(settings) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlcsqlite.New(tx)
	for _, setting := range settings {
		if _, err := queries.UpsertSetting(ctx, sqlcsqlite.UpsertSettingParams{
			Key:   setting.Key,
			Value: setting.Value,
		}); err != nil {
			return wrapError(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
