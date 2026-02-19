package redis

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/extra/redisotel"
	redis "github.com/go-redis/redis/v8"
	cache "github.com/redhajuanda/komon/cache"
)

const (
	defaultDB          = 0
	schema             = "redis"
	schemaRedisCluster = "redis-cluster"

	// refer to this wiki https://github.com/lettuce-io/lettuce-core/wiki/Redis-URI-and-connection-details
	schemaRedisSentinel = "redis-sentinel"
	defaultPoolSize     = 100
	defaultMinIdleCon   = 50

	queryMinIdleConns = "minIdleConns"
	queryPoolSize     = "poolSize"
	queryMasterName   = "master"

	queryDialTimeout  = "DialTimeout"
	queryReadTimeout  = "ReadTimeout"
	queryWriteTimeout = "WriteTimeout"
)

type ClientAble interface {
	redis.Cmdable
	io.Closer
}

// Cache redis cache object
type Cache struct {
	client ClientAble
	ns     string
}

func init() {
	cache.Register(schema, NewCache)
	cache.Register(schemaRedisCluster, NewCacheCluster)
	cache.Register(schemaRedisSentinel, NewCacheSentinel)
}

// NewCache create new redis cache
func NewCache(url *url.URL) (cache.Cache, error) {
	p, _ := url.User.Password()
	opt := &redis.Options{
		Addr:     url.Host,
		Password: p,
		DB:       defaultDB, // use default DB
	}

	if ts := url.Query().Get("tls"); ts != "" {
		opt.TLSConfig = &tls.Config{
			ServerName: ts,
		}
	}

	db := strings.TrimPrefix(url.Path, "/")
	if db != "" {
		i, err := strconv.Atoi(db)
		if err != nil {
			opt.DB = defaultDB
		}
		opt.DB = i
	}

	if ps := url.Query().Get(queryPoolSize); ps != "" {
		i, err := strconv.Atoi(ps)
		if err != nil {
			opt.PoolSize = defaultPoolSize
		}
		opt.PoolSize = i
	}

	if mic := url.Query().Get(queryMinIdleConns); mic != "" {
		i, err := strconv.Atoi(mic)
		if err != nil {
			opt.MinIdleConns = defaultMinIdleCon
		}
		opt.MinIdleConns = i
	}

	rClient := redis.NewClient(opt)
	rClient.AddHook(redisotel.TracingHook{})

	prefix := url.Query().Get("prefix")

	cache := &Cache{
		client: rClient,
		ns:     prefix,
	}
	_, err := cache.client.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	return cache, nil
}

func NewCacheCluster(url *url.URL) (cache.Cache, error) {
	address := strings.Split(url.Host, ",")
	if len(address) < 1 {
		return nil, errors.New("invalid address")
	}

	p, _ := url.User.Password()

	opts := &redis.ClusterOptions{
		Addrs:    address,
		Username: url.User.Username(),
		Password: p}

	if ps := url.Query().Get(queryPoolSize); ps != "" {
		i, err := strconv.Atoi(ps)
		if err != nil {
			opts.PoolSize = defaultPoolSize
		}
		opts.PoolSize = i
	}

	if mic := url.Query().Get(queryMinIdleConns); mic != "" {
		i, err := strconv.Atoi(mic)
		if err != nil {
			opts.MinIdleConns = defaultMinIdleCon
		}
		opts.MinIdleConns = i
	}

	if ts := url.Query().Get("tls"); ts != "" {
		opts.TLSConfig = &tls.Config{
			ServerName: ts,
		}
	}

	rClient := redis.NewClusterClient(opts)
	rClient.AddHook(redisotel.TracingHook{})

	cache := &Cache{
		client: rClient,
		ns:     url.Query().Get("prefix"),
	}
	_, err := cache.client.Ping(context.Background()).Result()
	return cache, err
}

func NewCacheSentinel(url *url.URL) (cache.Cache, error) {
	address := strings.Split(url.Host, ",")
	if len(address) < 1 {
		return nil, errors.New("invalid address")
	}

	p, _ := url.User.Password()

	masterName := url.Query().Get(queryMasterName)
	if masterName == "" {
		// default master name
		masterName = "mymaster"
	}

	opts := &redis.FailoverOptions{
		MasterName:    masterName,
		SentinelAddrs: address,
		Username:      url.User.Username(),
		Password:      p,
	}

	db := strings.TrimPrefix(url.Path, "/")
	if db != "" {
		i, err := strconv.Atoi(db)
		if err != nil {
			opts.DB = defaultDB
		}
		opts.DB = i
	}

	if ps := url.Query().Get(queryPoolSize); ps != "" {
		i, err := strconv.Atoi(ps)
		if err != nil {
			opts.PoolSize = defaultPoolSize
		}
		opts.PoolSize = i
	}

	if mic := url.Query().Get(queryMinIdleConns); mic != "" {
		i, err := strconv.Atoi(mic)
		if err != nil {
			opts.MinIdleConns = defaultMinIdleCon
		}
		opts.MinIdleConns = i
	}

	if ts := url.Query().Get("tls"); ts != "" {
		opts.TLSConfig = &tls.Config{
			ServerName: ts,
		}
	}

	if dt := url.Query().Get(queryDialTimeout); dt != "" {
		i, err := strconv.Atoi(dt)
		if err == nil {
			opts.DialTimeout = time.Duration(i)
		}
	}

	if rt := url.Query().Get(queryReadTimeout); rt != "" {
		i, err := strconv.Atoi(rt)
		if err == nil {
			opts.ReadTimeout = time.Duration(i)
		}
	}

	if wt := url.Query().Get(queryWriteTimeout); wt != "" {
		i, err := strconv.Atoi(wt)
		if err == nil {
			opts.WriteTimeout = time.Duration(i)
		}
	}

	rClient := redis.NewFailoverClient(opts)
	rClient.AddHook(redisotel.TracingHook{})

	cache := &Cache{
		client: rClient,
		ns:     url.Query().Get("prefix"),
	}
	_, err := cache.client.Ping(context.Background()).Result()
	return cache, err
}

// NewRedisCache creating instance of redis cache
func NewRedisCache(ns string, option ...Option) (*Cache, error) {
	r := &redis.Options{}
	for _, o := range option {
		o(r)
	}
	rClient := redis.NewClient(r)
	rClient.AddHook(redisotel.TracingHook{})
	cache := &Cache{
		client: rClient,
		ns:     ns,
	}
	_, err := cache.client.Ping(context.Background()).Result()
	return cache, err
}

func NewRedisCluster(ns string, addresses []string, option ...ClusterOption) (*Cache, error) {
	r := &redis.ClusterOptions{}
	for _, o := range option {
		o(r)
	}
	rClient := redis.NewClusterClient(r)
	rClient.AddHook(redisotel.TracingHook{})

	cache := &Cache{
		client: rClient,
		ns:     ns,
	}
	_, err := cache.client.Ping(context.Background()).Result()
	return cache, err
}

func NewRedisSentinel(ns string, addresses []string, option ...SentinelOption) (*Cache, error) {
	r := &redis.FailoverOptions{}
	for _, o := range option {
		o(r)
	}
	rClient := redis.NewFailoverClient(r)
	rClient.AddHook(redisotel.TracingHook{})

	cache := &Cache{
		client: rClient,
		ns:     ns,
	}
	_, err := cache.client.Ping(context.Background()).Result()
	return cache, err
}

type Option func(options *redis.Options)

type ClusterOption func(options *redis.ClusterOptions)

type SentinelOption func(options *redis.FailoverOptions)

func DefaultAddressOption(addresses []string, password string) ClusterOption {
	return func(options *redis.ClusterOptions) {
		options.Password = password
		options.Addrs = addresses
	}
}

func ClusterTLSOption(address string) ClusterOption {
	return func(options *redis.ClusterOptions) {
		options.TLSConfig = &tls.Config{
			ServerName: address,
		}
	}
}

func DefaultOption(address, password string) Option {
	return func(options *redis.Options) {
		options.Password = password
		options.Addr = address
	}
}

func TLSOption(address string) Option {
	return func(options *redis.Options) {
		options.TLSConfig = &tls.Config{
			ServerName: address,
		}
	}
}

// Set set value
func (c *Cache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	switch value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, []byte:
		return c.client.Set(ctx, c.ns+key, value, expiration).Err()
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return c.client.Set(ctx, c.ns+key, b, expiration).Err()
	}
}

// Increment increment int value
func (c *Cache) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	switch expiration {
	case 0:
		i, err := c.client.Incr(ctx, key).Result()
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		pipe := c.client.TxPipeline()

		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, expiration)

		_, err := pipe.Exec(ctx)
		if err != nil {
			return 0, err
		}
		return incr.Val(), nil
	}
}

// Get get value
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := c.client.Get(ctx, c.ns+key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, cache.NotFound
		}
		return nil, err
	}
	return b, nil
}

// GetObject get object value
func (c *Cache) GetObject(ctx context.Context, key string, doc interface{}) error {
	b, err := c.client.Get(ctx, c.ns+key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return cache.NotFound
		}
		return err
	}
	return json.Unmarshal(b, doc)
}

// GetString get string value
func (c *Cache) GetString(ctx context.Context, key string) (string, error) {
	b, err := c.Get(ctx, key)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// GetInt get int value
func (c *Cache) GetInt(ctx context.Context, key string) (int64, error) {
	b, err := c.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	i, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}

// GetFloat get float value
func (c *Cache) GetFloat(ctx context.Context, key string) (float64, error) {
	b, err := c.Get(ctx, key)
	if err != nil {
		return 0, err
	}

	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return 0, err
	}

	return f, nil
}

// Exist check if key exist
func (c *Cache) Exist(ctx context.Context, key string) bool {
	return c.client.Exists(ctx, c.ns+key).Val() > 0
}

// Delete delete record
func (c *Cache) Delete(ctx context.Context, key string, opts ...cache.DeleteOptions) error {
	deleteCache := &cache.DeleteCache{}
	for _, opt := range opts {
		opt(deleteCache)
	}

	if deleteCache.Pattern != "" {
		return c.deletePattern(ctx, deleteCache.Pattern)
	}

	return c.client.Del(ctx, c.ns+key).Err()
}

func (c *Cache) GetKeys(ctx context.Context, pattern string) []string {
	cmd := c.client.Keys(ctx, pattern)
	keys, err := cmd.Result()
	if err != nil {
		return nil
	}
	return keys
}

// deletePattern delete record by pattern
func (c *Cache) deletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, c.ns+pattern, 0).Iterator()
	var localKeys []string

	for iter.Next(ctx) {
		localKeys = append(localKeys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return err
	}

	if len(localKeys) > 0 {
		_, err := c.client.Pipelined(ctx, func(pipeline redis.Pipeliner) error {
			pipeline.Del(ctx, localKeys...)
			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}

// RemainingTime get remaining time
func (c *Cache) RemainingTime(ctx context.Context, key string) int {
	return int(c.client.TTL(ctx, c.ns+key).Val().Seconds())
}

// Close close connection
func (c *Cache) Close() error {
	return c.client.Close()
}
