package common_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/redhajuanda/komon/common"
)

func TestNewRedisClient_Standalone(t *testing.T) {
	mr := miniredis.RunT(t)

	cli, err := common.NewRedisClient(context.Background(), common.RedisOption{
		Sentinel: false,
		Hosts:    []string{mr.Addr()},
	})
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	defer cli.Close()

	if err := cli.Set(context.Background(), "k", "v", time.Minute).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := cli.Get(context.Background(), "k").Result()
	if err != nil || got != "v" {
		t.Fatalf("Get: got=%q err=%v", got, err)
	}
}

func TestNewRedisClient_RejectsEmptyHosts(t *testing.T) {
	_, err := common.NewRedisClient(context.Background(), common.RedisOption{})
	if err == nil {
		t.Fatal("expected error when Hosts is empty")
	}
}

func TestNewRedisClient_SentinelRequiresMasterName(t *testing.T) {
	_, err := common.NewRedisClient(context.Background(), common.RedisOption{
		Sentinel: true,
		Hosts:    []string{"127.0.0.1:26379"},
	})
	if err == nil {
		t.Fatal("expected error when Sentinel=true but MasterName empty")
	}
}

func TestNewRedisClient_StandalonePropagatesPingError(t *testing.T) {
	// Point at a closed port; ping must fail and the helper must return
	// the error (not panic / leak the client).
	_, err := common.NewRedisClient(context.Background(), common.RedisOption{
		Sentinel: false,
		Hosts:    []string{"127.0.0.1:1"}, // unlikely to be open
	})
	if err == nil {
		t.Fatal("expected ping error against closed port")
	}
}

func TestNewRedisClient_ContextCancelAbortsPing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := common.NewRedisClient(ctx, common.RedisOption{
		Sentinel: false,
		Hosts:    []string{"127.0.0.1:1"},
	})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
