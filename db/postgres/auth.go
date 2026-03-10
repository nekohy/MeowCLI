package postgres

import (
	"context"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"

	"github.com/jackc/pgx/v5"
)

const authKeysLockID int64 = 0x4D434C4941555448

func (s *Store) beginLockedAuthKeyTx(ctx context.Context) (pgx.Tx, *sqlcpostgres.Queries, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", authKeysLockID); err != nil {
		_ = tx.Rollback(context.Background())
		return nil, nil, wrapError(err)
	}
	return tx, s.queries.WithTx(tx), nil
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
	tx, queries, err := s.beginLockedAuthKeyTx(ctx)
	if err != nil {
		return db.AuthKey{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	adminCount, err := queries.CountAuthKeysByRole(ctx, "admin")
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	if adminCount > 0 {
		return db.AuthKey{}, db.ErrAlreadyInitialized
	}

	row, err := queries.CreateAuthKey(ctx, sqlcpostgres.CreateAuthKeyParams{
		Key:  arg.Key,
		Role: arg.Role,
		Note: arg.Note,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	committed = true
	return authKeyTo(row), nil
}

func (s *Store) CreateAuthKey(ctx context.Context, arg db.CreateAuthKeyParams) (db.AuthKey, error) {
	row, err := s.queries.CreateAuthKey(ctx, sqlcpostgres.CreateAuthKeyParams{
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
	row, err := s.queries.UpdateAuthKey(ctx, sqlcpostgres.UpdateAuthKeyParams{
		Key:  key,
		Role: role,
		Note: note,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}
	return authKeyTo(row), nil
}

func (s *Store) UpdateAuthKeyChecked(ctx context.Context, key string, role string, note string) (db.AuthKey, error) {
	tx, queries, err := s.beginLockedAuthKeyTx(ctx)
	if err != nil {
		return db.AuthKey{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
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

	row, err := queries.UpdateAuthKey(ctx, sqlcpostgres.UpdateAuthKeyParams{
		Key:  key,
		Role: role,
		Note: note,
	})
	if err != nil {
		return db.AuthKey{}, wrapError(err)
	}

	if err := tx.Commit(ctx); err != nil {
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
	tx, queries, err := s.beginLockedAuthKeyTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
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

	if err := tx.Commit(ctx); err != nil {
		return wrapError(err)
	}
	committed = true
	return nil
}

func (s *Store) CountAuthKeysByRole(ctx context.Context, role string) (int64, error) {
	return s.queries.CountAuthKeysByRole(ctx, role)
}
