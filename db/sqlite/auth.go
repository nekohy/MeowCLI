package sqlite

import (
	"context"
	"database/sql"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) beginImmediateAuthKeyConn(ctx context.Context) (*sql.Conn, *sqlcsqlite.Queries, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		conn.Close()
		return nil, nil, wrapError(err)
	}
	return conn, sqlcsqlite.New(conn), nil
}

func (s *Store) ListAuthKeys(ctx context.Context) ([]db.AuthKey, error) {
	rows, err := s.queries.ListAuthKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]db.AuthKey, len(rows))
	for i, row := range rows {
		out[i] = authKeyTo(row)
	}
	return out, nil
}

func (s *Store) GetAuthKey(ctx context.Context, key string) (db.AuthKey, error) {
	row, err := s.queries.GetAuthKey(ctx, key)
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	return authKeyTo(row), nil
}

func (s *Store) CreateInitialAuthKey(ctx context.Context, arg db.CreateAuthKeyParams) (db.AuthKey, error) {
	conn, queries, err := s.beginImmediateAuthKeyConn(ctx)
	if err != nil {
		return db.AuthKey{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
		_ = conn.Close()
	}()

	adminCount, err := queries.CountAuthKeysByRole(ctx, "admin")
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	if adminCount > 0 {
		return db.AuthKey{}, db.ErrAlreadyInitialized
	}

	row, err := queries.CreateAuthKey(ctx, sqlcsqlite.CreateAuthKeyParams{
		Key:  arg.Key,
		Role: arg.Role,
		Note: arg.Note,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	committed = true
	return authKeyTo(row), nil
}

func (s *Store) CreateAuthKey(ctx context.Context, arg db.CreateAuthKeyParams) (db.AuthKey, error) {
	row, err := s.queries.CreateAuthKey(ctx, sqlcsqlite.CreateAuthKeyParams{
		Key:  arg.Key,
		Role: arg.Role,
		Note: arg.Note,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	return authKeyTo(row), nil
}

func (s *Store) UpdateAuthKey(ctx context.Context, key string, role string, note string) (db.AuthKey, error) {
	row, err := s.queries.UpdateAuthKey(ctx, sqlcsqlite.UpdateAuthKeyParams{
		Role: role,
		Note: note,
		Key:  key,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	return authKeyTo(row), nil
}

func (s *Store) UpdateAuthKeyChecked(ctx context.Context, key string, role string, note string) (db.AuthKey, error) {
	conn, queries, err := s.beginImmediateAuthKeyConn(ctx)
	if err != nil {
		return db.AuthKey{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
		_ = conn.Close()
	}()

	existing, err := queries.GetAuthKey(ctx, key)
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}

	if role == "user" && existing.Role == "admin" {
		adminCount, err := queries.CountAuthKeysByRole(ctx, "admin")
		if err != nil {
			return db.AuthKey{}, wrapError(err)
		}
		if adminCount <= 1 {
			return db.AuthKey{}, db.ErrLastAdmin
		}
	}

	row, err := queries.UpdateAuthKey(ctx, sqlcsqlite.UpdateAuthKeyParams{
		Role: role,
		Note: note,
		Key:  key,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	committed = true
	return authKeyTo(row), nil
}

func (s *Store) DeleteAuthKey(ctx context.Context, key string) error {
	affected, err := s.queries.DeleteAuthKey(ctx, key)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAuthKeyChecked(ctx context.Context, key string) error {
	conn, queries, err := s.beginImmediateAuthKeyConn(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
		_ = conn.Close()
	}()

	existing, err := queries.GetAuthKey(ctx, key)
	if err != nil {
		return wrapError(err)
	}

	if existing.Role == "admin" {
		adminCount, err := queries.CountAuthKeysByRole(ctx, "admin")
		if err != nil {
			return wrapError(err)
		}
		if adminCount <= 1 {
			return db.ErrLastAdmin
		}
	}

	affected, err := queries.DeleteAuthKey(ctx, key)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return wrapError(err)
	}
	committed = true
	return nil
}

func (s *Store) CountAuthKeysByRole(ctx context.Context, role string) (int64, error) {
	return s.queries.CountAuthKeysByRole(ctx, role)
}
