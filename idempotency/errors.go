package idempotency

import "errors"

// ErrClosed is returned when TryClaim is called on a closed store.
var ErrClosed = errors.New("idempotency: store is closed")
