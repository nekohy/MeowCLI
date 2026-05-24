package scheduling

import (
	"context"
	"time"
)

const throttleDeadlineRetryInterval = time.Minute

type ThrottleDeadlineRefreshConfig struct {
	Component    string
	Refresh      func(context.Context) error
	NextDeadline func(context.Context) (time.Time, error)
	ReportError  func(error, string)
}

type RefreshAfterDeadlineConfig struct {
	Deadline    time.Time
	Superseded  func() bool
	Refresh     func(context.Context) error
	ReportError func(error)
}

type throttleDeadlineRefresher struct {
	refresh              func(context.Context) error
	nextDeadline         func(context.Context) (time.Time, error)
	logError             func(error, string)
	refreshErrorMessage  string
	deadlineErrorMessage string
}

func StartThrottleDeadlineRefresh(ctx context.Context, config ThrottleDeadlineRefreshConfig) {
	refresher := throttleDeadlineRefresher{
		refresh:              config.Refresh,
		nextDeadline:         config.NextDeadline,
		logError:             config.ReportError,
		refreshErrorMessage:  config.Component + ": refresh available after throttle deadline",
		deadlineErrorMessage: config.Component + ": next throttle deadline",
	}
	go refresher.run(ctx)
}

func (r throttleDeadlineRefresher) run(ctx context.Context) {
	r.doRefresh(ctx)
	for {
		deadline, err := r.nextDeadline(ctx)
		if err != nil {
			r.reportError(err, r.deadlineErrorMessage)
			if !sleepContext(ctx, throttleDeadlineRetryInterval) {
				return
			}
			continue
		}

		delay := throttleDeadlineRetryInterval
		if !deadline.IsZero() {
			delay = time.Until(deadline)
			if delay < 0 {
				delay = 0
			}
		}
		if !sleepContext(ctx, delay) {
			return
		}
		r.doRefresh(ctx)
	}
}

func (r throttleDeadlineRefresher) doRefresh(ctx context.Context) {
	if err := r.refresh(ctx); err != nil {
		r.reportError(err, r.refreshErrorMessage)
	}
}

func (r throttleDeadlineRefresher) reportError(err error, message string) {
	r.logError(err, message)
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func RefreshAfterDeadline(config RefreshAfterDeadlineConfig) {
	ctx := context.Background()
	if !sleepContext(ctx, time.Until(config.Deadline)) {
		return
	}
	if config.Superseded() {
		return
	}
	if err := config.Refresh(ctx); err != nil {
		config.ReportError(err)
	}
}
