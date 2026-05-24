package scheduling

import (
	"context"
	"time"
)

const DefaultSchedulerWriteTimeout = 5 * time.Second

func WithDefaultWriteTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithWriteTimeout(ctx, DefaultSchedulerWriteTimeout)
}

func WithWriteTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
