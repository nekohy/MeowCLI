package sqlite

import (
	"context"
	"time"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) UpsertQuota(ctx context.Context, arg db.UpsertQuotaParams) error {
	_, err := s.queries.UpsertQuota(ctx, sqlcsqlite.UpsertQuotaParams{
		CredentialID: arg.CredentialID,
		Quota5h:      arg.Quota5h,
		Quota7d:      arg.Quota7d,
		QuotaSpark5h: arg.QuotaSpark5h,
		QuotaSpark7d: arg.QuotaSpark7d,
		Reset5h:      fmtTime(arg.Reset5h),
		Reset7d:      fmtTime(arg.Reset7d),
		ResetSpark5h: fmtTime(arg.ResetSpark5h),
		ResetSpark7d: fmtTime(arg.ResetSpark7d),
	})
	return err
}

func (s *Store) SetQuotaThrottled(ctx context.Context, credentialID string, modelTier string, throttledUntil time.Time) error {
	switch modelTier {
	case "spark":
		return s.queries.SetQuotaThrottledSpark(ctx, sqlcsqlite.SetQuotaThrottledSparkParams{
			CredentialID:        credentialID,
			ThrottledUntilSpark: fmtTime(throttledUntil),
		})
	case "default":
		return s.queries.SetQuotaThrottled(ctx, sqlcsqlite.SetQuotaThrottledParams{
			CredentialID:   credentialID,
			ThrottledUntil: fmtTime(throttledUntil),
		})
	default:
		value := fmtTime(throttledUntil)
		return s.queries.SetQuotaThrottledAll(ctx, sqlcsqlite.SetQuotaThrottledAllParams{
			CredentialID:        credentialID,
			ThrottledUntil:      value,
			ThrottledUntilSpark: value,
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
		resolved[i] = listAvailableCodexRowTo(row.ID, row.PlanType, row.Quota5h, row.Quota7d, row.QuotaSpark5h, row.QuotaSpark7d, row.Reset5h, row.Reset7d, row.ResetSpark5h, row.ResetSpark7d, row.ThrottledUntil, row.ThrottledUntilSpark, row.SyncedAt)
	}

	return resolved, nil
}
