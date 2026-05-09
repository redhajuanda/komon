# cache

Redis-backed cache primitives for Go services: typed get/set with pluggable
serialization, batch and pipelined operations, hash and set helpers, atomic
counters, and pattern-based deletion. Built on
[`redis/go-redis/v9`](https://github.com/redis/go-redis) with OpenTelemetry
tracing and metrics wired in.

## Why

Most services need the same handful of caching patterns — single-key
get/set, MGET fan-out, hash-per-entity, "set of related keys", rate-limit
counters — and most reinvent them. This package centralizes them with three
goals:

- **Symmetric Set/Get.** Whatever you put in comes back out, regardless of
  serializer (JSON or MessagePack).
- **Stampede-resistant TTLs.** All write paths support per-item TTL jitter
  so simultaneously-warmed entries don't expire in lockstep.
- **Production observability by default.** Sentinel construction
  registers OpenTelemetry tracing and metrics automatically.

## Install

```bash
go get github.com/redhajuanda/komon/cache
```

Requires Go 1.25+.

## Quick start

### Standalone Redis

```go
import (
    "context"
    "time"

    "github.com/redhajuanda/komon/cache"
    "github.com/redhajuanda/komon/common"
)

c, err := cache.NewRedis(ctx, cache.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel: false,
        Hosts:    []string{"localhost:6379"},
    },
    UseMsgPack: true,
})
if err != nil { /* ... */ }
defer c.Close()
```

### Redis Sentinel

```go
c, err := cache.NewRedis(ctx, cache.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel:   true,
        Hosts:      []string{"sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"},
        MasterName: "mymaster",
        Password:   os.Getenv("REDIS_PASSWORD"),
    },
    UseMsgPack: true,
})
```

### Wrap an existing client (tests, custom pools)

```go
cli := redis.NewClient(&redis.Options{Addr: mr.Addr()}) // miniredis in tests
c := cache.NewRedisClient(cli, false /* useMsgPack */)
// Optional: register OTel on a real cluster.
_ = cache.InstrumentOTel(c)
```

## Usage

### Get / Set

```go
type User struct {
    ID   int64  `json:"id"   msgpack:"id"`
    Name string `json:"name" msgpack:"name"`
}

// Set with default 24h TTL.
_ = c.Set(ctx, "user:1", User{ID: 1, Name: "Ada"})

// Set with explicit TTL + jitter (recommended in hot paths).
_ = c.Set(ctx, "user:1", u, cache.WithTTLJitter(cache.TTLMedium))

var u User
err := c.Get(ctx, "user:1", &u)
if errors.Is(err, cache.ErrNotFound) { /* miss */ }
```

Primitives (`bool`, `int`, `float64`, …) round-trip cleanly: they are routed
through the configured serializer so `Get(&dest)` works for any type. Strings
and `[]byte` are written raw — read them back with `GetString` or via the
embedded `*redis.Client`.

### High-traffic preset

```go
opts := cache.HighTrafficOpts(cache.TTLLarge) // jittered TTL + msgpack
_ = c.Set(ctx, key, value, opts...)
_ = c.Get(ctx, key, &dest, opts...)
```

### MGET (index-aligned)

```go
keys := []string{"u:1", "u:2", "missing", "u:4"}
var users []*User // misses become nil entries
_ = c.MGet(ctx, keys, &users)
```

`*[]T` zero-fills misses; `*[]*T` leaves them as nil.

### Hashes

```go
_ = c.HSet(ctx, "user:1:profile", []*cache.DataSet{
    {Key: "name", Value: "Ada"},
    {Key: "age",  Value: 36},
}, cache.WithTTL(time.Hour))

var name string
_ = c.HGet(ctx, "user:1:profile", "name", &name)

var fields []any
_ = c.HGetAll(ctx, "user:1:profile", &fields) // alphabetical field order
```

`HMGetPipelined` / `HMSetPipelined` fan out across many hash keys in a single
round trip — use them when you have N hashes to read/write.

### Set-of-keys index

```go
// Store an index of related cache keys.
_ = c.SetMember(ctx, "user:1:orders", []*cache.DataSet{
    {Key: "order:101"}, {Key: "order:102"}, // Value ignored
}, cache.WithTTL(cache.TTLLarge))

// Resolve them in one shot (SMEMBERS + MGET).
var orders []*Order
n, err := c.GetMember(ctx, "user:1:orders", &orders)
```

### Counters

```go
// Sliding window: TTL refreshes on every call.
n, _ := c.Increment(ctx, "rate:user:1", cache.WithTTL(time.Minute))

// Fixed window: TTL set only on first creation (Redis 7+).
n, _ := c.IncrementFixed(ctx, "rate:user:1", cache.WithTTL(time.Minute))
```

Use `IncrementFixed` for rate limiters — `Increment`'s sliding TTL means a
hot key never expires.

### Existence + deletion

```go
ok, err := c.Exists(ctx, "user:1") // (bool, error) — outage is distinguishable from absence
_ = c.Del(ctx, "user:1", "user:2")
_ = c.DeleteWithPattern(ctx, "user:*") // SCAN + DEL in batches
```

## Options

| Option | Effect |
|---|---|
| `cache.WithTTL(d)` | fixed TTL |
| `cache.WithTTLJitter(d, [pct])` | TTL with random ±jitter (default 10%) |
| `cache.WithMsgPack()` | switch to MessagePack for this call |
| `cache.WithJSON()` | switch to JSON for this call |
| `cache.HighTrafficOpts(ttl)` | jittered TTL + msgpack preset |

Recommended TTL bands (driven by collection size):

| Constant | Value | Use when |
|---|---|---|
| `cache.TTLVeryLarge` | 2h | >500K rows |
| `cache.TTLLarge` | 6h | 50K–500K rows |
| `cache.TTLMedium` | 12h | 5K–50K rows |
| `cache.DefaultExpire` | 24h | <5K rows (default) |

## Errors

- `cache.ErrNotFound` — returned on cache miss by `Get`, `GetString`, `GetInt`,
  `HGet`, `HGetAll`, and `GetMember`.
- `cache.ErrInvalidSliceDestination` — MGet/HGetAll dest must be a non-nil
  pointer to `[]T` or `[]*T`.

## Testing

The package is exercised against [`miniredis`](https://github.com/alicebob/miniredis)
in unit tests and ships table-driven coverage for all public methods.

```bash
go test ./cache/... -race -cover
```

## When *not* to use this

- You need read-through / write-through semantics. This package is a
  thin Redis wrapper, not a cache-aside framework.
- You need cross-region replication semantics. Use a dedicated tool.
- You need strict serializability. Cache the value separately and serialize
  through a database.
