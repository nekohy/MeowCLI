package bridge

import (
	"time"

	"github.com/maypok86/otter/v2"
)

type fillFirstCache struct {
	*otter.Cache[string, string]
}

func newFillFirstCache() *fillFirstCache {
	return &fillFirstCache{otter.Must(&otter.Options[string, string]{
		ExpiryCalculator: otter.ExpiryWriting[string, string](time.Hour),
	})}
}

func (c *fillFirstCache) deleteIf(key string, credentialID string) {
	_, _ = c.ComputeIfPresent(key, func(current string) (string, otter.ComputeOp) {
		if current == credentialID {
			return "", otter.InvalidateOp
		}
		return current, otter.CancelOp
	})
}
