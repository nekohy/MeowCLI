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

	d.SetMaxOpenConns(4)
	d.SetMaxIdleConns(2)

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
	if err := ensureModelPluginColumn(ctx, d); err != nil {
		d.Close()
		return nil, err
	}
	if err := ensureCodexQuota1moColumns(ctx, d); err != nil {
		d.Close()
		return nil, err
	}
	if err := ensureCodexQuotaResetCreditsColumn(ctx, d); err != nil {
		d.Close()
		return nil, err
	}

	return &Store{
		db:      d,
		queries: sqlcsqlite.New(d),
	}, nil
}

func ensureCodexQuota1moColumns(ctx context.Context, d *sql.DB) error {
	columns, err := sqliteColumnSet(ctx, d, "codex_quota")
	if err != nil {
		return err
	}
	additions := map[string]string{
		"quota_1mo":       "ALTER TABLE codex_quota ADD COLUMN quota_1mo REAL NOT NULL DEFAULT 1.0",
		"reset_1mo":       "ALTER TABLE codex_quota ADD COLUMN reset_1mo TEXT",
		"quota_spark_1mo": "ALTER TABLE codex_quota ADD COLUMN quota_spark_1mo REAL NOT NULL DEFAULT 1.0",
		"reset_spark_1mo": "ALTER TABLE codex_quota ADD COLUMN reset_spark_1mo TEXT",
	}
	for name, stmt := range additions {
		if columns[name] {
			continue
		}
		if _, err := d.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite add codex_quota.%s column: %w", name, err)
		}
	}
	return nil
}

func ensureCodexQuotaResetCreditsColumn(ctx context.Context, d *sql.DB) error {
	columns, err := sqliteColumnSet(ctx, d, "codex_quota")
	if err != nil {
		return err
	}
	if columns["reset_credits_count"] {
		return nil
	}
	if _, err := d.ExecContext(ctx, "ALTER TABLE codex_quota ADD COLUMN reset_credits_count INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("sqlite add codex_quota.reset_credits_count column: %w", err)
	}
	return nil
}

func sqliteColumnSet(ctx context.Context, d *sql.DB, table string) (map[string]bool, error) {
	rows, err := d.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, fmt.Errorf("sqlite inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("sqlite scan %s schema: %w", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite read %s schema: %w", table, err)
	}
	return columns, nil
}

func ensureModelPluginColumn(ctx context.Context, d *sql.DB) error {
	columns, err := sqliteColumnSet(ctx, d, "models")
	if err != nil {
		return err
	}
	if columns["plugin"] {
		return nil
	}
	if _, err := d.ExecContext(ctx, "ALTER TABLE models ADD COLUMN plugin TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("sqlite add models.plugin column: %w", err)
	}
	return nil
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
