package auth

import (
	"context"
	"sync/atomic"

	db "github.com/nekohy/MeowCLI/internal/store"
)

type keySnapshot struct {
	byKey      map[string]db.AuthKey
	adminCount int
}

type KeyCache struct {
	store    db.Store
	snapshot atomic.Pointer[keySnapshot]
}

func NewKeyCache(store db.Store) *KeyCache {
	c := &KeyCache{store: store}
	c.snapshot.Store(&keySnapshot{byKey: make(map[string]db.AuthKey)})
	return c
}

func (c *KeyCache) Load(ctx context.Context) error {
	keys, err := c.store.ListAuthKeys(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]db.AuthKey, len(keys))
	admins := 0
	for _, k := range keys {
		m[k.Key] = k
		if k.Role == "admin" {
			admins++
		}
	}
	c.snapshot.Store(&keySnapshot{byKey: m, adminCount: admins})
	return nil
}

func (c *KeyCache) Lookup(key string) (db.AuthKey, bool) {
	snap := c.snapshot.Load()
	ak, ok := snap.byKey[key]
	return ak, ok
}

// NeedsSetup returns true when no admin key exists yet.
func (c *KeyCache) NeedsSetup() bool {
	return c.snapshot.Load().adminCount == 0
}

func (c *KeyCache) Refresh(ctx context.Context) error {
	return c.Load(ctx)
}
