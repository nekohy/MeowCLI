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
		Reset5h:      tsFrom(arg.Reset5h),
		Reset7d:      tsFrom(arg.Reset7d),
	})
	return err
}

func (s *Store) SetQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return s.queries.SetQuotaThrottled(ctx, sqlcpostgres.SetQuotaThrottledParams{
		CredentialID:   credentialID,
		ThrottledUntil: tsFrom(throttledUntil),
	})
}

func (s *Store) ListAvailableCodex(ctx context.Context) ([]db.ListAvailableCodexRow, error) {
	rows, err := s.queries.ListAvailableCodex(ctx)
	if err != nil {
		return nil, err
	}

	resolved := make([]db.ListAvailableCodexRow, len(rows))
	for i, row := range rows {
		resolved[i] = listAvailableCodexRowTo(row.ID, row.PlanType, row.Quota5h, row.Quota7d, tsTo(row.Reset5h), tsTo(row.Reset7d), tsTo(row.ThrottledUntil), tsTo(row.SyncedAt))
	}

	return resolved, nil
}
