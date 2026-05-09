# komon

**Komon Library** — a curated set of Go packages that codify the
patterns our services share: caching, distributed coordination, structured
errors, observability, pagination. Each package is independently importable;
take only what you need.

```
github.com/redhajuanda/komon
```

## Why

Across services we kept rewriting the same code: a Redis cache wrapper, a
"claim once" idempotency check, a distributed lock, structured logging,
OpenTelemetry boilerplate, error wrapping with stack traces, paginated query
plumbing. komon picks one good answer per problem, ships it with tests, and
keeps the surface narrow so it doesn't become a kitchen sink.

## Packages

| Package | Purpose | Status |
|---|---|---|
| [`cache`](./cache/README.md) | Redis-backed cache: get/set, MGET, hashes, set-of-keys index, counters, pattern delete. JSON / MessagePack serializers, jittered TTLs, OTel built in. | Stable |
| [`dlock`](./dlock/README.md) | Distributed lock (Redlock via redsync) with a token-free, id-based contract. `TryLock` / `Lock` / `Unlock` over Redis or Sentinel. | Stable |
| [`idempotency`](./idempotency/README.md) | Claim-once primitive (`SET NX PX`) for deduplicating message processing across consumers. | Stable |
| [`logger`](./logger/) | Context-aware structured logger (logrus-based) with request-ID and trace-ID propagation. | Stable |
| [`tracer`](./tracer/) | OpenTelemetry setup helpers (Jaeger exporter, request-ID middleware, stack-trace utilities). | Stable |
| [`fail`](./fail/) | Error type with cause chains, public-safe failure registry, and stack-aware extraction. | Stable |
| [`pagination`](./pagination/) | Request/response shapes for offset and cursor pagination. | Stable |
| [`common`](./common/) | Shared low-level utilities (pointer helpers, Redis client construction shared by `cache`/`dlock`/`idempotency`, sentinel dial overrides). | Internal-ish |

## Install

Each package is imported under its own path. Pull only what you need:

```bash
go get github.com/redhajuanda/komon/cache
go get github.com/redhajuanda/komon/dlock
# ...etc
```

Requires Go 1.25+.

## Quick example

```go
import (
    "context"
    "time"

    "github.com/redhajuanda/komon/cache"
    "github.com/redhajuanda/komon/common"
)

func main() {
    ctx := context.Background()
    c, err := cache.NewRedis(ctx, cache.RedisOption{
        RedisOption: common.RedisOption{
            Sentinel:   true,
            Hosts:      []string{"sentinel-1:26379", "sentinel-2:26379"},
            MasterName: "mymaster",
        },
        UseMsgPack: true,
    })
    if err != nil { panic(err) }
    defer c.Close()

    _ = c.Set(ctx, "user:1", User{ID: 1, Name: "Ada"}, cache.WithTTLJitter(cache.TTLMedium))
}
```

See each package's `README.md` for full usage.

## Conventions

- **Independent imports.** No package re-exports another's API. If you
  import `cache`, you don't accidentally pull in `tracer`.
- **`common` is shared infra.** Construction primitives (`common.RedisOption`,
  `common.NewRedisClient`) used by `cache`, `dlock`, and `idempotency` live
  here so dial / ping / OTel logic has one source of truth. Nothing
  application-level lives in `common`.
- **Context-first.** Every IO method takes `ctx` as its first argument and
  honors cancellation. Constructors that talk to the network (e.g. Redis
  Sentinel ping) accept `ctx` so callers can bound startup time.
- **Errors are sentinels you can match.** Every package exports its public
  errors as named variables; use `errors.Is` to branch on them.
- **No global state.** Constructors return values; callers own lifetime.
  `Close()` is idempotent.
- **Tests against real-ish backends.** Redis-backed packages use
  [`miniredis`](https://github.com/alicebob/miniredis) in their tests; we
  don't mock the wire format.

## Testing

```bash
go vet ./...
go test ./... -race -cover
```

CI runs the same.

## Documentation

Every package ships a `README.md` covering: what it is, why it exists, how
to install, the contract, quick start, usage, options, errors, gotchas,
and when *not* to use it.

If you're adding a new package or refreshing an existing one, follow
[`docs/PACKAGE_README_GUIDE.md`](./docs/PACKAGE_README_GUIDE.md) for the
exact structure and tone.

## Contributing

- Keep each package's surface narrow. New methods need a real, recurring
  use case, not a hypothetical one.
- New public APIs require tests and a README update in the same change.
- Prefer fixing existing primitives over adding new ones.
- Don't introduce shared abstractions before the third user emerges.

## License

MIT License
