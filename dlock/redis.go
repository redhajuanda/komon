package dlock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redsync/redsync/v4"
	rsgoredis "github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redhajuanda/komon/common"
	"github.com/redis/go-redis/v9"
)

// RedisOption configures a Redis-backed DLocker. The shared dial/ping/OTel
// concerns (and the Sentinel-vs-standalone toggle) live in
// common.RedisOption; dlock has no fields beyond the shared ones today.
type RedisOption struct {
	common.RedisOption
}

type redisLocker struct {
	client      redis.UniversalClient
	rs          *redsync.Redsync
	opt         Options
	closeClient bool

	mutexes sync.Map // id (string) -> *redsync.Mutex
	closed  atomic.Bool
}

// NewRedisClient wraps an existing redis.UniversalClient (e.g. *redis.Client
// against miniredis or a single node). The caller retains ownership of the
// client; Close on the returned DLocker is a no-op for the client.
func NewRedisClient(client redis.UniversalClient, opts ...Option) DLocker {
	pool := rsgoredis.NewPool(client)
	return &redisLocker{
		client:      client,
		rs:          redsync.New(pool),
		opt:         applyOptions(opts),
		closeClient: false,
	}
}

// NewRedis constructs a DLocker backed by Redis. opt.Sentinel selects between
// a Sentinel failover client (true) and a standalone client (false). The ping
// uses ctx so callers can bound startup time. The locker owns the client and
// will close it on Close.
func NewRedis(ctx context.Context, o RedisOption, opts ...Option) (DLocker, error) {
	client, err := common.NewRedisClient(ctx, o.RedisOption)
	if err != nil {
		return nil, err
	}
	pool := rsgoredis.NewPool(client)
	return &redisLocker{
		client:      client,
		rs:          redsync.New(pool),
		opt:         applyOptions(opts),
		closeClient: true,
	}, nil
}

func (l *redisLocker) key(id string) string {
	return l.opt.KeyPrefix + id
}

// acquire builds a redsync.Mutex with the given retry budget and attempts
// acquisition. On success the mutex is recorded in l.mutexes for later Unlock.
func (l *redisLocker) acquire(ctx context.Context, id string, ttl time.Duration, tries int) error {
	if l.closed.Load() {
		return ErrClosed
	}
	if ttl <= 0 {
		return ErrNotAcquired
	}

	// Reserve the slot up-front so a concurrent Lock for the same id on the
	// same instance fails fast instead of racing on Redis. We store a
	// placeholder; the real mutex replaces it on success.
	placeholder := &redsync.Mutex{}
	if _, loaded := l.mutexes.LoadOrStore(id, placeholder); loaded {
		return ErrNotAcquired
	}

	mu := l.rs.NewMutex(
		l.key(id),
		redsync.WithExpiry(ttl),
		redsync.WithTries(tries),
		redsync.WithRetryDelay(l.opt.RetryDelay),
		redsync.WithDriftFactor(l.opt.DriftFactor),
	)

	var err error
	if tries == 1 {
		err = mu.TryLockContext(ctx)
	} else {
		err = mu.LockContext(ctx)
	}
	if err != nil {
		l.mutexes.Delete(id)
		return mapAcquireErr(ctx, err)
	}

	l.mutexes.Store(id, mu)
	return nil
}

func (l *redisLocker) TryLock(ctx context.Context, id string, ttl time.Duration) error {
	return l.acquire(ctx, id, ttl, 1)
}

func (l *redisLocker) Lock(ctx context.Context, id string, ttl time.Duration) error {
	return l.acquire(ctx, id, ttl, l.opt.Tries)
}

func (l *redisLocker) Unlock(ctx context.Context, id string) error {
	v, ok := l.mutexes.LoadAndDelete(id)
	if !ok {
		return ErrLockNotHeld
	}
	mu, ok := v.(*redsync.Mutex)
	if !ok || mu == nil || mu.Name() == "" {
		// Placeholder from an in-flight acquire that has not completed yet.
		return ErrLockNotHeld
	}

	ok, err := mu.UnlockContext(ctx)
	if err != nil {
		if errors.Is(err, redsync.ErrLockAlreadyExpired) {
			return ErrLockExpired
		}
		return err
	}
	if !ok {
		return ErrLockNotHeld
	}
	return nil
}

func (l *redisLocker) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Drop tracked mutexes; we do not attempt remote release on Close to
	// avoid surprising callers. Holders rely on TTL as the safety net.
	l.mutexes.Range(func(k, _ any) bool {
		l.mutexes.Delete(k)
		return true
	})
	if l.closeClient {
		return l.client.Close()
	}
	return nil
}

// mapAcquireErr translates redsync errors into the package's public sentinels.
// Context cancellation is preserved so callers can distinguish it from
// genuine contention.
func mapAcquireErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var taken *redsync.ErrTaken
	if errors.As(err, &taken) || errors.Is(err, redsync.ErrFailed) {
		return ErrNotAcquired
	}
	return err
}
