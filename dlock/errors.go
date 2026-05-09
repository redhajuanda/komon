package dlock

import "errors"

var (
	// ErrNotAcquired is returned when the lock cannot be acquired (already
	// held by another holder, or the retry budget was exhausted).
	ErrNotAcquired = errors.New("dlock: lock not acquired")

	// ErrLockNotHeld is returned by Unlock when this DLocker instance does
	// not hold a lock for the given id.
	ErrLockNotHeld = errors.New("dlock: lock not held by this instance")

	// ErrLockExpired is returned by Unlock when the lock's TTL elapsed
	// before the release reached Redis.
	ErrLockExpired = errors.New("dlock: lock expired before unlock")

	// ErrClosed is returned when the locker has been closed.
	ErrClosed = errors.New("dlock: locker is closed")
)
