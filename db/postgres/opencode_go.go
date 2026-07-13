package postgres

import (
	"context"
	"time"

	sqlcpostgres "github.com/nekohy/MeowCLI/internal/db/postgres"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledOpenCodeGo(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledOpenCodeGo(ctx)
}

func (s *Store) CountOpenCodeGo(ctx context.Context) (int64, error) {
	return s.queries.CountOpenCodeGo(ctx)
}

func (s *Store) CountOpenCodeGoFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountOpenCodeGoFiltered(ctx, sqlcpostgres.CountOpenCodeGoFilteredParams{
		Search:       postgresCodexSearchPattern(filter.Search),
		Statuses:     db.CredentialStatusFilterValue(filter.Statuses),
		UnsyncedOnly: filter.UnsyncedOnly,
	})
}

func (s *Store) GetOpenCodeGo(ctx context.Context, id string) (db.OpenCodeGoCredential, error) {
	row, err := s.queries.GetOpenCodeGo(ctx, id)
	if err != nil {
		return db.OpenCodeGoCredential{}, wrapError(err)
	}
	return openCodeGoCredentialTo(row), nil
}

func (s *Store) ListOpenCodeGo(ctx context.Context) ([]db.ListOpenCodeGoRow, error) {
	rows, err := s.queries.ListOpenCodeGo(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListOpenCodeGoRow, len(rows))
	for i, row := range rows {
		resolved[i] = listOpenCodeGoRowTo(
			row.ID, row.Status, row.ApiKey, row.AuthCookie,
			row.Reason, tsTo(row.CreatedAt), tsTo(row.UpdatedAt),
			row.Quota5h, row.Quota7d, row.Quota1mo, tsTo(row.Reset5h), tsTo(row.Reset7d), tsTo(row.Reset1mo),
			int(row.RewardsCount),
			tsTo(row.ThrottledUntil), tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func (s *Store) ListOpenCodeGoPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListOpenCodeGoRow, error) {
	rows, err := s.queries.ListOpenCodeGoPaged(ctx, sqlcpostgres.ListOpenCodeGoPagedParams{
		Search:       postgresCodexSearchPattern(arg.Search),
		Statuses:     db.CredentialStatusFilterValue(arg.Statuses),
		UnsyncedOnly: arg.UnsyncedOnly,
		PageOffset:   arg.Offset,
		PageLimit:    arg.Limit,
	})
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListOpenCodeGoRow, len(rows))
	for i, row := range rows {
		resolved[i] = listOpenCodeGoRowTo(
			row.ID, row.Status, row.ApiKey, row.AuthCookie,
			row.Reason, tsTo(row.CreatedAt), tsTo(row.UpdatedAt),
			row.Quota5h, row.Quota7d, row.Quota1mo, tsTo(row.Reset5h), tsTo(row.Reset7d), tsTo(row.Reset1mo),
			int(row.RewardsCount),
			tsTo(row.ThrottledUntil), tsTo(row.SyncedAt),
		)
	}
	return resolved, nil
}

func (s *Store) UpsertOpenCodeGo(ctx context.Context, arg db.UpsertOpenCodeGoParams) (db.OpenCodeGoCredential, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpsertOpenCodeGo(ctx, sqlcpostgres.UpsertOpenCodeGoParams{
		ID:         arg.ID,
		Status:     arg.Status,
		ApiKey:     arg.APIKey,
		AuthCookie: arg.AuthCookie,
		Reason:     arg.Reason,
	})
	if err != nil {
		return db.OpenCodeGoCredential{}, wrapError(err)
	}
	if db.ShouldClearCredentialThrottle(arg.Status) {
		if err := queries.ClearOpenCodeGoQuotaThrottle(ctx, arg.ID); err != nil {
			return db.OpenCodeGoCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed = true
	return openCodeGoCredentialTo(row), nil
}

func (s *Store) DeleteOpenCodeGo(ctx context.Context, id string) error {
	affected, err := s.queries.DeleteOpenCodeGo(ctx, id)
	if err != nil {
		return wrapError(err)
	}
	if affected == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateOpenCodeGoStatus(ctx context.Context, id string, status string, reason string) (db.OpenCodeGoCredential, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	queries := s.queries.WithTx(tx)
	row, err := queries.UpdateOpenCodeGoStatus(ctx, sqlcpostgres.UpdateOpenCodeGoStatusParams{
		ID: id, Status: status, Reason: reason,
	})
	if err != nil {
		return db.OpenCodeGoCredential{}, wrapError(err)
	}
	if db.ShouldClearCredentialThrottle(status) {
		if err := queries.ClearOpenCodeGoQuotaThrottle(ctx, id); err != nil {
			return db.OpenCodeGoCredential{}, wrapError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed = true
	return openCodeGoCredentialTo(row), nil
}

func (s *Store) RestoreExpiredThrottledOpenCodeGo(ctx context.Context) error {
	return wrapError(s.queries.RestoreExpiredThrottledOpenCodeGo(ctx))
}

func (s *Store) NextOpenCodeGoThrottleDeadline(ctx context.Context) (time.Time, error) {
	value, err := s.queries.NextOpenCodeGoThrottleDeadline(ctx)
	if err != nil {
		return time.Time{}, wrapError(err)
	}
	return tsTo(value), nil
}

func (s *Store) UpsertOpenCodeGoQuota(ctx context.Context, arg db.UpsertOpenCodeGoQuotaParams) error {
	_, err := s.queries.UpsertOpenCodeGoQuota(ctx, sqlcpostgres.UpsertOpenCodeGoQuotaParams{
		CredentialID: arg.CredentialID,
		Quota5h:      arg.Quota5h,
		Quota7d:      arg.Quota7d,
		Quota1mo:     arg.Quota1mo,
		Reset5h:      tsFrom(arg.Reset5h),
		Reset7d:      tsFrom(arg.Reset7d),
		Reset1mo:     tsFrom(arg.Reset1mo),
		RewardsCount: int32(arg.RewardsCount),
	})
	return wrapError(err)
}

func (s *Store) SetOpenCodeGoQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return wrapError(s.queries.SetOpenCodeGoQuotaThrottled(ctx, sqlcpostgres.SetOpenCodeGoQuotaThrottledParams{
		CredentialID: credentialID, ThrottledUntil: tsFrom(throttledUntil),
	}))
}

func (s *Store) DeleteOpenCodeGoQuota(ctx context.Context, credentialID string) error {
	_, err := s.queries.DeleteOpenCodeGoQuota(ctx, credentialID)
	return wrapError(err)
}

func (s *Store) ListAvailableOpenCodeGo(ctx context.Context) ([]db.ListAvailableOpenCodeGoRow, error) {
	rows, err := s.queries.ListAvailableOpenCodeGo(ctx)
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListAvailableOpenCodeGoRow, len(rows))
	for i, row := range rows {
		resolved[i] = db.ListAvailableOpenCodeGoRow{
			ID: row.ID, AuthCookie: row.AuthCookie,
			Quota5h: row.Quota5h, Quota7d: row.Quota7d, Quota1mo: row.Quota1mo,
			Reset5h: tsTo(row.Reset5h), Reset7d: tsTo(row.Reset7d), Reset1mo: tsTo(row.Reset1mo),
			RewardsCount:   int(row.RewardsCount),
			ThrottledUntil: tsTo(row.ThrottledUntil), SyncedAt: tsTo(row.SyncedAt),
		}
	}
	return resolved, nil
}

func openCodeGoCredentialTo(value sqlcpostgres.OpencodeGo) db.OpenCodeGoCredential {
	return db.OpenCodeGoCredential{
		ID: value.ID, Status: value.Status, APIKey: value.ApiKey, AuthCookie: value.AuthCookie,
		Reason:    value.Reason,
		CreatedAt: tsTo(value.CreatedAt), UpdatedAt: tsTo(value.UpdatedAt),
	}
}

func listOpenCodeGoRowTo(
	id, status, apiKey, authCookie, reason string,
	createdAt, updatedAt time.Time,
	quota5h, quota7d, quota1mo float64,
	reset5h, reset7d, reset1mo time.Time,
	rewardsCount int,
	throttledUntil, syncedAt time.Time,
) db.ListOpenCodeGoRow {
	return db.ListOpenCodeGoRow{
		ID: id, Status: status, APIKey: apiKey, AuthCookie: authCookie,
		Reason:    reason,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
		Quota5h: quota5h, Quota7d: quota7d, Quota1mo: quota1mo,
		Reset5h: reset5h, Reset7d: reset7d, Reset1mo: reset1mo,
		RewardsCount:   rewardsCount,
		ThrottledUntil: throttledUntil, SyncedAt: syncedAt,
	}
}
