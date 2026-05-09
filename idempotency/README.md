# idempotency

Redis-backed claim-once primitive for deduplicating message processing across
consumers. A single `TryClaim` call is an atomic "did I see this message ID
yet?" check — the first caller wins and processes; everyone else skips and
ACKs the duplicate.

## Why

At-least-once delivery is the norm for message brokers (Kafka, RabbitMQ,
SQS, Pub/Sub, etc.). Consumers need a fast, shared, atomic way to ask:

> *Have we already processed this message?*

A naïve `EXISTS k → SET k` has a race: two consumers see "absent" and both
process. The correct primitive is `SET NX PX` — atomic, with TTL. This
package wraps that pattern, adds a deduplication key layout, and exposes a
narrow contract that's hard to misuse.

## Install

```bash
go get github.com/redhajuanda/komon/idempotency
```

Requires Go 1.25+.

## Contract

```go
type Idempotency interface {
    // TryClaim atomically claims the idempotency key for the given topic
    // and message ID. Returns true if claimed (caller should process),
    // false if already processed (caller should skip/ACK). Keys expire
    // after ttl to limit storage growth.
    TryClaim(ctx context.Context, topic, messageID string, ttl time.Duration) (claimed bool, err error)
}
```

`ttl` should be **longer than the broker's maximum redelivery window** so a
late retry of the same message still finds the dedupe key alive. For most
brokers a few hours is enough; for systems with long retry windows
(e.g. compacted log topics) bias higher.

## Quick start

### Standalone Redis

```go
import (
    "context"
    "time"

    "github.com/redhajuanda/komon/common"
    "github.com/redhajuanda/komon/idempotency"
)

s, err := idempotency.NewRedis(ctx, idempotency.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel: false,
        Hosts:    []string{"localhost:6379"},
    },
})
if err != nil { /* ... */ }
defer s.Close()
```

### Redis Sentinel

```go
s, err := idempotency.NewRedis(ctx, idempotency.RedisOption{
    RedisOption: common.RedisOption{
        Sentinel:   true,
        Hosts:      []string{"sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"},
        MasterName: "mymaster",
    },
})
```

### Wrap an existing client

```go
cli := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
s := idempotency.NewRedisClient(cli)
defer s.Close()
```

## Usage

### Consumer loop

```go
for msg := range messages {
    claimed, err := s.TryClaim(ctx, "orders", msg.ID, 6*time.Hour)
    if err != nil {
        // Surface to your error handling — do NOT process if you can't tell.
        return err
    }
    if !claimed {
        msg.Ack() // already processed; just acknowledge the duplicate
        continue
    }

    if err := process(ctx, msg); err != nil {
        // Decision: nack and let the broker redeliver. The dedupe key is
        // still in place, so a different consumer that picks it up will
        // skip. If you want retry-on-failure semantics, scope the claim
        // to a *successful* outcome instead — see "Two-phase claim" below.
        msg.Nack()
        continue
    }
    msg.Ack()
}
```

### Two-phase claim (retry-friendly)

If you want failed processing to *retry*, don't claim before processing.
Instead, claim only on success, and pair it with a short-lived in-flight
lock (e.g. via [`komon/dlock`](../dlock/README.md)) so two consumers don't
do the work simultaneously:

```go
// 1. Take a short lock to serialize processing.
if err := lock.TryLock(ctx, "msg:"+msg.ID, 30*time.Second); err != nil {
    if errors.Is(err, dlock.ErrNotAcquired) { return nil }
    return err
}
defer lock.Unlock(ctx, "msg:"+msg.ID)

// 2. Was it already processed by a previous attempt?
claimed, _ := s.TryClaim(ctx, "orders", msg.ID, 6*time.Hour)
if !claimed { msg.Ack(); return nil }

// 3. Process. If this fails, the claim stays — by design — so the next
// retry sees "already processed" and skips. If you want true retry,
// delete the claim on failure: s.client.Del(ctx, key).
if err := process(ctx, msg); err != nil {
    return err
}
msg.Ack()
```

Pick a model up-front and stick to it; mixing them silently corrupts your
delivery guarantees.

### Custom key layouts

The default key is `idem:<topic>:<messageID>`. Override:

```go
// Just the prefix.
s := idempotency.NewRedisClient(cli, idempotency.WithKeyPrefix("svc:dedupe:"))

// Full builder (the returned key is used verbatim).
s := idempotency.NewRedisClient(cli, idempotency.WithKeyBuilder(
    func(topic, id string) string {
        return "ns/" + topic + "/" + id
    },
))
```

| Option | Default | Purpose |
|---|---|---|
| `idempotency.WithKeyPrefix(p)` | `idem:` | prefix prepended to `<topic>:<messageID>` |
| `idempotency.WithKeyBuilder(fn)` | nil | full control over key layout |

## Errors

| Error / case | Meaning |
|---|---|
| `idempotency.ErrClosed` | `TryClaim` called after `Close` |
| ttl ≤ 0 | rejected — would never expire and leak storage |
| any other error | network/redis failure — **do not assume claim status** |

> **Important:** if `TryClaim` returns an error, you do *not* know whether
> the claim landed. Treat it as "unsafe to process" — surface it and let
> the broker redeliver later.

## Semantics & gotchas

- **TTL must outlive your worst redelivery window.** If a retry arrives
  after the dedupe key expired, you will reprocess.
- **Topic is part of the key.** The same `messageID` from different topics
  can both be claimed independently — that's intentional.
- **The store is fail-closed on errors.** No silent "true" on a Redis
  outage; the caller must decide.
- **`Close` is idempotent** and only closes the underlying client when
  this package created it.

## Testing

The package is exercised against [`miniredis`](https://github.com/alicebob/miniredis)
with table-driven tests covering: first-wins/second-skips, topic isolation,
reclaim after TTL, 64-goroutine concurrent claim → exactly one winner,
non-positive TTL rejection, post-Close error, idempotent Close, and custom
key layouts.

```bash
go test ./idempotency/... -race -cover
```

## When *not* to use this

- Your broker already provides exactly-once semantics (e.g. Kafka EOS
  inside the same cluster) and you don't cross brokers.
- You need transactional dedupe with the message effects (e.g. "process
  and store atomically"). Use a database transaction that includes the
  dedupe row.
- Your message volume × TTL exceeds Redis memory. Scale horizontally or
  shorten TTL — but don't shorten below the redelivery window.
