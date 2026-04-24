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
			if _, err := conn.ExecContext(context.Background(), "PRAGMA foreign_keys=ON", nil); err != nil {
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
	if _, err := d.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}

	if _, err := d.ExecContext(ctx, schemaSQL); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite apply schema: %w", err)
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
