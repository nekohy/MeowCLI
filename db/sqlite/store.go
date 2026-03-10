package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"

	_ "modernc.org/sqlite"
)

const timeLayout = "2006-01-02 15:04:05"

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db      *sql.DB
	queries *sqlcsqlite.Queries
}

var _ db.Store = (*Store)(nil)

func Open(_ context.Context, dsn string) (*Store, error) {
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := d.Exec("PRAGMA journal_mode=WAL"); err != nil {
		d.Close()
		return nil, fmt.Errorf("sqlite pragma WAL: %w", err)
	}

	if _, err := d.Exec(schemaSQL); err != nil {
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
