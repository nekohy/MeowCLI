package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

//go:embed schema/*.sql
var schemaFS embed.FS

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcpostgres.Queries
}

var _ db.Store = (*Store)(nil)

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	log.Info().
		Int32("max_conns", cfg.MaxConns).
		Int32("min_conns", cfg.MinConns).
		Dur("max_conn_lifetime", cfg.MaxConnLifetime).
		Dur("max_conn_idle_time", cfg.MaxConnIdleTime).
		Msg("postgres pool config")

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if err := applySchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return &Store{
		pool:    pool,
		queries: sqlcpostgres.New(pool),
	}, nil
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}

	s.pool.Close()
}

func (s *Store) SaveSettings(ctx context.Context, settings []db.UpsertSettingParams) error {
	if s == nil {
		return nil
	}
	if len(settings) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	for _, setting := range settings {
		if _, err := queries.UpsertSetting(ctx, sqlcpostgres.UpsertSettingParams{
			Key:   setting.Key,
			Value: setting.Value,
		}); err != nil {
			return wrapError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	paths, err := fs.Glob(schemaFS, "schema/*.sql")
	if err != nil {
		return fmt.Errorf("glob postgres schema: %w", err)
	}
	sort.Strings(paths)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire postgres schema connection: %w", err)
	}
	defer conn.Release()

	for _, path := range paths {
		sqlText, err := fs.ReadFile(schemaFS, path)
		if err != nil {
			return fmt.Errorf("read postgres schema %s: %w", path, err)
		}
		if len(sqlText) == 0 {
			continue
		}
		if _, err := conn.Conn().PgConn().Exec(ctx, string(sqlText)).ReadAll(); err != nil {
			return fmt.Errorf("apply postgres schema %s: %w", path, err)
		}
	}

	return nil
}
