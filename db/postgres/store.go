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
)

//go:embed schema/*.sql
var schemaFS embed.FS

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcpostgres.Queries
}

var _ db.Store = (*Store)(nil)

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
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
