// Package idempotency provides a Redis-backed claim-once primitive for
// deduplicating message processing across consumers.
//
// A claim is a single SET NX PX against a topic-scoped key. The first caller
// to claim wins and should process the message; subsequent callers should
// skip (and typically ACK) the duplicate. Keys expire after the configured
// TTL to bound storage growth — choose a TTL longer than the maximum
// reasonable redelivery window for your broker.
package idempotency

import (
	"context"
	"time"
)

// Idempotency is the deduplication contract.
type Idempotency interface {
	// TryClaim atomically claims the idempotency key for the given topic and
	// message ID. Returns true if claimed (caller should process), false if
	// already processed (caller should skip/ACK). Keys expire after ttl to
	// limit storage growth.
	TryClaim(ctx context.Context, topic, messageID string, ttl time.Duration) (claimed bool, err error)
}
