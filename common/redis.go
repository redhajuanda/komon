package common

import (
	"context"
	"errors"
	"net"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// RedisOption is the shared configuration for constructing a Redis client —
// either Sentinel-backed (Sentinel=true) or a standalone single-node client
// (Sentinel=false). komon's redis-backed packages (cache, dlock,
// idempotency) embed this struct in their own RedisOption so dial logic has
// a single source of truth.
type RedisOption struct {
	// Sentinel selects the topology. true = Sentinel failover client (Hosts
	// is the list of sentinel addresses, MasterName is required).
	// false = standalone client against Hosts[0].
	Sentinel bool

	// Hosts is the list of sentinel host:port pairs when Sentinel=true,
	// or a single-element list with the redis host:port when Sentinel=false.
	Hosts []string

	// MasterName names the monitored master in Sentinel mode. Ignored when
	// Sentinel=false.
	MasterName string

	Username   string
	Password   string
	DB         int
	PoolSize   int
	MinIdleCon int

	// SentinelMasterDialOverride routes non-sentinel TCP dials to this
	// host:port (local dev / Docker Desktop scenarios where Sentinels return
	// addresses not dialable from the host). Ignored when Sentinel=false.
	SentinelMasterDialOverride string
}

// NewRedisClient builds a Redis client (Sentinel failover or standalone
// based on opt.Sentinel), pings it using the supplied context (so callers
// can bound startup time), and registers OpenTelemetry tracing + metrics.
// On any failure the client is closed before returning the error.
func NewRedisClient(ctx context.Context, opt RedisOption) (*redis.Client, error) {
	if len(opt.Hosts) == 0 {
		return nil, errors.New("common: RedisOption.Hosts is required")
	}

	var client *redis.Client
	if opt.Sentinel {
		if opt.MasterName == "" {
			return nil, errors.New("common: RedisOption.MasterName is required when Sentinel=true")
		}
		fo := &redis.FailoverOptions{
			SentinelAddrs: opt.Hosts,
			MasterName:    opt.MasterName,
			Username:      opt.Username,
			Password:      opt.Password,
			DB:            opt.DB,
			PoolSize:      opt.PoolSize,
			MinIdleConns:  opt.MinIdleCon,
		}
		if d := SentinelMasterDialOverride(opt.SentinelMasterDialOverride, opt.Hosts); d != nil {
			fo.Dialer = d
		}
		client = redis.NewFailoverClient(fo)
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:         opt.Hosts[0],
			Username:     opt.Username,
			Password:     opt.Password,
			DB:           opt.DB,
			PoolSize:     opt.PoolSize,
			MinIdleConns: opt.MinIdleCon,
		})
	}

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := redisotel.InstrumentTracing(client); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// SentinelMasterDialOverride returns a Dialer for redis.FailoverOptions when override is non-empty.
// Any dial whose addr is not in sentinelSeedAddrs is routed to override (typically the
// host-published Redis port, e.g. 127.0.0.1:6380). Sentinel seed addrs must match the
// strings go-redis uses (same host:port as in SentinelAddrs).
func SentinelMasterDialOverride(override string, sentinelSeedAddrs []string) func(context.Context, string, string) (net.Conn, error) {
	if override == "" {
		return nil
	}
	seeds := make(map[string]struct{}, len(sentinelSeedAddrs))
	for _, a := range sentinelSeedAddrs {
		seeds[a] = struct{}{}
	}
	d := net.Dialer{}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := seeds[addr]; ok {
			return d.DialContext(ctx, network, addr)
		}
		return d.DialContext(ctx, network, override)
	}
}
