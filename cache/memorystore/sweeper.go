package memorystore

// Sweep satisfies cache.Sweeper for the Memorystore backend.
//
// Redis expires keys eagerly on access and lazily via its own background
// sampler — this sweep is a no-op for API compatibility (satisfies
// cache.Sweeper) and always returns 0.
//
// Callers who need a precise live count on a Cache[K,V] value can still
// invoke Sweep unconditionally: on Redis it costs nothing, and on backends
// that hold expired entries in memory (e.g. golly/cache.InMemory) it
// reclaims them. The Sweeper interface exists so higher-level code can
// stay backend-agnostic.
func (c *Client) Sweep() int {
	return 0
}
