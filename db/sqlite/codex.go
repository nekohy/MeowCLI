package sqlite

import (
	"context"
	"strings"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledCodex(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledCodex(ctx)
}

func (s *Store) CountCodex(ctx context.Context) (int64, error) {
	return s.queries.CountCodex(ctx)
}

func (s *Store) CountCodexFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountCodexFiltered(ctx, sqlcsqlite.CountCodexFilteredParams{
		Search:       sqliteCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		PlanType:     strings.ToLower(strings.TrimSpace(filter.PlanType)),
		UnsyncedOnly: sqliteBool(filter.UnsyncedOnly),
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
	row, err := s.queries.UpdateCodexTokens(ctx, sqlcsqlite.UpdateCodexTokensParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		Expired:      fmtTime(arg.Expired),
		RefreshToken: arg.RefreshToken,
		PlanType:     arg.PlanType,
		PlanExpired:  fmtTime(arg.PlanExpired),
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
			row.Expired,
			row.RefreshToken,
			row.PlanType,
			row.PlanExpired,
			row.Quota5h,
			row.Quota7d,
			row.Reset5h,
			row.Reset7d,
			row.ThrottledUntil,
			row.SyncedAt,
		)
	}
	return resolved, nil
}

func (s *Store) ListCodexPaged(ctx context.Context, arg db.ListCodexPagedParams) ([]db.ListCodexRow, error) {
	rows, err := s.queries.ListCodexPaged(ctx, sqlcsqlite.ListCodexPagedParams{
		Search:       sqliteCodexSearchPattern(arg.Search),
		Status:       strings.TrimSpace(arg.Status),
		PlanType:     strings.ToLower(strings.TrimSpace(arg.PlanType)),
		UnsyncedOnly: sqliteBool(arg.UnsyncedOnly),
		PageOffset:   int64(arg.Offset),
		PageLimit:    int64(arg.Limit),
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
			row.Expired,
			row.RefreshToken,
			row.PlanType,
			row.PlanExpired,
			row.Quota5h,
			row.Quota7d,
			row.Reset5h,
			row.Reset7d,
			row.ThrottledUntil,
			row.SyncedAt,
		)
	}
	return resolved, nil
}

func sqliteCodexSearchPattern(search string) string {
	value := strings.TrimSpace(search)
	if value == "" {
		return ""
	}
	return "%" + strings.ToLower(value) + "%"
}

func sqliteBool(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (s *Store) CreateCodex(ctx context.Context, arg db.CreateCodexParams) (db.Codex, error) {
	row, err := s.queries.CreateCodex(ctx, sqlcsqlite.CreateCodexParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		Expired:      fmtTime(arg.Expired),
		RefreshToken: arg.RefreshToken,
		PlanType:     arg.PlanType,
		PlanExpired:  fmtTime(arg.PlanExpired),
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

func (s *Store) UpdateCodexStatus(ctx context.Context, id string, status string) (db.Codex, error) {
	row, err := s.queries.UpdateCodexStatus(ctx, sqlcsqlite.UpdateCodexStatusParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		return db.Codex{}, wrapError(err)
	}
	return codexTo(row), nil
}
