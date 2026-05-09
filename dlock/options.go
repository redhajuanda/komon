package dlock

import "time"

const (
	defaultTries       = 32
	defaultRetryDelay  = 200 * time.Millisecond
	defaultDriftFactor = 0.01
	defaultKeyPrefix   = "dlock:"
)

// Options configure a DLocker.
type Options struct {
	// Tries bounds how many acquisition attempts Lock makes before giving
	// up with ErrNotAcquired. TryLock always uses 1.
	Tries int

	// RetryDelay is the base sleep between Lock retries. Redsync adds its
	// own jitter on top.
	RetryDelay time.Duration

	// DriftFactor is the fraction of TTL subtracted from the validity
	// window to account for clock drift between client and Redis.
	DriftFactor float64

	// KeyPrefix is prepended to every id before it reaches Redis.
	KeyPrefix string
}

// Option mutates Options.
type Option func(*Options)

// WithTries sets the Lock retry budget. Values < 1 are clamped to 1.
func WithTries(n int) Option {
	return func(o *Options) {
		if n < 1 {
			n = 1
		}
		o.Tries = n
	}
}

// WithRetryDelay sets the base sleep between Lock retries.
func WithRetryDelay(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.RetryDelay = d
		}
	}
}

// WithDriftFactor overrides the clock-drift compensation factor.
func WithDriftFactor(f float64) Option {
	return func(o *Options) {
		if f > 0 {
			o.DriftFactor = f
		}
	}
}

// WithKeyPrefix overrides the Redis key prefix.
func WithKeyPrefix(p string) Option {
	return func(o *Options) {
		o.KeyPrefix = p
	}
}

func defaultOptions() Options {
	return Options{
		Tries:       defaultTries,
		RetryDelay:  defaultRetryDelay,
		DriftFactor: defaultDriftFactor,
		KeyPrefix:   defaultKeyPrefix,
	}
}

func applyOptions(opts []Option) Options {
	o := defaultOptions()
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}
