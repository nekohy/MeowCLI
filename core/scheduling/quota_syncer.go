package scheduling

import (
	"context"
	"sync"
	"time"
)

// QuotaSyncer 对具有独立 synced_at 时间戳的行执行配额同步，负责定时器调度和到期行过滤；调用方在 Sync 中维护处理器特定的令牌、配额获取和持久化逻辑
type QuotaSyncer[T any] struct {
	SyncInterval func() time.Duration
	List         func(context.Context) ([]T, error)
	CacheRows    func(context.Context, []T)
	Sync         func(context.Context, T)
	RowID        func(T) string
	SyncedAt     func(T) time.Time
	ResetAt      func(T) time.Time
	WithSyncedAt func(T, time.Time) T
	ReportError  func(error, string)

	WarmErrorMessage    string
	ListErrorMessage    string
	RefreshErrorMessage string
}

const QuotaResetRefreshGrace = 5 * time.Second

func (q QuotaSyncer[T]) Start(ctx context.Context) {
	go func() {
		rows := q.loadAndCacheRows(ctx, q.WarmErrorMessage)

		for {
			timer := time.NewTimer(q.nextDelay(rows, time.Now()))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				rows = q.syncDueRows(ctx)
			}
		}
	}()
}

func (q QuotaSyncer[T]) nextDelay(rows []T, now time.Time) time.Duration {
	interval := q.syncInterval()
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
		resetAt := q.resetAt(row)
		if resetAt.IsZero() {
			continue
		}
		delay = resetAt.Add(QuotaResetRefreshGrace).Sub(now)
		if delay <= 0 {
			return 0
		}
		if delay < next {
			next = delay
		}
	}
	return next
}

func (q QuotaSyncer[T]) syncDueRows(ctx context.Context) []T {
	rows, err := q.List(ctx)
	if err != nil {
		q.reportError(err, q.ListErrorMessage)
		return nil
	}

	now := time.Now()
	interval := q.syncInterval()
	attempted := make(map[string]time.Time)
	sem := make(chan struct{}, DefaultQuotaSyncConcurrency)
	var wg sync.WaitGroup
syncLoop:
	for _, row := range q.prioritizedDueRows(rows, now, interval) {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break syncLoop
		}
		attempted[q.RowID(row)] = now
		wg.Add(1)
		go func(row T) {
			defer wg.Done()
			defer func() { <-sem }()
			q.Sync(ctx, row)
		}(row)
	}
	wg.Wait()

	refreshed := q.loadAndCacheRows(ctx, q.RefreshErrorMessage)
	if refreshed == nil {
		refreshed = rows
	}
	return q.withAttemptedSyncTimes(refreshed, attempted)
}

func (q QuotaSyncer[T]) prioritizedDueRows(rows []T, now time.Time, interval time.Duration) []T {
	if len(rows) == 0 {
		return nil
	}
	resetDue := make([]T, 0, len(rows))
	intervalDue := make([]T, 0, len(rows))
	for _, row := range rows {
		if quotaResetRefreshDue(q.resetAt(row), now) {
			resetDue = append(resetDue, row)
			continue
		}
		if quotaSyncDue(q.SyncedAt(row), now, interval) {
			intervalDue = append(intervalDue, row)
		}
	}
	return append(resetDue, intervalDue...)
}

func (q QuotaSyncer[T]) withAttemptedSyncTimes(rows []T, attempted map[string]time.Time) []T {
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

func quotaSyncDue(syncedAt time.Time, now time.Time, interval time.Duration) bool {
	return syncedAt.IsZero() || !syncedAt.Add(interval).After(now)
}

func quotaResetRefreshDue(resetAt time.Time, now time.Time) bool {
	return !resetAt.IsZero() && !resetAt.Add(QuotaResetRefreshGrace).After(now)
}

func EarliestTime(times ...time.Time) time.Time {
	var earliest time.Time
	for _, t := range times {
		if t.IsZero() {
			continue
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

func (q QuotaSyncer[T]) loadAndCacheRows(ctx context.Context, failureMsg string) []T {
	rows, err := q.List(ctx)
	if err != nil {
		q.reportError(err, failureMsg)
		return nil
	}
	q.CacheRows(ctx, rows)
	return rows
}

func (q QuotaSyncer[T]) syncInterval() time.Duration {
	if q.SyncInterval == nil {
		return 0
	}
	return q.SyncInterval()
}

func (q QuotaSyncer[T]) resetAt(row T) time.Time {
	if q.ResetAt == nil {
		return time.Time{}
	}
	return q.ResetAt(row)
}

func (q QuotaSyncer[T]) reportError(err error, message string) {
	if q.ReportError != nil {
		q.ReportError(err, message)
	}
}
