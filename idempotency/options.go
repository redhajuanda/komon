package idempotency

const defaultKeyPrefix = "idem:"

// Options configure the idempotency store.
type Options struct {
	// KeyPrefix is prepended to every redis key. Default "idem:".
	KeyPrefix string

	// KeyBuilder, when non-nil, overrides the default
	// "<prefix><topic>:<messageID>" layout. Use this when integrating with an
	// existing key namespace.
	KeyBuilder func(topic, messageID string) string
}

// Option mutates Options.
type Option func(*Options)

// WithKeyPrefix overrides the default "idem:" prefix.
func WithKeyPrefix(p string) Option {
	return func(o *Options) { o.KeyPrefix = p }
}

// WithKeyBuilder installs a custom key layout. The returned key is used
// verbatim (no prefix is added) so the builder is responsible for any
// namespacing.
func WithKeyBuilder(fn func(topic, messageID string) string) Option {
	return func(o *Options) { o.KeyBuilder = fn }
}

func defaultOptions() Options {
	return Options{KeyPrefix: defaultKeyPrefix}
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
