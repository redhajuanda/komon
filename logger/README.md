# logger

## 1. What it is

This package is a small **structured logging façade** built on [**Logrus**](https://github.com/sirupsen/logrus) (`github.com/sirupsen/logrus`). It attaches a fixed `service` field to every entry, optionally pulls **`request_id`** and **`correlation_id`** from `context.Context` via [`github.com/redhajuanda/komon/tracer`](../tracer), adds optional **`stack`** frames for errors that implement **`github.com/pkg/errors`–style stack traces**, injects a **`source`** field (`file:line (operation)` from `runtime.Caller`), and can **redact** configured keys (including nested maps and slices). Global helpers configure the underlying Logrus logger **after** initialization.

## 2. Why

- **Same shapes across services**: Every log line carries `service` plus consistent optional tracing fields, so ingestion and dashboards stay predictable.
- **Less boilerplate**: `WithContext` ties logs to HTTP/async traces without repeating header propagation logic.
- **Safer debugging**: `WithStack` serializes stack traces when errors support them; `source` points to the call site without manual file/line logging.
- **Privacy/compliance helpers**: `RedactedFields` masks sensitive keys case-insensitively and walks nested structures.

Rolling your own usually means reimplementing context keys, stack formatting, redaction, or scattering Logrus calls with inconsistent fields—this package centralizes those behaviors behind a narrow interface.

## 3. Install

Requires **Go 1.25+** (see repository root `go.mod`).

```bash
go get github.com/redhajuanda/komon/logger
```

Dependencies include Logrus and the sibling `tracer` package in this module.

## 4. Contract — `Logger` interface

All chaining methods (`WithContext`, `WithStack`, `WithParam`, `WithParams`, `SkipSource`) return a **new** `Logger` sharing configuration; they do not mutate the receiver’s stored entry for sibling loggers.

| Method | Behavior |
|--------|----------|
| `WithContext(ctx context.Context)` | If `ctx` is non-nil, reads `request_id` and `correlation_id` from the tracer package’s context values. Empty strings are omitted (no field added). |
| `WithStack(err error)` | Adds field `stack` with `tracer.MarshalStack(err)`. If `err` does not implement `StackTracer` (`StackTrace() pkg/errors.StackTrace`), `stack` is **`nil`**—there is **no** `err != nil` guard in the implementation; callers typically pass a non-nil wrapped error. |
| `WithParam(key, value)` | Adds one field; value is passed through redaction/masking by key and recursive rules. |
| `WithParams(params Params)` | Adds multiple fields (`Params` is `map[string]any`); values are masked recursively where applicable. |
| `SkipSource()` | Subsequent log calls from this logger **do not** append the `source` field (see §9). |
| `Error` / `Errorf` | Log at error level; append `source` unless skipped. |
| `Info` / `Infof`, `Warn` / `Warnf`, `Debug` / `Debugf` | Same pattern as Logrus for the respective levels. |
| `Fatal` / `Fatalf` | Log then **exit the process** (Logrus default: `os.Exit(1)`). |
| `Panic` / `Panicf` | Log then **panic** (Logrus behavior). |

Package-level **`New`** is the factory: it returns a `Logger` and initializes internal global state used by **`SetOutput`**, **`SetFormatter`**, and **`SetLevel`** (see §9).

There are **no** package-level sentinel errors; failures are ordinary Logrus I/O/format errors where applicable.

## 5. Quick start

There is a **single initialization model**: create the logger once with a service name, then configure Logrus globally. There is no alternate “mode” comparable to Redis standalone vs Sentinel; configuration is **process-wide** after `New`.

```go
package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/redhajuanda/komon/logger"
	"github.com/redhajuanda/komon/tracer"
)

func main() {
	log := logger.New("my-service", logger.Options{
		RedactedFields: []string{"password", "authorization", "token"},
	})

	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	ctx := tracer.SetRequestID(context.Background(), "req-abc")
	ctx = tracer.SetCorrelationID(ctx, "corr-xyz")

	log.WithContext(ctx).
		WithParam("user_id", 42).
		Info("ready")
}
```

**Order matters**: call `New` before `SetOutput` / `SetFormatter` / `SetLevel`, or those functions will dereference a nil global store and **panic**.

## 6. Usage

### Structured fields and hot paths

```go
log.WithParams(logger.Params{
	"op":    "checkout",
	"ms":    elapsed.Milliseconds(),
	"retry": attempt,
}).Infof("completed in %dms", elapsed.Milliseconds())
```

### Request-scoped tracing

Populate context with `tracer.SetRequestID` / `tracer.SetCorrelationID` (or your middleware that does the same). Then:

```go
log.WithContext(ctx).Info("handling request")
```

### Errors with stack traces

`WithStack` is most useful when `err` was built with wrappers that expose `github.com/pkg/errors` stack traces (for example chains using `github.com/cockroachdb/errors` or `github.com/pkg/errors`).

```go
import "github.com/cockroachdb/errors"

// ...

if err != nil {
	log.WithContext(ctx).
		WithStack(err).
		WithParam("route", r.URL.Path).
		Errorf("handler failed: %v", err)
}
```

### Redaction

Configure keys once in `Options.RedactedFields`. Matching is **case-insensitive** on map keys. Nested `map[string]any`, `Params`, `[]any`, and `[]map[string]any` are traversed so inner keys like `password` are masked as `[REDACTED]`.

```go
log := logger.New("api", logger.Options{
	RedactedFields: []string{"password", "api_key"},
})
log.WithParams(logger.Params{
	"user": map[string]any{"name": "jane", "password": "x"},
}).Info("signup")
```

### Skipping `source`

Use when the automatic `source` field is noisy or misleading (e.g., tiny wrapper functions):

```go
log.SkipSource().Debug("verbose trace")
```

## 7. Options

`New` accepts `options ...Options`. **Only the first `Options` value is used** if you pass more than one; extras are ignored.

| Field | Default | Purpose |
|-------|---------|---------|
| `RedactedFields` | `nil` (no masking) | Key names (any case) whose values become `"[REDACTED]"` in `WithParam` / `WithParams`, including nested structures. |

There are no `With*` functional options on this API—configuration is this struct plus Logrus `Set*` package functions.

## 8. Errors

This package does not define exported sentinel errors.

| Situation | What happens |
|-----------|----------------|
| Invalid keys / bad format strings | Same as underlying Logrus (e.g. formatting issues). |
| `Fatal` / `Fatalf` | Process terminates after writing the log. |
| `Panic` / `Panicf` | Panic after writing the log. |
| `SetOutput` / `SetFormatter` / `SetLevel` before `New` | **Panic** (nil global store). |

## 9. Semantics and gotchas

- **Global singleton**: `New` assigns a package-level `logStore`. A **second** `New` replaces that store; earlier references to loggers returned from a previous `New` may still hold old entries depending on chaining— treat **`New` as once-per-process** initialization unless you fully understand the sharing model.
- **`SetOutput` / `SetFormatter` / `SetLevel`** affect the **same** underlying Logrus logger created by the **last** successful `New`.
- **`source` mutation**: Each log call (`Info`, `Errorf`, …) may call `appendSource`, which does `entry.WithField("source", ...)`. That **mutates the embedded `*logrus.Entry`** for that branch of the chain—chained loggers derived from the same entry can see order-dependent `source` behavior; prefer **one chain per line** or use `SkipSource` when isolating.
- **`WithStack` and `nil` / non-stack errors**: There is no check for `err == nil`; `stack` may be `nil` in JSON output. Prefer `if err != nil { log.WithStack(err)... }`.
- **Stack support**: Plain `errors.New` values do not produce frames; use a stack-capable error package if you rely on `stack`.
- **Tracer contract**: IDs only appear when set on the context via the `tracer` package’s keys; missing values are silently omitted.

## 10. Testing

From the repository root:

```bash
go test ./logger -count=1
```

**Covered today** (see `logger_test.go`): field masking for `WithParams` and `WithParam` (nested maps, arrays, case insensitivity), and a benchmark for logging with stack. Run with `-race` in CI if you introduce concurrent logging tests.

## 11. When not to use this

- You need **multiple independent root loggers** with different outputs/levels in one process—this package’s global `Set*` helpers and `logStore` are a poor fit.
- You standardize on **`log/slog`** or OpenTelemetry logging bridges only and want zero Logrus dependency.
- You require **per-request log level** or logger instances without shared global state—build on `logrus.Logger` directly or another abstraction.

For distributed tracing separately from log fields, combine this with your OTel pipeline; this package focuses on **log fields** (`request_id`, `correlation_id`, `stack`, `source`) rather than replacing tracing exports.
