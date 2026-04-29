package postgres

import (
	"context"
	"strings"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledGeminiCLI(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledGeminiCLI(ctx)
}

func (s *Store) CountGeminiCLI(ctx context.Context) (int64, error) {
	return s.queries.CountGeminiCLI(ctx)
}

func (s *Store) CountGeminiCLIFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountGeminiCLIFiltered(ctx, sqlcpostgres.CountGeminiCLIFilteredParams{
		Search:       postgresCodexSearchPattern(filter.Search),
		Status:       strings.TrimSpace(filter.Status),
		PlanType:     strings.TrimSpace(filter.PlanType),
		UnsyncedOnly: filter.UnsyncedOnly,
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpdateGeminiTokens(ctx, sqlcpostgres.UpdateGeminiTokensParams{
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      tsFrom(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		ID:           arg.ID,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	if shouldClearCredentialThrottle(arg.Status) {
		if err := queries.ClearGeminiQuotaThrottle(ctx, arg.ID); err != nil {
			return db.GeminiCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GeminiCredential{}, err
	}
	committed = true
	return geminiCredentialTo(row), nil
}

func (s *Store) UpdateGeminiPlanType(ctx context.Context, id string, planType string) (db.GeminiCredential, error) {
	row, err := s.queries.UpdateGeminiPlanType(ctx, sqlcpostgres.UpdateGeminiPlanTypeParams{
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
			tsTo(row.Expired),
			row.Email,
			row.ProjectID,
			row.PlanType,
			row.Reason,
			row.QuotaPro,
			row.QuotaFlash,
			row.QuotaFlashlite,
			tsTo(row.ResetPro),
			tsTo(row.ResetFlash),
			tsTo(row.ResetFlashlite),
			tsTo(row.ThrottledUntil),
			tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func (s *Store) ListGeminiCLIPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListGeminiCLIRow, error) {
	rows, err := s.queries.ListGeminiCLIPaged(ctx, sqlcpostgres.ListGeminiCLIPagedParams{
		Search:       postgresCodexSearchPattern(arg.Search),
		Status:       strings.TrimSpace(arg.Status),
		PlanType:     strings.TrimSpace(arg.PlanType),
		UnsyncedOnly: arg.UnsyncedOnly,
		PageOffset:   arg.Offset,
		PageLimit:    arg.Limit,
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
			tsTo(row.Expired),
			row.Email,
			row.ProjectID,
			row.PlanType,
			row.Reason,
			row.QuotaPro,
			row.QuotaFlash,
			row.QuotaFlashlite,
			tsTo(row.ResetPro),
			tsTo(row.ResetFlash),
			tsTo(row.ResetFlashlite),
			tsTo(row.ThrottledUntil),
			tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func (s *Store) UpsertGeminiCLI(ctx context.Context, arg db.UpsertGeminiCLIParams) (db.GeminiCredential, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpsertGeminiCLI(ctx, sqlcpostgres.UpsertGeminiCLIParams{
		ID:           arg.ID,
		Status:       arg.Status,
		AccessToken:  arg.AccessToken,
		RefreshToken: arg.RefreshToken,
		Expired:      tsFrom(arg.Expired),
		Email:        arg.Email,
		ProjectID:    arg.ProjectID,
		PlanType:     arg.PlanType,
		Reason:       arg.Reason,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	if shouldClearCredentialThrottle(arg.Status) {
		if err := queries.ClearGeminiQuotaThrottle(ctx, arg.ID); err != nil {
			return db.GeminiCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GeminiCredential{}, err
	}
	committed = true
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GeminiCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpdateGeminiCLIStatus(ctx, sqlcpostgres.UpdateGeminiCLIStatusParams{
		Status: status,
		Reason: reason,
		ID:     id,
	})
	if err != nil {
		return db.GeminiCredential{}, wrapError(err)
	}
	if shouldClearCredentialThrottle(status) {
		if err := queries.ClearGeminiQuotaThrottle(ctx, id); err != nil {
			return db.GeminiCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GeminiCredential{}, err
	}
	committed = true
	return geminiCredentialTo(row), nil
}

func (s *Store) RestoreExpiredThrottledGeminiCLI(ctx context.Context) error {
	return wrapError(s.queries.RestoreExpiredThrottledGeminiCLI(ctx))
}
