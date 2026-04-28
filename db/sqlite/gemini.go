package sqlite

import (
	"context"
	"strings"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledGeminiCLI(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledGeminiCLI(ctx)
}

func (s *Store) CountGeminiCLI(ctx context.Context) (int64, error) {
	return s.queries.CountGeminiCLI(ctx)
}

func (s *Store) CountGeminiCLIFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountGeminiCLIFiltered(ctx, sqlcsqlite.CountGeminiCLIFilteredParams{
		Search:       sqliteCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		PlanType:     strings.TrimSpace(filter.PlanType),
		UnsyncedOnly: sqliteBool(filter.UnsyncedOnly),
	})
}

func (s *Store) GetGeminiCLI(ctx context.Context, id string) (db.GeminiCredential, error) {
	row, err := s.queries.GetGeminiCLI(ctx, id)
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	return geminiCredentialTo(row), nil
}

func (s *Store) UpdateGeminiTokens(ctx context.Context, arg db.UpdateGeminiTokensParams) (db.GeminiCredential, error) {
	row, err := s.queries.UpdateGeminiTokens(ctx, sqlcsqlite.UpdateGeminiTokensParams{
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      fmtTime(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		ID:           arg.ID,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	return geminiCredentialTo(row), nil
}

func (s *Store) UpdateGeminiPlanType(ctx context.Context, id string, planType string) (db.GeminiCredential, error) {
	row, err := s.queries.UpdateGeminiPlanType(ctx, sqlcsqlite.UpdateGeminiPlanTypeParams{
		ID:       id,
		PlanType: strings.TrimSpace(planType),
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	return geminiCredentialTo(row), nil
}

func (s *Store) ListGeminiCLI(ctx context.Context) ([]db.ListGeminiCLIRow, error) {
	rows, err := s.queries.ListGeminiCLI(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListGeminiCLIRow, len(rows))
	for i, row := range rows {
		resolved[i] = listGeminiCLIRowTo(
			row.ID,
			row.Status,
			row.AccessToken,
			row.RefreshToken,
			row.Expired,
			row.Email,
			row.ProjectID,
			row.PlanType,
			row.Reason,
			row.QuotaPro,
			row.QuotaFlash,
			row.QuotaFlashlite,
			row.ResetPro,
			row.ResetFlash,
			row.ResetFlashlite,
			row.ThrottledUntil,
			row.SyncedAt,
		)
	}
	return resolved, nil
}

func (s *Store) ListGeminiCLIPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListGeminiCLIRow, error) {
	rows, err := s.queries.ListGeminiCLIPaged(ctx, sqlcsqlite.ListGeminiCLIPagedParams{
		Search:       sqliteCodexSearchPattern(arg.Search),
		Status:       strings.TrimSpace(arg.Status),
		PlanType:     strings.TrimSpace(arg.PlanType),
		UnsyncedOnly: sqliteBool(arg.UnsyncedOnly),
		PageOffset:   int64(arg.Offset),
		PageLimit:    int64(arg.Limit),
	})
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListGeminiCLIRow, len(rows))
	for i, row := range rows {
		resolved[i] = listGeminiCLIRowTo(
			row.ID,
			row.Status,
			row.AccessToken,
			row.RefreshToken,
			row.Expired,
			row.Email,
			row.ProjectID,
			row.PlanType,
			row.Reason,
			row.QuotaPro,
			row.QuotaFlash,
			row.QuotaFlashlite,
			row.ResetPro,
			row.ResetFlash,
			row.ResetFlashlite,
			row.ThrottledUntil,
			row.SyncedAt,
		)
	}
	return resolved, nil
}

func (s *Store) UpsertGeminiCLI(ctx context.Context, arg db.UpsertGeminiCLIParams) (db.GeminiCredential, error) {
	row, err := s.queries.UpsertGeminiCLI(ctx, sqlcsqlite.UpsertGeminiCLIParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      fmtTime(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		Reason:       arg.Reason,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	return geminiCredentialTo(row), nil
}

func (s *Store) DeleteGeminiCLI(ctx context.Context, id string) error {
	affected, err := s.queries.DeleteGeminiCLI(ctx, id)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateGeminiCLIStatus(ctx context.Context, id string, status string, reason string) (db.GeminiCredential, error) {
	row, err := s.queries.UpdateGeminiCLIStatus(ctx, sqlcsqlite.UpdateGeminiCLIStatusParams{
		Status: status,
		Reason: reason,
		ID:     id,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	return geminiCredentialTo(row), nil
}
