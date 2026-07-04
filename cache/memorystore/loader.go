package memorystore

import (
	"context"
	"time"
)

// GetOrLoad satisfies cache.Loader[string, []byte] for the Memorystore
// backend. It returns the cached value for key; on a miss, load is invoked
// exactly once per concurrent miss (in-process single-flight) and the
// result is stored under key with the supplied TTL via SETEX-equivalent
// semantics. All concurrent callers waiting on the same key share the
// result of that single load.
//
// Semantics:
//   - Cache hit: return the cached value; load is not called.
//   - Cache miss: enter single-flight for key; re-check the cache under the
//     flight (a sibling flight may have populated it), otherwise call load,
//     write the value with SetWithTTL, and return.
//   - load error: propagated to every waiting caller; nothing is cached.
//
// The single-flight guarantee is per-process only: it protects the
// upstream loader from thundering-herd traffic from *this* process, not
// across every replica sharing the Memorystore instance. Applications
// that need cross-process coordination should layer a distributed lock
// (e.g. Redlock) on top.
func (c *Client) GetOrLoad(
	ctx context.Context,
	key string,
	ttl time.Duration,
	load func(context.Context) ([]byte, error),
) ([]byte, error) {
	// Fast path: hit without paying the single-flight bookkeeping cost.
	if v, ok := c.Get(ctx, key); ok {
		return v, nil
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		// Re-check under the flight — a sibling in-process flight may
		// have populated the key while we were queuing.
		if v, ok := c.Get(ctx, key); ok {
			return v, nil
		}
		loaded, err := load(ctx)
		if err != nil {
			// Do not cache on error — let the next caller retry the load.
			return nil, err
		}
		if err := c.SetWithTTL(ctx, key, loaded, ttl); err != nil {
			return nil, err
		}
		return loaded, nil
	})
	if err != nil {
		return nil, err
	}
	// v is any: a nil []byte loaded value survives the type assertion as
	// (nil, true); a nil interface (shouldn't happen on this branch) is
	// handled by the comma-ok form to avoid a panic.
	result, _ := v.([]byte)
	return result, nil
}
