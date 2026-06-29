// Package memorystore provides an oss.nandlabs.io/golly/cache.Cache
// backend for Google Cloud Memorystore for Redis. It wraps
// go-redis/v9's universal client so the same code works against
// Standard, Standard High-Availability, and Memorystore for Redis
// Cluster deployments (clustered tiers).
//
// Both AUTH-token and IAM (OAuth2) auth are supported; pair the IAM
// helper in iam.go with Config.CredentialsProvider for short-lived
// rotating tokens when running on Memorystore for Redis 7.x.
package memorystore

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"oss.nandlabs.io/golly/cache"
)

// Config configures a Memorystore connection.
//
// Addrs accepts a single Memorystore primary endpoint for Standard /
// Standard HA tiers; for Memorystore for Redis Cluster, list one or
// more discovery endpoints — go-redis discovers shards automatically.
type Config struct {
	// Addrs is the list of Memorystore endpoints (host:port).
	Addrs []string

	// Username is used with RBAC / IAM. Leave empty for AUTH-only
	// instances. With IAM auth on Memorystore for Redis 7.x, set this
	// to the IAM principal email or service-account ID.
	Username string

	// Password is a static AUTH token. Ignored if CredentialsProvider is
	// also set.
	Password string

	// CredentialsProvider returns a fresh AUTH token on every connection
	// attempt. Use IAMAuthProvider for Memorystore for Redis 7.x IAM auth
	// — Google OAuth2 access tokens expire ~1 hour, and the underlying
	// token source caches and refreshes transparently.
	CredentialsProvider func(ctx context.Context) (string, error)

	// TLSConfig controls TLS in transit. The zero value enables TLS with
	// MinVersion = TLS 1.2 (required by Memorystore for in-transit
	// encryption). Set a custom *tls.Config for private CAs.
	TLSConfig *tls.Config

	// DisableTLS forces plaintext. Default false. Use only against test
	// endpoints (miniredis, local fakes).
	DisableTLS bool

	// PoolSize, ReadTimeout, WriteTimeout, DialTimeout — passthroughs to
	// go-redis's UniversalOptions. Zero values use go-redis defaults.
	PoolSize     int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	DialTimeout  time.Duration
}

// Client is a cache.Cache[string, []byte] backed by Memorystore for Redis.
type Client struct {
	rdb redis.UniversalClient
}

// Compile-time check that Client satisfies cache.Cache[string, []byte].
var _ cache.Cache[string, []byte] = (*Client)(nil)

// ErrInvalidConfig is returned by New when Config is unusable.
var ErrInvalidConfig = errors.New("memorystore: invalid config")

// New returns a ready-to-use Client. Returns ErrInvalidConfig if Addrs is
// empty. The underlying redis client connects lazily — New itself does
// not perform I/O. Call Client.Ping(ctx) to validate connectivity.
func New(cfg *Config) (*Client, error) {
	if cfg == nil || len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("%w: at least one address required", ErrInvalidConfig)
	}

	opts := &redis.UniversalOptions{
		Addrs:        cfg.Addrs,
		Username:     cfg.Username,
		Password:     cfg.Password,
		PoolSize:     cfg.PoolSize,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		DialTimeout:  cfg.DialTimeout,
	}

	if cfg.CredentialsProvider != nil {
		username := cfg.Username
		opts.CredentialsProviderContext = func(ctx context.Context) (string, string, error) {
			token, err := cfg.CredentialsProvider(ctx)
			if err != nil {
				return "", "", fmt.Errorf("memorystore: credentials provider: %w", err)
			}
			return username, token, nil
		}
	}

	if !cfg.DisableTLS {
		if cfg.TLSConfig != nil {
			opts.TLSConfig = cfg.TLSConfig
		} else {
			opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
	}

	return &Client{rdb: redis.NewUniversalClient(opts)}, nil
}

// Ping verifies connectivity by issuing a Redis PING.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Get returns the value for key. Missing keys return (nil, false).
func (c *Client) Get(ctx context.Context, key string) ([]byte, bool) {
	v, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	return v, true
}

// Set stores value under key with no expiry.
func (c *Client) Set(ctx context.Context, key string, value []byte) error {
	return c.rdb.Set(ctx, key, value, 0).Err()
}

// SetWithTTL stores value under key with the given TTL. A ttl of
// cache.NoExpiry (zero) means the entry never expires.
func (c *Client) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// Delete removes the key. Returns true when the key existed.
func (c *Client) Delete(ctx context.Context, key string) bool {
	n, err := c.rdb.Del(ctx, key).Result()
	if err != nil {
		return false
	}
	return n > 0
}

// Has reports whether key is present (and not expired server-side).
func (c *Client) Has(ctx context.Context, key string) bool {
	n, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false
	}
	return n > 0
}

// Clear empties the cache via FLUSHDB. Destructive admin op — only call
// against caches that exclusively belong to this application.
func (c *Client) Clear(ctx context.Context) error {
	switch v := c.rdb.(type) {
	case *redis.ClusterClient:
		return v.ForEachMaster(ctx, func(ctx context.Context, m *redis.Client) error {
			return m.FlushDB(ctx).Err()
		})
	default:
		return c.rdb.FlushDB(ctx).Err()
	}
}

// Len returns the total number of keys via DBSIZE. For cluster
// deployments it sums DBSIZE across masters. Approximate under
// concurrent writes.
func (c *Client) Len(ctx context.Context) int {
	switch v := c.rdb.(type) {
	case *redis.ClusterClient:
		total := int64(0)
		_ = v.ForEachMaster(ctx, func(ctx context.Context, m *redis.Client) error {
			n, err := m.DBSize(ctx).Result()
			if err == nil {
				total += n
			}
			return nil
		})
		return int(total)
	default:
		n, err := c.rdb.DBSize(ctx).Result()
		if err != nil {
			return 0
		}
		return int(n)
	}
}

// Close releases the underlying redis client and its connection pool.
// Safe to call multiple times.
func (c *Client) Close() error {
	return c.rdb.Close()
}
