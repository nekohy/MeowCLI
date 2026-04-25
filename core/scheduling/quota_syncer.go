package scheduling

import (
	"context"
	"time"
)

// QuotaSyncer 对具有独立 synced_at 时间戳的行执行配额同步，负责定时器调度和到期行过滤；调用方在 Sync 中维护处理器特定的令牌、配额获取和持久化逻辑
type QuotaSyncer[T any] struct {
	Interval     func() time.Duration
	List         func(context.Context) ([]T, error)
	Refresh      func(context.Context, []T)
	Sync         func(context.Context, T)
	RowID        func(T) string
	SyncedAt     func(T) time.Time
	WithSyncedAt func(T, time.Time) T
	LogError     func(error, string)

	WarmErrorMessage    string
	ListErrorMessage    string
	RefreshErrorMessage string
}

func (q QuotaSyncer[T]) Start(ctx context.Context) {
	go func() {
		rows := q.refresh(ctx, q.WarmErrorMessage)

		for {
			timer := time.NewTimer(q.NextDelay(rows, time.Now()))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				rows = q.SyncDue(ctx)
			}
		}
	}()
}

func (q QuotaSyncer[T]) NextDelay(rows []T, now time.Time) time.Duration {
	interval := q.interval()
	if len(rows) == 0 {
		return interval
	}

	next := interval
	for _, row := range rows {
		syncedAt := q.SyncedAt(row)
		if syncedAt.IsZero() {
			return 0
		}
		delay := syncedAt.Add(interval).Sub(now)
		if delay <= 0 {
			return 0
		}
		if delay < next {
			next = delay
		}
	}
	return next
}

func (q QuotaSyncer[T]) SyncDue(ctx context.Context) []T {
	rows, err := q.List(ctx)
	if err != nil {
		q.logError(err, q.ListErrorMessage)
		return nil
	}

	now := time.Now()
	interval := q.interval()
	attempted := make(map[string]time.Time)
	for _, row := range rows {
		if !QuotaSyncDue(q.SyncedAt(row), now, interval) {
			continue
		}
		attempted[q.RowID(row)] = now
		q.Sync(ctx, row)
	}

	refreshed := q.refresh(ctx, q.RefreshErrorMessage)
	if refreshed == nil {
		refreshed = rows
	}
	return q.ApplyAttemptTimes(refreshed, attempted)
}

func (q QuotaSyncer[T]) ApplyAttemptTimes(rows []T, attempted map[string]time.Time) []T {
	if len(rows) == 0 || len(attempted) == 0 {
		return rows
	}
	next := append([]T(nil), rows...)
	for i := range next {
		attemptedAt, ok := attempted[q.RowID(next[i])]
		if ok && q.SyncedAt(next[i]).Before(attemptedAt) {
			next[i] = q.WithSyncedAt(next[i], attemptedAt)
		}
	}
	return next
}

func QuotaSyncDue(syncedAt time.Time, now time.Time, interval time.Duration) bool {
	return syncedAt.IsZero() || !syncedAt.Add(interval).After(now)
}

func (q QuotaSyncer[T]) refresh(ctx context.Context, failureMsg string) []T {
	rows, err := q.List(ctx)
	if err != nil {
		q.logError(err, failureMsg)
		return nil
	}
	q.Refresh(ctx, rows)
	return rows
}

func (q QuotaSyncer[T]) interval() time.Duration {
	if q.Interval == nil {
		return 0
	}
	return q.Interval()
}

func (q QuotaSyncer[T]) logError(err error, message string) {
	if q.LogError != nil {
		q.LogError(err, message)
	}
}
