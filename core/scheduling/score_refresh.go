package scheduling

import (
	"context"
	"time"
)

type ScoreRefreshLoop struct {
	Interval        func() time.Duration
	DefaultInterval time.Duration
	Refresh         func()
}

func (l ScoreRefreshLoop) Start(ctx context.Context) {
	if l.Refresh == nil {
		return
	}
	go func() {
		for {
			interval := l.interval()
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				l.Refresh()
			}
		}
	}()
}

func (l ScoreRefreshLoop) interval() time.Duration {
	if l.Interval != nil {
		if interval := l.Interval(); interval > 0 {
			return interval
		}
	}
	if l.DefaultInterval > 0 {
		return l.DefaultInterval
	}
	return time.Minute
}

func RefreshRows[T any, S any](
	load func() *S,
	compareAndSwap func(*S, *S) bool,
	cloneRows func(*S) []T,
	buildSnapshot func([]T) *S,
	refreshRow func(*T),
) {
	if load == nil || compareAndSwap == nil || cloneRows == nil || buildSnapshot == nil || refreshRow == nil {
		return
	}

	for {
		snap := load()
		if snap == nil {
			return
		}

		rows := cloneRows(snap)
		for i := range rows {
			refreshRow(&rows[i])
		}

		if compareAndSwap(snap, buildSnapshot(rows)) {
			return
		}
	}
}
