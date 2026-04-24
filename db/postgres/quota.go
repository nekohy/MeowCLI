package postgres

import (
	"context"
	"time"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) UpsertQuota(ctx context.Context, arg db.UpsertQuotaParams) error {
	_, err := s.queries.UpsertQuota(ctx, sqlcpostgres.UpsertQuotaParams{
		CredentialID: arg.CredentialID,
		Quota5h:      arg.Quota5h,
		Quota7d:      arg.Quota7d,
		QuotaSpark5h: arg.QuotaSpark5h,
		QuotaSpark7d: arg.QuotaSpark7d,
		Reset5h:      tsFrom(arg.Reset5h),
		Reset7d:      tsFrom(arg.Reset7d),
		ResetSpark5h: tsFrom(arg.ResetSpark5h),
		ResetSpark7d: tsFrom(arg.ResetSpark7d),
	})
	return err
}

func (s *Store) SetQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error {
	switch modelTier {
	case "spark":
		return s.queries.SetQuotaThrottledSpark(ctx, sqlcpostgres.SetQuotaThrottledSparkParams{
			CredentialID:        credentialID,
			ThrottledUntilSpark: tsFrom(throttledUntil),
		})
	case "default":
		return s.queries.SetQuotaThrottled(ctx, sqlcpostgres.SetQuotaThrottledParams{
			CredentialID:   credentialID,
			ThrottledUntil: tsFrom(throttledUntil),
		})
	default:
		return s.queries.SetQuotaThrottledAll(ctx, sqlcpostgres.SetQuotaThrottledAllParams{
			CredentialID:   credentialID,
			ThrottledUntil: tsFrom(throttledUntil),
		})
	}
}

func (s *Store) DeleteQuota(ctx context.Context, credentialID string) error {
	affected, err := s.queries.DeleteQuota(ctx, credentialID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) ListAvailableCodex(ctx context.Context) ([]db.ListAvailableCodexRow, error) {
	rows, err := s.queries.ListAvailableCodex(ctx)
	if err != nil {
		return nil, err
	}

	resolved := make([]db.ListAvailableCodexRow, len(rows))
	for i, row := range rows {
		resolved[i] = listAvailableCodexRowTo(row.ID, row.PlanType, row.Quota5h, row.Quota7d, row.QuotaSpark5h, row.QuotaSpark7d, tsTo(row.Reset5h), tsTo(row.Reset7d), tsTo(row.ResetSpark5h), tsTo(row.ResetSpark7d), tsTo(row.ThrottledUntil), tsTo(row.ThrottledUntilSpark), tsTo(row.SyncedAt))
	}

	return resolved, nil
}
