package sqlite

import (
	"context"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) ListSettings(ctx context.Context) ([]db.Setting, error) {
	rows, err := s.queries.ListSettings(ctx)
	if err != nil {
		return nil, wrapError(err)
	}
	resolved := make([]db.Setting, len(rows))
	for i, row := range rows {
		resolved[i] = settingTo(row.Key, row.Value, row.UpdatedAt)
	}
	return resolved, nil
}

func (s *Store) UpsertSetting(ctx context.Context, arg db.UpsertSettingParams) (db.Setting, error) {
	row, err := s.queries.UpsertSetting(ctx, sqlcsqlite.UpsertSettingParams{
		Key:   arg.Key,
		Value: arg.Value,
	})
	if err != nil {
		return db.Setting{}, wrapError(err)
	}
	return settingTo(row.Key, row.Value, row.UpdatedAt), nil
}
