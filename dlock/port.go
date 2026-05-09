// Package dlock provides a Redis-backed distributed lock built on top of the
// Redlock algorithm via go-redsync/redsync.
//
// The DLocker contract intentionally hides the lock token from callers; the
// implementation tracks per-id state internally so that Unlock(id) can release
// the correct fencing token. A single DLocker instance is therefore the
// single holder of a given id within its process; use distinct ids (or distinct
// instances) for re-entrant patterns.
package dlock

import (
	"context"
	"time"
)

// DLocker is a distributed lock with per-id state.
type DLocker interface {
	// TryLock attempts to acquire the lock for id once. It returns
	// ErrNotAcquired immediately if another holder owns the lock.
	TryLock(ctx context.Context, id string, ttl time.Duration) error

	// Lock blocks until the lock for id is acquired, ctx is done, or the
	// configured retry budget is exhausted. Returns ctx.Err() on cancellation
	// and ErrNotAcquired when retries are exhausted.
	Lock(ctx context.Context, id string, ttl time.Duration) error

	// Unlock releases a lock previously acquired by this instance for id.
	// Returns ErrLockNotHeld if this instance does not hold id, and
	// ErrLockExpired if the lock expired before the unlock reached Redis.
	Unlock(ctx context.Context, id string) error

	// Close releases any resources owned by the locker. It is safe to call
	// multiple times.
	Close() error
}
