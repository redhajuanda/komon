# Package README convention

Every komon package ships a `README.md` at its root. This document defines
the structure, the order, and the tone — so a developer who has read one
README can navigate any other one in seconds.

This is **not** a place for marketing prose. The goal is engineer-to-engineer
truth: what the package does, the trade-offs it makes, and the
incident-grade gotchas you only learn the hard way.

## When to update

You **must** update the README in the same change that:

- adds, removes, or renames a public symbol (function, type, constant,
  error)
- changes a public method's signature, return values, or error semantics
- adds, removes, or renames an `Option`
- changes default behavior callers rely on (TTLs, retry budgets, prefixes)
- introduces a new caller pattern that wasn't documented before
- discovers a gotcha worth recording (an incident, a surprising behavior)

If the README would lie after your change, it's not done.

## Structure

Use this exact section order. Skip a section only if it genuinely doesn't
apply (most packages need all of them); never reorder.

1. **Title (h1)** — the package name only. No tagline.
2. **One-paragraph definition** — what it is, what it's built on, in two
   to four sentences. Name the underlying technologies (`go-redis`,
   `redsync`, `SET NX PX`) so readers can map to mental models they
   already have.
3. **Why** — the real problem it solves. Two or three short paragraphs or
   a bulleted "goals" list. Say *what makes this package's surface
   different* from rolling your own.
4. **Install** — the `go get` line and the minimum Go version.
5. **Contract** — the interface and a one-line behavior note per method.
   *Skip only if the package has no public interface.*
6. **Quick start** — the smallest working snippet for each construction
   mode in this order: standalone (or basic) → managed (Sentinel, hosted)
   → wrap-existing-resource (tests, custom pools). Same structure across
   packages so callers can switch modes by analogy.
7. **Usage** — the canonical patterns. Each pattern gets a `### h3`
   heading and a code block. Real, copy-pastable code — not pseudocode.
   Cover the things callers will actually do, not every API method
   exhaustively (that's what godoc is for).
8. **Options** — every `With*` and every public option struct field, in a
   table: `| Option | Default | Purpose |`. If there are presets (e.g.
   `HighTrafficOpts`), list them too.
9. **Errors** — every public error sentinel and the conditions that
   produce it, in a table: `| Error | Meaning |`. Include relevant
   stdlib errors callers should match (e.g. `context.Canceled`).
10. **Semantics & gotchas** — the bullets future-you will be glad you
    wrote: invariants that aren't obvious from the API, things that can
    surprise under load, what happens if you misuse it, what the TTL
    actually protects against. **This is the highest-leverage section in
    the file.**
11. **Testing** — what it's tested against, the command to run.
12. **When *not* to use this** — bullet list of situations where the
    reader should reach for a different tool. Honest beats promotional.

Optional but encouraged:

- A short cross-reference to sibling packages when they're commonly used
  together (e.g. `dlock` + `idempotency` for two-phase claim).

## Tone

- **Plain, direct sentences.** Aim for a senior engineer skimming on
  Monday morning. No hype words ("blazing fast," "powerful").
- **Active voice.** "The store rejects non-positive TTL." Not "Non-positive
  TTLs are rejected by the store."
- **Name the trade-offs.** If something is opportunistic (Redlock), say
  so. If something is fail-closed on errors, say so. If TTL is the safety
  net, say so.
- **No emojis.** No badges except CI/build status if/when they exist.
- **Code over prose.** A six-line code block is worth a paragraph.
- **Explain *why*, not just *what*.** "Use `IncrementFixed` for rate
  limiters — `Increment`'s sliding TTL means a hot key never expires."
  Not just "use `IncrementFixed` for rate limiters."

## Code block rules

- All examples must compile against the current API. If a snippet would
  fail `go build`, it doesn't ship.
- Show imports the first time a package appears in the file:

  ```go
  import (
      "context"
      "time"

      "github.com/redhajuanda/komon/cache"
      "github.com/redhajuanda/komon/common"
  )
  ```

  Subsequent snippets can omit imports.

- Prefer realistic identifiers (`user:1`, `orders`) over `foo`/`bar`.
- Use `ctx` for the context argument; don't redeclare it in every snippet.
- Show error handling at least once per snippet block. After that, `_ =`
  and `if err != nil { /* ... */ }` are fine for brevity.

## Tables

- Always have a header row.
- Two- or three-column max. If you need more, use a list under each item.
- Sort alphabetically *unless* there's a meaningful order (e.g. severity,
  call sequence) — in which case state it in the table caption.

## Linking

- Cross-package links are relative: `[`dlock`](../dlock/README.md)`.
- External links go inline, with the URL in parentheses; don't rely on
  reference-style links.
- Don't link to godoc unless you specifically want the symbol-level view.

## Anti-patterns

These have appeared in past drafts and should not survive review:

- **"This package provides…"** — always cuttable. Just say what it is.
- **A "Features" section.** Features belong in Usage.
- **A "Roadmap" section.** Use the issue tracker.
- **Class diagrams or ASCII art.** Almost never earns its space. Prefer a
  short prose explanation or a code snippet.
- **Long preamble before the install line.** Engineers want to type
  `go get` within the first scroll.
- **Restating godoc.** The README is for *patterns and pitfalls*, godoc is
  for the symbol catalog. Don't list every method again.
- **Ambiguous error guidance.** "On error, handle appropriately" is not
  documentation. Say what the error means and what to do with it.

## Template

A skeleton you can copy. Replace `<pkg>` and the bracketed placeholders;
delete sections that genuinely don't apply.

```markdown
# <pkg>

<One paragraph: what it is, what it's built on, in two to four sentences.>

## Why

<Two or three short paragraphs, or a bulleted goals list.>

## Install

\`\`\`bash
go get github.com/redhajuanda/komon/<pkg>
\`\`\`

Requires Go 1.25+.

## Contract

\`\`\`go
type <Interface> interface {
    <Method>(ctx context.Context, ...) (..., error)
    // ...
}
\`\`\`

- `<Method>` <one-line behavior + error contract>.

## Quick start

### Standalone

\`\`\`go
// minimal working example
\`\`\`

### Managed (Sentinel / hosted)

\`\`\`go
// production-flavored example
\`\`\`

### Wrap an existing resource

\`\`\`go
// for tests / custom pools
\`\`\`

## Usage

### <Pattern 1>

\`\`\`go
// real, copy-pastable
\`\`\`

### <Pattern 2>

\`\`\`go
// ...
\`\`\`

## Options

| Option | Default | Purpose |
|---|---|---|
| `<pkg>.With<Foo>(...)` | <default> | <one-line> |

## Errors

| Error | Meaning |
|---|---|
| `<pkg>.Err<Foo>` | <one-line> |

## Semantics & gotchas

- <invariant or surprising behavior>
- <TTL / retry / cancellation rule>
- <failure-mode behavior>

## Testing

\`\`\`bash
go test ./<pkg>/... -race -cover
\`\`\`

<one-line summary of what's covered>

## When *not* to use this

- <case>
- <case>
```

## Reviewing a README

A maintainer reviewing the README change should ask, in order:

1. Does the title section say what this is in two sentences?
2. Can a reader find the install line within one screen of scrolling?
3. Does Quick start actually compile against the current API?
4. Does every public option appear in the Options table?
5. Does every public error sentinel appear in the Errors table?
6. Is there at least one gotcha that an engineer would only learn from an
   incident?
7. Does "When *not* to use this" name a real alternative?
8. Are there any sentences that could be deleted without losing
   information?

If any answer is "no," request changes.

## Examples

The current best references in this repo are
[`cache/README.md`](../cache/README.md),
[`dlock/README.md`](../dlock/README.md), and
[`idempotency/README.md`](../idempotency/README.md). When in doubt, mirror
their structure and tone exactly.
