package postgres

import (
	"context"
	"time"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) UpsertGeminiQuota(ctx context.Context, arg db.UpsertGeminiQuotaParams) error {
	_, err := s.queries.UpsertGeminiQuota(ctx, sqlcpostgres.UpsertGeminiQuotaParams{
		CredentialID:   arg.CredentialID,
		QuotaPro:       arg.QuotaPro,
		ResetPro:       tsFrom(arg.ResetPro),
		QuotaFlash:     arg.QuotaFlash,
		ResetFlash:     tsFrom(arg.ResetFlash),
		QuotaFlashlite: arg.QuotaFlashlite,
		ResetFlashlite: tsFrom(arg.ResetFlashlite),
	})
	return err
}

func (s *Store) SetGeminiQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return s.queries.SetGeminiQuotaThrottled(ctx, sqlcpostgres.SetGeminiQuotaThrottledParams{
		CredentialID:   credentialID,
		ThrottledUntil: tsFrom(throttledUntil),
	})
}

func (s *Store) DeleteGeminiQuota(ctx context.Context, credentialID string) error {
	affected, err := s.queries.DeleteGeminiQuota(ctx, credentialID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) ListAvailableGeminiCLI(ctx context.Context) ([]db.ListAvailableGeminiCLIRow, error) {
	rows, err := s.queries.ListAvailableGeminiCLI(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListAvailableGeminiCLIRow, len(rows))
	for i, row := range rows {
		resolved[i] = db.ListAvailableGeminiCLIRow{
			ID:             row.ID,
			Email:          row.Email,
			ProjectID:      row.ProjectID,
			PlanType:       row.PlanType,
			QuotaPro:       row.QuotaPro,
			ResetPro:       tsTo(row.ResetPro),
			QuotaFlash:     row.QuotaFlash,
			ResetFlash:     tsTo(row.ResetFlash),
			QuotaFlashlite: row.QuotaFlashlite,
			ResetFlashlite: tsTo(row.ResetFlashlite),
			ThrottledUntil: tsTo(row.ThrottledUntil),
			SyncedAt:       tsTo(row.SyncedAt),
		}
	}
	return resolved, nil
}
