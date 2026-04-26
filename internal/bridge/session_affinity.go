package bridge

import (
	"time"

	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog/log"
)

const defaultSessionAffinityTTL = 24 * time.Hour

func newSessionAffinityCache() *otter.Cache[string, string] {
	cache, err := otter.New[string, string](&otter.Options[string, string]{
		ExpiryCalculator: otter.ExpiryWriting[string, string](defaultSessionAffinityTTL),
	})
	if err != nil {
		log.Error().Err(err).Msg("bridge: create session affinity cache")
		return nil
	}
	return cache
}

func (h *Handler) sessionCredential(key string) (string, bool) {
	if h == nil || h.sessions == nil || key == "" {
		return "", false
	}
	return h.sessions.GetIfPresent(key)
}

func (h *Handler) bindSessionCredential(key string, credentialID string) {
	if h == nil || h.sessions == nil || key == "" || credentialID == "" {
		return
	}
	// 会话亲和只做 bridge 层的内存级软绑定，provider 通过 key 前缀隔离
	h.sessions.Set(key, credentialID)
}
