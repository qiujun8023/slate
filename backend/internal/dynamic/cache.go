package dynamic

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	cacheMaxEntries = 256
	// cacheLoadTimeout bounds a shared upstream request, which outlives any one caller.
	cacheLoadTimeout = 2 * time.Minute
)

type cacheEntry struct {
	value   any
	fetched time.Time
}

// ttlCache shares one upstream request among concurrent callers and keeps successful
// results for ttl; several contents often point at the same city or source.
type ttlCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	group   singleflight.Group
}

func newCache() *ttlCache { return &ttlCache{entries: map[string]cacheEntry{}} }

func (c *ttlCache) get(ctx context.Context, key string, ttl time.Duration, now time.Time, load func(context.Context) (any, error)) (any, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Sub(e.fetched) < ttl {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()
	// The load is detached from ctx so one caller giving up does not fail the others;
	// each caller still stops waiting when its own ctx ends.
	ch := c.group.DoChan(key, func() (any, error) {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheLoadTimeout)
		defer cancel()
		v, err := load(lctx)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if len(c.entries) >= cacheMaxEntries {
			for k, e := range c.entries {
				if now.Sub(e.fetched) >= ttl || len(c.entries) >= cacheMaxEntries {
					delete(c.entries, k)
				}
			}
		}
		c.entries[key] = cacheEntry{v, now}
		c.mu.Unlock()
		return v, nil
	})
	select {
	case r := <-ch:
		return r.Val, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
