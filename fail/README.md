# fail

## 1. What it is

The **fail** package models errors as **`Fail`**: an internal error with a **stack trace** (via [`github.com/cockroachdb/errors`](https://github.com/cockroachdb/errors) — `WithStackDepth`, `WrapWithDepthf`, etc.) plus an optional **`Failure`**: a **public-safe** triple of **`Code`**, **`Message`**, and **`HTTPStatus`** for API responses. Callers construct `Fail` with **`New` / `Newf` / `Wrap` / `Wrapf`**, attach a **`Failure`** and optional **`Data`**, and at the boundary use **`Extract`**, **`MustExtract`**, or **`FailureOf`** to render responses while logging **`OriginalError()`** internally.

## 2. Why

- **One type for “what broke” vs “what we tell the client”**: operators see stack-bearing internal errors; clients see stable codes and HTTP status.
- **Consistent handler code**: **`MustExtract`** always yields a usable `*Fail` (plain errors become wrapped with the default internal failure shape).
- **Builtin catalog**: Common HTTP-aligned **`Failure`** values are predefined (`ErrBadRequest`, `ErrNotFound`, …) so services share vocabulary.
- **`errors.Is` / `errors.As`**: `Fail` implements **`Unwrap()`** so the cockroachdb/errors chain stays inspectable.

Without this split, teams either leak internals in JSON or lose stack/context when translating to HTTP.

## 3. Install

Requires **Go 1.25+** (repository root `go.mod`).

```bash
go get github.com/redhajuanda/komon/fail
```

The implementation depends on **`github.com/cockroachdb/errors`** for stack capture and wrapping.

## 4. Contract

### `Failure` (`public.go`)

| Field / method | Semantics |
|----------------|-----------|
| `Code` | Application-level string code (e.g. `"404001"`). |
| `Message` | Text safe to return to API consumers. |
| `HTTPStatus` | Suggested HTTP status (e.g. `404`). |
| `TemperMessage`, `TemperHTTPStatus`, `TemperCode` | **Mutate the receiver in place** and return `*Failure`. See §9 — do **not** call these on shared builtins. |
| `Register(code, message, httpStatus)` | Allocates and returns a **new** `*Failure`. Not stored in a global registry beyond your own package-level variable. |

### `Fail` (`failure.go`)

| Constructor | Returns | Notes |
|-------------|---------|--------|
| `New(msg)` | `*Fail` | Internal message; stack at call site. |
| `Newf(format, args...)` | `*Fail` | Formatted internal message; stack at call site. |
| `Wrap(err)` | `*Fail` or **`nil`** | **`nil` if `err` is nil**. If `err` is already `*Fail` with non-nil `originalError`, returns that same value (no re-wrap). |
| `Wrapf(err, format, args...)` | `*Fail` or **`nil`** | Same nil and `*Fail` short-circuit as `Wrap`; if `err` is `*Fail` with inner error, **additional message is not applied** (see §9). |

| Chain / accessor | Semantics |
|------------------|-----------|
| `WithFailure(*Failure)` | Sets the public-facing failure; **mutates** the `Fail` and returns it. |
| `WithData(any)` | Optional payload (e.g. validation details); **mutates** and returns. |
| `OriginalError() error` | Full internal error for logs — **do not** send to clients. |
| `Data() any` | Extra data or **`nil`**. |
| `HasFailure() bool` | **`true`** only if `WithFailure` was used (public is non-nil). |
| `GetFailure() *Failure` | Returns paired failure, or **`ErrInternalServer`** if none set — **never nil**. |
| `Error() string` | Delegates to internal error string. |
| `Unwrap() error` | Inner error for `errors.Is` / `errors.As`. |

### Extraction helpers (`extract.go`)

| Function | Behavior |
|----------|----------|
| `Extract(err)` | `(nil, false)` if `err` is nil. Otherwise `errors.As` for `*Fail`. |
| `MustExtract(err)` | **`nil` if `err` is nil**. If chain contains `*Fail`, returns it. Else **`Wrap(err)`** (no `WithFailure` → `GetFailure()` is internal server error). |
| `IsFailure(err, pf *Failure) bool` | **`true`** only if a `*Fail` exists **and** `GetFailure() == pf` (**pointer equality**). |
| `FailureOf(err)` | **`nil` if `err` is nil**. If not a `*Fail`, returns **`ErrInternalServer`**. Else `GetFailure()`. |

## 5. Quick start

Define domain failures once (package level), return `Fail` from services, normalize at HTTP layer.

```go
package myapp

import (
	"errors"
	"database/sql"

	"github.com/redhajuanda/komon/fail"
)

var ErrShipmentGone = fail.Register("404012", "Shipment is no longer available", 404)

func mapShipmentLookupErr(id string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fail.Newf("shipment %s missing", id).WithFailure(ErrShipmentGone)
	}
	return fail.Wrap(err).WithFailure(fail.ErrInternalServer)
}
```

HTTP handler sketch:

```go
func handle(w http.ResponseWriter, r *http.Request) {
	if err := doWork(r.Context()); err != nil {
		f := fail.MustExtract(err)
		// log f.OriginalError() internally
		pf := f.GetFailure()
		// write JSON: pf.Code, pf.Message, pf.HTTPStatus, optional f.Data()
		_ = pf
		return
	}
}
```

## 6. Usage

### Attach public shape and payload

```go
return fail.Wrap(err).
	WithFailure(fail.ErrUnprocessable).
	WithData(map[string][]string{
		"email": {"must be a valid email"},
	})
```

### Prefer `MustExtract` at boundaries

Plain `error` values become `*Fail` via `Wrap`, so **`GetFailure()`** falls back to **`ErrInternalServer`** until you explicitly pair another failure upstream.

### Branch on a specific failure

`IsFailure` uses **pointer identity** — compare against the **same** `*Failure` you registered (typically a package-level `var`).

```go
if fail.IsFailure(err, ErrShipmentGone) {
	// ...
}
```

### When you only need HTTP mapping

```go
pf := fail.FailureOf(err)
if pf != nil {
	w.WriteHeader(pf.HTTPStatus)
}
```

### Logging

Always log **`f.OriginalError()`** (or the raw `err` before strip) so retention tools get stacks and wrapped causes.

## 7. “Options” — builtins and registration

There is no `Options` struct. Use **`Register`** and the predefined values:

| Builtin | Code (default) | HTTP |
|---------|----------------|------|
| `ErrInternalServer` | `500000` | 500 |
| `ErrBadRequest` | `400000` | 400 |
| `ErrUnauthorized` | `401000` | 401 |
| `ErrForbidden` | `403000` | 403 |
| `ErrNotFound` | `404000` | 404 |
| `ErrConflict` | `409000` | 409 |
| `ErrUnprocessable` | `422000` | 422 |
| `ErrTooManyRequest` | `429000` | 429 |

Add finer-grained codes with **`Register`** at startup / package init and assign to **`var`** so **`IsFailure`** can compare pointers reliably.

## 8. Errors table

Failures are **data**, not Go `error` sentinel variables (except as values you compare by pointer via **`IsFailure`**).

| Name | When to use |
|------|-------------|
| Builtin `Failure` vars above | Default HTTP/catalog responses. |
| Custom `*Failure` from `Register` | Domain-specific codes/messages/status. |
| `GetFailure()` fallback | Any `Fail` without `WithFailure` maps to **`ErrInternalServer`** for external responses. |

There are **no** exported `var ErrXyz error` sentinels in this package — the builtins are **`Failure`** pointers.

## 9. Semantics and gotchas

- **`Temper*` mutates shared state**: Calling **`TemperMessage`** / **`TemperHTTPStatus`** / **`TemperCode`** on **`fail.ErrNotFound`** (or any shared `Failure`) changes it for **the whole process**. Prefer **`Register`** with the final text/status, or copy into a new struct if you need variants.
- **`WithFailure` / `WithData` mutate the `Fail`**: They return `*Fail` for chaining but update the same struct; avoid sharing one `*Fail` across goroutines while mutating.
- **`IsFailure` is pointer equality** on **`GetFailure()`**, not code string equality. Two different `Register` calls with the same code produce **distinct** pointers — **`IsFailure`** would be false unless you compare the exact variable.
- **`GetFailure()` vs `HasFailure()`**: Use **`HasFailure`** when you must know if a public failure was **explicitly** set vs falling back to internal error.
- **`Wrap` / `Wrapf` on existing `*Fail`**: If the inner `originalError` is already set, **`Wrap` returns the same instance** and **`Wrapf` does not add** the formatted context (by design in current code — do not assume `Wrapf` always annotates).
- **`FailureOf(nil)`** is **`nil`**; **`MustExtract(nil)`** is **`nil`** — guard before dereferencing for logging/JSON.
- **Logger integration**: Packages like **`logger`** can use **`WithStack(err)`** when the error implements stack traces compatible with **`tracer.MarshalStack`** (typically pkg/errors-style chains); cockroachdb/errors fits that model when wrapping is consistent.

## 10. Testing

```bash
go test ./fail -count=1
```

This package currently has **no `_test.go` files** in-tree; add table-driven tests around **`Extract` / `MustExtract` / `IsFailure`** and custom **`Register`** failures as behavior grows.

## 11. When not to use this

- You only need **`errors.Is` / sentinel errors** without HTTP-facing codes or structured client payloads.
- You already standardize on **gRPC status** / **connect-go** codes end-to-end and do not want a parallel `Failure` model.
- You require **immutable** error values everywhere — **`Fail` / `Failure` tempering and `With*`** are mutable patterns tuned for ergonomics, not purely functional style.

For simple internal tools with no public API surface, the stdlib **`errors`** package may be enough; **fail** pays off when every error path must map cleanly to **HTTP + stable codes + optional `Data`**.
