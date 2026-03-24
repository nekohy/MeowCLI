package codex

import (
	"context"
	"time"
)

const (
	defaultDialTimeout           = 10 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 45 * time.Second
	defaultExpectContinueTimeout = 1 * time.Second
	defaultIdleConnTimeout       = 90 * time.Second
	defaultQuotaRequestTimeout   = 15 * time.Second
	defaultRefreshTokenTimeout   = 20 * time.Second
)

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
