package cache

import (
	"context"
)

type Cache interface {
	Close() error

	// Basic operations
	Get(ctx context.Context, key string, dest any, opts ...Option) error
	GetString(ctx context.Context, key string) (string, error)
	GetInt(ctx context.Context, key string) (int64, error)
	Set(ctx context.Context, key string, value any, opts ...Option) error
	Del(ctx context.Context, keys ...string) error
	// Exists reports whether at least one of the given keys exists.
	Exists(ctx context.Context, keys ...string) (bool, error)

	// Batch operations
	MGet(ctx context.Context, keys []string, dest any, opts ...Option) error
	SetMultiple(ctx context.Context, data []*DataSet, opts ...Option) error

	// Hash operations
	HSet(ctx context.Context, key string, data []*DataSet, opts ...Option) error
	HGet(ctx context.Context, key string, field string, dest any, opts ...Option) error
	HGetAll(ctx context.Context, key string, dest any, opts ...Option) error
	HMGetPipelined(ctx context.Context, requests map[string]string, opts ...Option) (map[string][]byte, error)
	HMSetPipelined(ctx context.Context, writes map[string][]*DataSet, opts ...Option) error
	UnmarshalValue(raw []byte, dest any, opts ...Option) error

	// Member/Set operations
	SetMember(ctx context.Context, key string, data []*DataSet, opts ...Option) error
	GetMember(ctx context.Context, key string, dest any, opts ...Option) (int, error)

	// Pattern deletion
	DeleteWithPattern(ctx context.Context, pattern string) error

	// Atomic operations.
	// Increment uses a sliding TTL (reset on every call); IncrementFixed
	// sets the TTL only on first creation.
	Increment(ctx context.Context, key string, opts ...Option) (int64, error)
	IncrementFixed(ctx context.Context, key string, opts ...Option) (int64, error)
}
