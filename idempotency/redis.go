package idempotency

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/redhajuanda/komon/common"
	"github.com/redis/go-redis/v9"
)

// RedisOption configures a Redis-backed Store. The shared dial/ping/OTel
// concerns (and the Sentinel-vs-standalone toggle) live in
// common.RedisOption.
type RedisOption struct {
	common.RedisOption
}

// Store is a Redis-backed Idempotency. It is safe for concurrent use.
type Store struct {
	client      redis.UniversalClient
	opt         Options
	closeClient bool
	closed      atomic.Bool
}

// NewRedisClient wraps an existing redis.UniversalClient. The caller retains
// ownership of the client; Close on the store does not close it.
func NewRedisClient(client redis.UniversalClient, opts ...Option) *Store {
	return &Store{
		client:      client,
		opt:         applyOptions(opts),
		closeClient: false,
	}
}

// NewRedis constructs a Store backed by Redis. opt.Sentinel selects between
// a Sentinel failover client (true) and a standalone client (false). The
// ping uses ctx so callers can bound startup time. The store owns the
// client and will close it on Close.
func NewRedis(ctx context.Context, o RedisOption, opts ...Option) (*Store, error) {
	client, err := common.NewRedisClient(ctx, o.RedisOption)
	if err != nil {
		return nil, err
	}
	return &Store{
		client:      client,
		opt:         applyOptions(opts),
		closeClient: true,
	}, nil
}

func (s *Store) key(topic, messageID string) string {
	if s.opt.KeyBuilder != nil {
		return s.opt.KeyBuilder(topic, messageID)
	}
	return s.opt.KeyPrefix + topic + ":" + messageID
}

// TryClaim performs an atomic SET NX PX. The stored value is "1"; only the
// presence of the key matters.
func (s *Store) TryClaim(ctx context.Context, topic, messageID string, ttl time.Duration) (bool, error) {
	if s.closed.Load() {
		return false, ErrClosed
	}
	if ttl <= 0 {
		return false, errors.New("idempotency: ttl must be > 0")
	}
	return s.client.SetNX(ctx, s.key(topic, messageID), "1", ttl).Result()
}

// Close releases store-owned resources. Safe to call multiple times.
func (s *Store) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.closeClient {
		return s.client.Close()
	}
	return nil
}

// compile-time interface check
var _ Idempotency = (*Store)(nil)
