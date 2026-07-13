package sqlite

import (
	"context"
	"time"

	sqlcsqlite "github.com/nekohy/MeowCLI/internal/db/sqlite"
	db "github.com/nekohy/MeowCLI/internal/store"
)

func (s *Store) CountEnabledOpenCodeGo(ctx context.Context) (int64, error) {
	return s.queries.CountEnabledOpenCodeGo(ctx)
}

func (s *Store) CountOpenCodeGo(ctx context.Context) (int64, error) {
	return s.queries.CountOpenCodeGo(ctx)
}

func (s *Store) CountOpenCodeGoFiltered(ctx context.Context, filter db.CredentialFilterParams) (int64, error) {
	return s.queries.CountOpenCodeGoFiltered(ctx, sqlcsqlite.CountOpenCodeGoFilteredParams{
		Search:       sqliteCodexSearchPattern(filter.Search),
		Statuses:     db.CredentialStatusFilterValue(filter.Statuses),
		UnsyncedOnly: sqliteBool(filter.UnsyncedOnly),
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
			row.Reason, row.CreatedAt, row.UpdatedAt,
			row.Quota5h, row.Quota7d, row.Quota1mo, row.Reset5h, row.Reset7d, row.Reset1mo,
			row.RewardsCount,
			row.ThrottledUntil, row.SyncedAt,
		)
	}
	return resolved, nil
}

func (s *Store) ListOpenCodeGoPaged(ctx context.Context, arg db.ListCredentialPagedParams) ([]db.ListOpenCodeGoRow, error) {
	rows, err := s.queries.ListOpenCodeGoPaged(ctx, sqlcsqlite.ListOpenCodeGoPagedParams{
		Search:       sqliteCodexSearchPattern(arg.Search),
		Statuses:     db.CredentialStatusFilterValue(arg.Statuses),
		UnsyncedOnly: sqliteBool(arg.UnsyncedOnly),
		PageOffset:   int64(arg.Offset),
		PageLimit:    int64(arg.Limit),
	})
	if err != nil {
		return nil, err
	}
	resolved := make([]db.ListOpenCodeGoRow, len(rows))
	for i, row := range rows {
		resolved[i] = listOpenCodeGoRowTo(
			row.ID, row.Status, row.ApiKey, row.AuthCookie,
			row.Reason, row.CreatedAt, row.UpdatedAt,
			row.Quota5h, row.Quota7d, row.Quota1mo, row.Reset5h, row.Reset7d, row.Reset1mo,
			row.RewardsCount,
			row.ThrottledUntil, row.SyncedAt,
		)
	}
	return resolved, nil
}

func (s *Store) UpsertOpenCodeGo(ctx context.Context, arg db.UpsertOpenCodeGoParams) (db.OpenCodeGoCredential, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlcsqlite.New(tx)
	row, err := queries.UpsertOpenCodeGo(ctx, sqlcsqlite.UpsertOpenCodeGoParams{
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
	if err := tx.Commit(); err != nil {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.OpenCodeGoCredential{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlcsqlite.New(tx)
	row, err := queries.UpdateOpenCodeGoStatus(ctx, sqlcsqlite.UpdateOpenCodeGoStatusParams{
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
	if err := tx.Commit(); err != nil {
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
	return parseTime(value), nil
}

func (s *Store) UpsertOpenCodeGoQuota(ctx context.Context, arg db.UpsertOpenCodeGoQuotaParams) error {
	_, err := s.queries.UpsertOpenCodeGoQuota(ctx, sqlcsqlite.UpsertOpenCodeGoQuotaParams{
		CredentialID: arg.CredentialID,
		Quota5h:      arg.Quota5h,
		Quota7d:      arg.Quota7d,
		Quota1mo:     arg.Quota1mo,
		Reset5h:      sqliteTimeString(arg.Reset5h),
		Reset7d:      sqliteTimeString(arg.Reset7d),
		Reset1mo:     sqliteTimeString(arg.Reset1mo),
		RewardsCount: int64(arg.RewardsCount),
	})
	return wrapError(err)
}

func (s *Store) SetOpenCodeGoQuotaThrottled(ctx context.Context, credentialID string, throttledUntil time.Time) error {
	return wrapError(s.queries.SetOpenCodeGoQuotaThrottled(ctx, sqlcsqlite.SetOpenCodeGoQuotaThrottledParams{
		CredentialID: credentialID, ThrottledUntil: fmtTime(throttledUntil),
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
			Reset5h: parseTime(row.Reset5h), Reset7d: parseTime(row.Reset7d), Reset1mo: parseTime(row.Reset1mo),
			RewardsCount:   int(row.RewardsCount),
			ThrottledUntil: parseTime(row.ThrottledUntil), SyncedAt: parseTime(row.SyncedAt),
		}
	}
	return resolved, nil
}

func openCodeGoCredentialTo(value sqlcsqlite.OpencodeGo) db.OpenCodeGoCredential {
	return db.OpenCodeGoCredential{
		ID: value.ID, Status: value.Status, APIKey: value.ApiKey, AuthCookie: value.AuthCookie,
		Reason:    value.Reason,
		CreatedAt: parseTime(value.CreatedAt), UpdatedAt: parseTime(value.UpdatedAt),
	}
}

func listOpenCodeGoRowTo(
	id, status, apiKey, authCookie, reason, createdAt, updatedAt string,
	quota5h, quota7d, quota1mo float64,
	reset5h, reset7d, reset1mo string,
	rewardsCount int64,
	throttledUntil, syncedAt string,
) db.ListOpenCodeGoRow {
	return db.ListOpenCodeGoRow{
		ID: id, Status: status, APIKey: apiKey, AuthCookie: authCookie,
		Reason:    reason,
		CreatedAt: parseTime(createdAt), UpdatedAt: parseTime(updatedAt),
		Quota5h: quota5h, Quota7d: quota7d, Quota1mo: quota1mo,
		Reset5h: parseTime(reset5h), Reset7d: parseTime(reset7d), Reset1mo: parseTime(reset1mo),
		RewardsCount:   int(rewardsCount),
		ThrottledUntil: parseTime(throttledUntil), SyncedAt: parseTime(syncedAt),
	}
}
