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
		Reset5h:      fmtTime(arg.Reset5h),
		Reset7d:      fmtTime(arg.Reset7d),
	})
	return err
}

func (s *Store) SetQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return s.queries.SetQuotaThrottled(ctx, sqlcsqlite.SetQuotaThrottledParams{
		CredentialID:   credentialID,
		ThrottledUntil: fmtTime(throttledUntil),
	})
}

func (s *Store) ListAvailableCodex(ctx context.Context) ([]db.ListAvailableCodexRow, error) {
	rows, err := s.queries.ListAvailableCodex(ctx)
	if err != nil {
		return nil, err
	}

	resolved := make([]db.ListAvailableCodexRow, len(rows))
	for i, row := range rows {
		resolved[i] = listAvailableCodexRowTo(row.ID, row.PlanType, row.Quota5h, row.Quota7d, row.Reset5h, row.Reset7d, row.ThrottledUntil, row.SyncedAt)
	}

	return resolved, nil
}
