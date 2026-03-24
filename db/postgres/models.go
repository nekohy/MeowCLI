package postgres

import (
	"context"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) ReverseInfoFromModel(ctx context.Context, alias string) (db.ReverseInfoFromModelRow, error) {
	row, err := s.queries.ReverseInfoFromModel(ctx, alias)
	if err != nil {
		return db.ReverseInfoFromModelRow{}, wrapError(err)
	}
	return reverseInfoTo(row.Origin, row.Handler, row.Extra), nil
}

func (s *Store) ListModels(ctx context.Context) ([]db.ModelRow, error) {
	rows, err := s.queries.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ModelRow, len(rows))
	for i, row := range rows {
		resolved[i] = modelRowTo(row.Alias, row.Origin, row.Handler, row.Extra)
	}
	return resolved, nil
}

func (s *Store) CountModels(ctx context.Context) (int64, error) {
	return s.queries.CountModels(ctx)
}

func (s *Store) CountModelsByHandler(ctx context.Context, handler string) (int64, error) {
	return s.queries.CountModelsByHandler(ctx, handler)
}

func (s *Store) CreateModel(ctx context.Context, arg db.CreateModelParams) (db.ModelRow, error) {
	row, err := s.queries.CreateModel(ctx, sqlcpostgres.CreateModelParams{
		Alias:   arg.Alias,
		Origin:  arg.Origin,
		Handler: arg.Handler,
		Extra:   arg.Extra,
	})
	if err != nil {
		return db.ModelRow{}, wrapError(err)
	}
	return modelRowTo(row.Alias, row.Origin, row.Handler, row.Extra), nil
}

func (s *Store) UpdateModel(ctx context.Context, arg db.UpdateModelParams) (db.ModelRow, error) {
	row, err := s.queries.UpdateModel(ctx, sqlcpostgres.UpdateModelParams{
		Alias:   arg.Alias,
		Origin:  arg.Origin,
		Handler: arg.Handler,
		Extra:   arg.Extra,
	})
	if err != nil {
		return db.ModelRow{}, wrapError(err)
	}
	return modelRowTo(row.Alias, row.Origin, row.Handler, row.Extra), nil
}

func (s *Store) DeleteModel(ctx context.Context, alias string) error {
	affected, err := s.queries.DeleteModel(ctx, alias)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}
