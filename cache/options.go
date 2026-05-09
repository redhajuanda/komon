package cache

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type (
	Serialize int

	Options struct {
		expire    time.Duration
		serialize Serialize
	}

	Option func(*Options)
)

const (
	SerializeJSON Serialize = iota
	SerializeMessagePack
)

const (
	DefaultExpire        = 24 * time.Hour
	DefaultJitterPercent = 0.10
)

// TTL Classification based on table size and row count.
// Very Large (>500K rows): 1-2 hours
// Large (50K-500K rows): 4-6 hours
// Medium (5K-50K rows): 8-12 hours
// Small (<5K rows): use DefaultExpire (24 hours) - no need to specify TTL
const (
	TTLVeryLarge = 2 * time.Hour
	TTLLarge     = 6 * time.Hour
	TTLMedium    = 12 * time.Hour
)

func (r Serialize) Valid() bool {
	return r >= SerializeJSON && r <= SerializeMessagePack
}

func (r Serialize) Marshal(v any) ([]byte, error) {
	switch r {
	case SerializeMessagePack:
		return msgpack.Marshal(v)
	default:
		return json.Marshal(v)
	}
}

func (r Serialize) Unmarshal(data []byte, v any) error {
	switch r {
	case SerializeMessagePack:
		return msgpack.Unmarshal(data, v)
	default:
		return json.Unmarshal(data, v)
	}
}

func DefaultOptions() *Options {
	return &Options{
		serialize: SerializeJSON,
		expire:    DefaultExpire,
	}
}

// WithTTL sets cache expiration duration.
func WithTTL(ttl time.Duration) Option {
	return func(o *Options) {
		if ttl > 0 {
			o.expire = ttl
		}
	}
}

// WithTTLJitter sets cache expiration with random jitter (default ±10%).
// Jitter helps prevent cache stampede by spreading expiration times.
func WithTTLJitter(ttl time.Duration, jitterPercent ...float64) Option {
	return func(o *Options) {
		if ttl <= 0 {
			return
		}

		percent := DefaultJitterPercent
		if len(jitterPercent) > 0 && jitterPercent[0] > 0 {
			percent = jitterPercent[0]
		}

		jitter := float64(ttl) * percent
		minDuration := float64(ttl) - jitter
		maxDuration := float64(ttl) + jitter
		randomDuration := minDuration + rand.Float64()*(maxDuration-minDuration)

		o.expire = time.Duration(randomDuration)
	}
}

// WithMsgPack sets serialization to MessagePack format.
// MessagePack is more compact and faster than JSON.
func WithMsgPack() Option {
	return func(o *Options) {
		o.serialize = SerializeMessagePack
	}
}

// WithJSON sets serialization to JSON format (default).
func WithJSON() Option {
	return func(o *Options) {
		o.serialize = SerializeJSON
	}
}

func (o *Options) GetSerialize() Serialize {
	return o.serialize
}

// GetExpire returns the configured TTL, or DefaultExpire when unset/non-positive.
// Guards against zero-value Options{} or callers that bypass WithTTL guard,
// preventing keys from being written without expiration.
func (o *Options) GetExpire() time.Duration {
	if o.expire <= 0 {
		return DefaultExpire
	}
	return o.expire
}

// ApplyJitter applies random jitter (±10%) to a duration.
// Used internally to spread expiration times and prevent cache stampede.
func ApplyJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	jitter := float64(d) * DefaultJitterPercent
	minDuration := float64(d) - jitter
	maxDuration := float64(d) + jitter
	return time.Duration(minDuration + rand.Float64()*(maxDuration-minDuration))
}

// HighTrafficOpts returns pre-defined options for high-traffic APIs.
// Use these to ensure Get and Set use the same serialization format.
//
// Usage:
//
//	cache.Get(ctx, key, &result, cache.HighTrafficOpts(cache.TTLMedium)...)
//	cache.Set(ctx, key, value, cache.HighTrafficOpts(cache.TTLMedium)...)
func HighTrafficOpts(ttl time.Duration) []Option {
	return []Option{WithTTLJitter(ttl), WithMsgPack()}
}
