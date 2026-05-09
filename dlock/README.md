# dlock

Production-grade distributed lock for Go services, backed by Redis (standalone
or Sentinel) via the [Redlock](https://redis.io/docs/manual/patterns/distributed-locks/)
algorithm. Built on [`go-redsync/redsync/v4`](https://github.com/go-redsync/redsync)
with a token-free, id-based contract.

## Why

Some operations must run at most once across all replicas of a service:

- consuming a deduped job
- refreshing a shared cache without thundering-herd
- running a singleton periodic task
- implementing a "leader-election lite" guard

`dlock` gives you a single primitive for all of them. Unlike a raw redsync
mutex, the contract hides the lock token from callers — you `Lock(id)` and
`Unlock(id)` from anywhere, and the package tracks the fencing token
internally so the unlock releases exactly the lock you held.

## Install

```bash
go get github.com/redhajuanda/komon/dlock
```

Requires Go 1.25+.

## Contract

```go
type DLocker interface {
    TryLock(ctx context.Context, id string, ttl time.Duration) error  // non-blocking
    Lock(ctx context.Context, id string, ttl time.Duration) error     // blocking
    Unlock(ctx context.Context, id string) error
    Close() error
}
```

- `TryLock` returns `ErrNotAcquired` immediately if the lock is held.
- `Lock` retries with jittered backoff until it acquires the lock, the
  context is cancelled (returns `ctx.Err()`), or the retry budget is
  exhausted (returns `ErrNotAcquired`).
- `Unlock` releases the lock acquired by **this DLocker instance** for `id`.
  Returns `ErrLockNotHeld` if this instance doesn't hold it, or
  `ErrLockExpired` if the TTL elapsed before the unlock reached Redis.
- `Close` is idempotent and only closes the underlying client when this
  package created it.

## Quick start

### Standalone Redis

```go
import (
    "context"
    "time"

    "github.com/redhajuanda/komon/common"
    "github.com/redhajuanda/komon/dlock"
)

l, err := dlock.NewRedis(ctx, dlock.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel: false,
        Hosts:    []string{"localhost:6379"},
    },
})
if err != nil { /* ... */ }
defer l.Close()
```

### Redis Sentinel

```go
l, err := dlock.NewRedis(ctx, dlock.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel:   true,
        Hosts:      []string{"sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"},
        MasterName: "mymaster",
    },
})
```

### Wrap an existing client (tests, custom pools)

```go
cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
l := dlock.NewRedisClient(cli)
defer l.Close()
```

## Usage

### Try-lock (non-blocking)

```go
if err := l.TryLock(ctx, "job:nightly-rollup", 30*time.Second); err != nil {
    if errors.Is(err, dlock.ErrNotAcquired) {
        return nil // someone else is doing it
    }
    return err
}
defer l.Unlock(context.Background(), "job:nightly-rollup")
// critical section...
```

### Blocking lock with deadline

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

if err := l.Lock(ctx, "user:42:checkout", 10*time.Second); err != nil {
    return err // ctx.Err() if timed out, ErrNotAcquired if budget exhausted
}
defer l.Unlock(context.Background(), "user:42:checkout")
```

### Tuning retry behavior

```go
l := dlock.NewRedisClient(cli,
    dlock.WithTries(20),                       // retry budget for Lock
    dlock.WithRetryDelay(50*time.Millisecond), // base sleep between retries
    dlock.WithKeyPrefix("svc:locks:"),
)
```

| Option | Default | Purpose |
|---|---|---|
| `dlock.WithTries(n)` | 32 | maximum acquisition attempts in `Lock` |
| `dlock.WithRetryDelay(d)` | 200ms | base sleep between retries (redsync adds jitter) |
| `dlock.WithDriftFactor(f)` | 0.01 | clock-drift compensation (Redlock parameter) |
| `dlock.WithKeyPrefix(p)` | `dlock:` | prefix prepended to every Redis key |

## Errors

| Error | Meaning |
|---|---|
| `dlock.ErrNotAcquired` | Lock held by another caller, or retry budget exhausted |
| `dlock.ErrLockNotHeld` | `Unlock` called for an id this instance never acquired |
| `dlock.ErrLockExpired` | TTL elapsed before `Unlock` reached Redis (likely you ran past TTL) |
| `dlock.ErrClosed` | Operation called after `Close` |
| `context.Canceled` / `context.DeadlineExceeded` | Returned from `Lock` when ctx is done |

## Semantics & gotchas

- **Per-process single-holder per id.** A second `Lock` for the same `id`
  on the same `DLocker` instance fails fast with `ErrNotAcquired` rather
  than self-racing on Redis. Use distinct ids (or distinct instances) for
  re-entrant patterns.
- **TTL is your safety net.** If your process crashes between `Lock` and
  `Unlock`, the lock auto-releases when its TTL expires. Pick a TTL >
  the worst-case duration of your critical section. Re-acquire (with a
  new token) before TTL if you need to extend.
- **Redlock with one master is fine for Sentinel.** Redsync requires N
  independent masters only if you want the multi-master Redlock variant.
  Sentinel already provides HA; one redsync over one client is the
  documented standard.

## Testing

The package is exercised against [`miniredis`](https://github.com/alicebob/miniredis)
with table-driven tests covering: free/held/re-entry TryLock, blocking Lock
with release, ctx-cancel, unknown-id unlock, TTL expiry, post-expiry
re-acquisition, 32-goroutine winner-takes-one, contended counter race test,
idempotent Close, and key-prefix verification.

```bash
go test ./dlock/... -race -cover
```

## When *not* to use this

- You need exactly-once execution across machine failure with strict
  fencing tokens — use a transactional database with a dedicated lock
  table or a tool like ZooKeeper / etcd.
- Your critical section is < 1ms — the Redis round trip dominates.
  Consider an in-process `sync.Mutex` per shard plus a deduplication key.
- You need fair queueing / FIFO ordering. Redlock is opportunistic.
