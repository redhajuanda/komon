package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/redhajuanda/komon/common"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const (
	scanCountHint  = 100
	deleteKeyBatch = 100
)

type (
	// RedisOption configures a Redis-backed cache. The shared dial/ping/OTel
	// concerns (and the Sentinel-vs-standalone toggle) live in
	// common.RedisOption; this type adds only the cache-specific knobs.
	RedisOption struct {
		common.RedisOption
		// UseMsgPack selects MessagePack as the default serialization
		// (recommended for high-traffic).
		UseMsgPack bool
	}

	Redis struct {
		*redis.Client
		options *Options
	}

	DataSet struct {
		Key   string
		Value any
	}
)

// NewRedisClient wraps a standalone redis.Client (for example miniredis in tests
// or a direct TCP node). It does not register OpenTelemetry instrumentation —
// callers running against a real cluster should call InstrumentOTel after
// construction (or use NewRedisSentinel which does it automatically).
func NewRedisClient(client *redis.Client, useMsgPack bool) *Redis {
	defaultOpts := DefaultOptions()
	if useMsgPack {
		WithMsgPack()(defaultOpts)
	}
	return &Redis{
		Client:  client,
		options: defaultOpts,
	}
}

// InstrumentOTel registers OpenTelemetry tracing and metrics on the wrapped
// client. Idempotency at the go-redis layer is not guaranteed; call once.
func InstrumentOTel(r *Redis) error {
	if err := redisotel.InstrumentTracing(r.Client); err != nil {
		return err
	}
	return redisotel.InstrumentMetrics(r.Client)
}

// NewRedis constructs a Redis-backed cache. opt.Sentinel selects between a
// Sentinel failover client (true) and a standalone client (false). The ping
// uses the supplied context so callers can bound startup time. OpenTelemetry
// tracing and metrics are registered automatically.
func NewRedis(ctx context.Context, opt RedisOption) (*Redis, error) {
	client, err := common.NewRedisClient(ctx, opt.RedisOption)
	if err != nil {
		return nil, err
	}

	defaultOpts := DefaultOptions()
	if opt.UseMsgPack {
		WithMsgPack()(defaultOpts)
	}

	return &Redis{
		Client:  client,
		options: defaultOpts,
	}, nil
}

// Close closes the underlying Redis client connection pool.
func (r *Redis) Close() error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Close()
}

func (r *Redis) applyOptions(opts ...Option) Options {
	opt := *r.options
	for _, fn := range opts {
		if fn != nil {
			fn(&opt)
		}
	}
	return opt
}

func (r *Redis) Get(ctx context.Context, key string, value any, opts ...Option) error {
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return err
	}

	opt := r.applyOptions(opts...)

	return opt.GetSerialize().Unmarshal(data, value)
}

func (r *Redis) GetInt(ctx context.Context, key string) (int64, error) {
	val, err := r.Client.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return val, nil
}

func (r *Redis) GetString(ctx context.Context, key string) (string, error) {
	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", err
	}
	return val, nil
}

func (r *Redis) MGet(ctx context.Context, keys []string, obj any, opts ...Option) error {
	if len(keys) == 0 {
		return nil
	}

	data, err := r.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return err
	}

	return decodeAlignedBulk(data, obj, r.applyOptions(opts...))
}

func bulkStringToBytes(v any) ([]byte, error) {
	switch x := v.(type) {
	case string:
		return []byte(x), nil
	case []byte:
		return x, nil
	default:
		return nil, fmt.Errorf("cache: unexpected redis bulk string type %T", v)
	}
}

// decodeAlignedBulk fills dest with one element per Redis value, preserving index alignment with MGET order.
// nil Redis values become the zero value ([]T) or nil entries ([]*T).
func decodeAlignedBulk(val []any, dest any, opt Options) error {
	if dest == nil {
		return ErrInvalidSliceDestination
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return ErrInvalidSliceDestination
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Slice {
		return ErrInvalidSliceDestination
	}

	typ := rv.Type().Elem()
	elemIsPtr := typ.Kind() == reflect.Ptr
	if elemIsPtr && typ.Elem().Kind() == reflect.Ptr {
		return ErrInvalidSliceDestination
	}
	var elemNonPtr reflect.Type
	if elemIsPtr {
		elemNonPtr = typ.Elem()
	} else {
		elemNonPtr = typ
	}

	out := reflect.MakeSlice(rv.Type(), len(val), len(val))
	for i, raw := range val {
		if raw == nil {
			out.Index(i).Set(reflect.Zero(typ))
			continue
		}
		b, err := bulkStringToBytes(raw)
		if err != nil {
			return err
		}
		if elemIsPtr {
			pv := reflect.New(elemNonPtr)
			if err := opt.GetSerialize().Unmarshal(b, pv.Interface()); err != nil {
				return err
			}
			out.Index(i).Set(pv)
		} else {
			pv := reflect.New(elemNonPtr)
			if err := opt.GetSerialize().Unmarshal(b, pv.Interface()); err != nil {
				return err
			}
			out.Index(i).Set(pv.Elem())
		}
	}
	rv.Set(out)
	return nil
}

func (r *Redis) GetMember(ctx context.Context, key string, obj any, opts ...Option) (int, error) {
	keys, err := r.Client.SMembers(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	totalMembers := len(keys)
	if totalMembers == 0 {
		return 0, ErrNotFound
	}

	return totalMembers, r.MGet(ctx, keys, obj, opts...)
}

// Set stores value under key with the configured (or default) TTL.
// Strings and []byte are written raw (so they can be read back by GetString /
// raw redis access). All other types — including primitives — are routed
// through the configured serializer so Get(ctx, key, &dest) can round-trip
// them. This keeps Set/Get symmetric regardless of the chosen serializer.
func (r *Redis) Set(ctx context.Context, key string, value any, opts ...Option) error {
	opt := r.applyOptions(opts...)

	switch value.(type) {
	case string, []byte:
		return r.Client.Set(ctx, key, value, opt.GetExpire()).Err()
	default:
		data, err := opt.GetSerialize().Marshal(value)
		if err != nil {
			return err
		}
		return r.Client.Set(ctx, key, data, opt.GetExpire()).Err()
	}
}

// SetMember adds cache key names (DataSet.Key) to a Redis set; DataSet.Value is ignored.
// Typical pattern: store related primary cache keys in the set, then resolve values with GetMember (SMEMBERS + MGET).
func (r *Redis) SetMember(ctx context.Context, key string, data []*DataSet, opts ...Option) error {
	opt := r.applyOptions(opts...)

	if len(data) == 0 {
		return nil
	}
	// Build []any so SAdd treats each entry as an individual member instead
	// of relying on go-redis's reflective slice-flattening.
	members := make([]any, 0, len(data))
	for _, v := range data {
		members = append(members, v.Key)
	}
	_, err := r.Client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SAdd(ctx, key, members...)
		pipe.Expire(ctx, key, ApplyJitter(opt.GetExpire()))
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Redis) deleteWithPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var keys []string
	for {
		k, next, err := r.Client.Scan(ctx, cursor, pattern, scanCountHint).Result()
		if err != nil {
			return err
		}
		keys = append(keys, k...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	for i := 0; i < len(keys); i += deleteKeyBatch {
		end := i + deleteKeyBatch
		if end > len(keys) {
			end = len(keys)
		}
		if err := r.Client.Del(ctx, keys[i:end]...).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Redis) DeleteWithPattern(ctx context.Context, pattern string) error {
	return r.deleteWithPattern(ctx, pattern)
}

// SetMultiple sets multiple cache entries with automatic jitter per-item.
// Jitter is applied to each item's TTL to prevent cache stampede when
// multiple entries are set simultaneously.
func (r *Redis) SetMultiple(ctx context.Context, data []*DataSet, opts ...Option) error {
	if len(data) == 0 {
		return nil
	}

	opt := r.applyOptions(opts...)

	_, err := r.Client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, v := range data {
			// Apply jitter per-item to spread expiration times
			itemExpire := ApplyJitter(opt.GetExpire())

			switch v.Value.(type) {
			case string, []byte:
				pipe.Set(ctx, v.Key, v.Value, itemExpire)
			default:
				vb, err := opt.GetSerialize().Marshal(v.Value)
				if err != nil {
					return err
				}
				pipe.Set(ctx, v.Key, vb, itemExpire)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Redis) HGetAll(ctx context.Context, key string, obj any, opts ...Option) error {
	data, err := r.Client.HGetAll(ctx, key).Result()
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return ErrNotFound
	}

	fields := make([]string, 0, len(data))
	for k := range data {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	allData := make([]any, 0, len(data))
	for _, k := range fields {
		allData = append(allData, data[k])
	}

	return decodeAlignedBulk(allData, obj, r.applyOptions(opts...))
}

// HMGetPipelined fetches one field per key across multiple hash keys in a single pipeline round trip.
// requests maps cacheKey → fieldName. Returns raw marshaled bytes for hits; misses are omitted.
// go-redis may report redis.Nil from Exec when some HGETs miss; that is treated as success and misses are omitted.
func (r *Redis) HMGetPipelined(ctx context.Context, requests map[string]string, _ ...Option) (map[string][]byte, error) {
	if len(requests) == 0 {
		return map[string][]byte{}, nil
	}

	type entry struct {
		key string
		cmd *redis.StringCmd
	}
	entries := make([]entry, 0, len(requests))

	_, err := r.Client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for key, field := range requests {
			cmd := pipe.HGet(ctx, key, field)
			entries = append(entries, entry{key: key, cmd: cmd})
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	result := make(map[string][]byte, len(entries))
	for _, e := range entries {
		data, err := e.cmd.Bytes()
		if err != nil {
			continue
		}
		result[e.key] = data
	}
	return result, nil
}

// HMSetPipelined sets hash fields and TTL across multiple keys in a single pipeline round trip.
// writes maps cacheKey → []*DataSet (fields to set on each key).
func (r *Redis) HMSetPipelined(ctx context.Context, writes map[string][]*DataSet, opts ...Option) error {
	if len(writes) == 0 {
		return nil
	}

	opt := r.applyOptions(opts...)

	_, err := r.Client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for key, datasets := range writes {
			if len(datasets) == 0 {
				continue
			}
			hashData := make(map[string]any, len(datasets))
			for _, v := range datasets {
				vb, err := opt.GetSerialize().Marshal(v.Value)
				if err != nil {
					return err
				}
				hashData[v.Key] = vb
			}
			pipe.HSet(ctx, key, hashData)
			pipe.Expire(ctx, key, ApplyJitter(opt.GetExpire()))
		}
		return nil
	})
	return err
}

// UnmarshalValue deserializes raw cache bytes into dest using the configured serializer.
func (r *Redis) UnmarshalValue(raw []byte, dest any, opts ...Option) error {
	opt := r.applyOptions(opts...)
	return opt.GetSerialize().Unmarshal(raw, dest)
}

// HGet retrieves a single field from a hash.
func (r *Redis) HGet(ctx context.Context, key string, field string, dest any, opts ...Option) error {
	opt := r.applyOptions(opts...)

	data, err := r.Client.HGet(ctx, key, field).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return err
	}

	return opt.GetSerialize().Unmarshal(data, dest)
}

// HSet sets hash fields with automatic jitter on expiration.
func (r *Redis) HSet(ctx context.Context, key string, data []*DataSet, opts ...Option) error {
	if len(data) == 0 {
		return nil
	}

	opt := r.applyOptions(opts...)

	hashData := make(map[string]any, len(data))
	for _, v := range data {
		vb, err := opt.GetSerialize().Marshal(v.Value)
		if err != nil {
			return err
		}
		hashData[v.Key] = vb
	}

	_, err := r.Client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key, hashData)
		pipe.Expire(ctx, key, ApplyJitter(opt.GetExpire()))
		return nil
	})

	return err
}

func (r *Redis) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.Client.Del(ctx, keys...).Err()
}

// Exists reports whether at least one of the given keys exists.
// A network/redis error is propagated so callers can distinguish
// "absent" from "couldn't tell".
func (r *Redis) Exists(ctx context.Context, keys ...string) (bool, error) {
	if len(keys) == 0 {
		return false, nil
	}
	n, err := r.Client.Exists(ctx, keys...).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Increment runs INCR + EXPIRE inside a Redis transaction (MULTI/EXEC) so the
// counter and TTL update together.
//
// IMPORTANT: This is a SLIDING window — the TTL is reset on every call. If
// traffic is continuous the key never expires. For fixed-window rate limiting
// (TTL set on first creation only), use IncrementFixed.
//
// TTL falls back to DefaultExpire when no options are provided or WithTTL(0)
// is passed.
func (r *Redis) Increment(ctx context.Context, key string, opts ...Option) (int64, error) {
	opt := r.applyOptions(opts...)
	pipe := r.Client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, opt.GetExpire())
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// IncrementFixed runs INCR and only sets TTL on first creation (EXPIRE NX,
// Redis 7.0+). Use this for fixed-window counters where the window must close
// regardless of traffic.
func (r *Redis) IncrementFixed(ctx context.Context, key string, opts ...Option) (int64, error) {
	opt := r.applyOptions(opts...)
	pipe := r.Client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, opt.GetExpire())
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
