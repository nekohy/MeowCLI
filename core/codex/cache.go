package codex

import (
	db "github.com/nekohy/MeowCLI/internal/store"
	"time"
)

// readCache 读取缓存；otter 的 TTL 机制已处理过期，
// GetIfPresent 只返回未过期条目
func (m *Manager) readCache(id string) (CodexCache, bool) {
	return m.cache.GetIfPresent(id)
}

// writeCache 写入缓存并通过 SetExpiresAfter 设置 TTL
func (m *Manager) writeCache(row db.Codex) {
	ttl := time.Until(m.availableUntil(row))
	if ttl <= 0 {
		return
	}
	m.cache.Set(row.ID, m.snapshotFromCodex(row))
	m.cache.SetExpiresAfter(row.ID, ttl)
}
