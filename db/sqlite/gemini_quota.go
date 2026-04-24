package sqlite

import (
	"context"
	"time"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) UpsertGeminiQuota(ctx context.Context, arg db.UpsertGeminiQuotaParams) error {
	_, err := s.queries.UpsertGeminiQuota(ctx, sqlcsqlite.UpsertGeminiQuotaParams{
		CredentialID:   arg.CredentialID,
		QuotaPro:       arg.QuotaPro,
		ResetPro:       fmtTime(arg.ResetPro),
		QuotaFlash:     arg.QuotaFlash,
		ResetFlash:     fmtTime(arg.ResetFlash),
		QuotaFlashlite: arg.QuotaFlashlite,
		ResetFlashlite: fmtTime(arg.ResetFlashlite),
	})
	return err
}

func (s *Store) SetGeminiQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return s.queries.SetGeminiQuotaThrottled(ctx, sqlcsqlite.SetGeminiQuotaThrottledParams{
		CredentialID:   credentialID,
		ThrottledUntil: fmtTime(throttledUntil),
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
			ResetPro:       parseTime(row.ResetPro),
			QuotaFlash:     row.QuotaFlash,
			ResetFlash:     parseTime(row.ResetFlash),
			QuotaFlashlite: row.QuotaFlashlite,
			ResetFlashlite: parseTime(row.ResetFlashlite),
			ThrottledUntil: parseTime(row.ThrottledUntil),
			SyncedAt:       parseTime(row.SyncedAt),
		}
	}
	return resolved, nil
}
