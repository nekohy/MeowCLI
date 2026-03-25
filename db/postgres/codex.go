package postgres

import (
	"context"
	"strings"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledCodex(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledCodex(ctx)
}

func (s *Store) CountCodex(ctx context.Context) (int64, error) {
	return s.queries.CountCodex(ctx)
}

func (s *Store) CountCodexFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountCodexFiltered(ctx, sqlcpostgres.CountCodexFilteredParams{
		Search:       postgresCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		PlanType:     strings.ToLower(strings.TrimSpace(filter.PlanType)),
		UnsyncedOnly: filter.UnsyncedOnly,
	})
}

func (s *Store) GetCodex(ctx context.Context, id string) (db.Codex, error) {
	row, err := s.queries.GetCodex(ctx, id)
	if err != nil {
		return db.Codex{}, wrapError(err)
	}
	return codexTo(row), nil
}

func (s *Store) UpdateCodexTokens(ctx context.Context, arg db.UpdateCodexTokensParams) (db.Codex, error) {
	row, err := s.queries.UpdateCodexTokens(ctx, sqlcpostgres.UpdateCodexTokensParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		Expired:      tsFrom(arg.Expired),
		RefreshToken: arg.RefreshToken,
		PlanType:     arg.PlanType,
		PlanExpired:  tsFrom(arg.PlanExpired),
	})
	if err != nil {
		return db.Codex{}, wrapError(err)
	}
	return codexTo(row), nil
}

func (s *Store) ListCodex(ctx context.Context) ([]db.ListCodexRow, error) {
	rows, err := s.queries.ListCodex(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListCodexRow, len(rows))
	for i, row := range rows {
		resolved[i] = listCodexRowTo(
			row.ID,
			row.Status,
			row.AccessToken,
			tsTo(row.Expired),
			row.RefreshToken,
			row.PlanType,
			tsTo(row.PlanExpired),
			row.Reason,
			row.Quota5h,
			row.Quota7d,
			tsTo(row.Reset5h),
			tsTo(row.Reset7d),
			tsTo(row.ThrottledUntil),
			tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func (s *Store) ListCodexPaged(ctx context.Context, arg db.ListCodexPagedParams) ([]db.ListCodexRow, error) {
	rows, err := s.queries.ListCodexPaged(ctx, sqlcpostgres.ListCodexPagedParams{
		Search:       postgresCodexSearchPattern(arg.Search),
		Status:       strings.TrimSpace(arg.Status),
		PlanType:     strings.ToLower(strings.TrimSpace(arg.PlanType)),
		UnsyncedOnly: arg.UnsyncedOnly,
		PageOffset:   arg.Offset,
		PageLimit:    arg.Limit,
	})
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListCodexRow, len(rows))
	for i, row := range rows {
		resolved[i] = listCodexRowTo(
			row.ID,
			row.Status,
			row.AccessToken,
			tsTo(row.Expired),
			row.RefreshToken,
			row.PlanType,
			tsTo(row.PlanExpired),
			row.Reason,
			row.Quota5h,
			row.Quota7d,
			tsTo(row.Reset5h),
			tsTo(row.Reset7d),
			tsTo(row.ThrottledUntil),
			tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func postgresCodexSearchPattern(search string) string {
	value := strings.TrimSpace(search)
	if value == "" {
		return ""
	}
	return "%" + strings.ToLower(value) + "%"
}

func (s *Store) CreateCodex(ctx context.Context, arg db.CreateCodexParams) (db.Codex, error) {
	row, err := s.queries.CreateCodex(ctx, sqlcpostgres.CreateCodexParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		Expired:      tsFrom(arg.Expired),
		RefreshToken: arg.RefreshToken,
		PlanType:     arg.PlanType,
		PlanExpired:  tsFrom(arg.PlanExpired),
	})
	if err != nil {
		return db.Codex{}, wrapError(err)
	}
	return codexTo(row), nil
}

func (s *Store) DeleteCodex(ctx context.Context, id string) error {
	affected, err := s.queries.DeleteCodex(ctx, id)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateCodexStatus(ctx context.Context, id string, status string, reason string) (db.Codex, error) {
	row, err := s.queries.UpdateCodexStatus(ctx, sqlcpostgres.UpdateCodexStatusParams{
		ID:     id,
		Status: status,
		Reason: reason,
	})
	if err != nil {
		return db.Codex{}, wrapError(err)
	}
	return codexTo(row), nil
}
