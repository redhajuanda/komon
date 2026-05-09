package idempotency_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/redhajuanda/komon/idempotency"
)

func newStore(t *testing.T, opts ...idempotency.Option) (*miniredis.Miniredis, *idempotency.Store) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return mr, idempotency.NewRedisClient(cli, opts...)
}

func TestTryClaim_FirstWinsSecondSkips(t *testing.T) {
	_, s := newStore(t)
	defer s.Close()
	ctx := context.Background()

	got, err := s.TryClaim(ctx, "orders", "msg-1", time.Minute)
	if err != nil || !got {
		t.Fatalf("first claim: claimed=%v err=%v", got, err)
	}
	got, err = s.TryClaim(ctx, "orders", "msg-1", time.Minute)
	if err != nil || got {
		t.Fatalf("second claim: claimed=%v err=%v (want false,nil)", got, err)
	}
}

func TestTryClaim_DifferentTopicsIsolated(t *testing.T) {
	_, s := newStore(t)
	defer s.Close()
	ctx := context.Background()

	if ok, _ := s.TryClaim(ctx, "orders", "msg-1", time.Minute); !ok {
		t.Fatal("orders/msg-1 should claim")
	}
	if ok, _ := s.TryClaim(ctx, "shipments", "msg-1", time.Minute); !ok {
		t.Fatal("shipments/msg-1 should claim independently")
	}
}

func TestTryClaim_ReclaimableAfterTTL(t *testing.T) {
	mr, s := newStore(t)
	defer s.Close()
	ctx := context.Background()

	if ok, _ := s.TryClaim(ctx, "t", "m", 100*time.Millisecond); !ok {
		t.Fatal("first claim")
	}
	mr.FastForward(200 * time.Millisecond)
	if ok, _ := s.TryClaim(ctx, "t", "m", time.Minute); !ok {
		t.Fatal("post-TTL claim should succeed")
	}
}

func TestTryClaim_ConcurrentExactlyOneWinner(t *testing.T) {
	mr, _ := newStore(t)

	const N = 64
	var wg sync.WaitGroup
	var wins atomic.Int64
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer cli.Close()
			s := idempotency.NewRedisClient(cli)
			defer s.Close()
			<-start
			ok, err := s.TryClaim(context.Background(), "topic", "id", time.Minute)
			if err != nil {
				t.Errorf("err: %v", err)
				return
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := wins.Load(); got != 1 {
		t.Fatalf("want exactly 1 winner, got %d", got)
	}
}

func TestTryClaim_RejectsNonPositiveTTL(t *testing.T) {
	_, s := newStore(t)
	defer s.Close()
	if _, err := s.TryClaim(context.Background(), "t", "m", 0); err == nil {
		t.Fatal("ttl=0 should error")
	}
	if _, err := s.TryClaim(context.Background(), "t", "m", -time.Second); err == nil {
		t.Fatal("ttl<0 should error")
	}
}

func TestTryClaim_AfterCloseFails(t *testing.T) {
	_, s := newStore(t)
	_ = s.Close()
	_, err := s.TryClaim(context.Background(), "t", "m", time.Minute)
	if !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	_, s := newStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestKeyLayout_DefaultPrefix(t *testing.T) {
	mr, s := newStore(t)
	defer s.Close()
	if _, err := s.TryClaim(context.Background(), "orders", "abc", time.Minute); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("idem:orders:abc") {
		t.Fatal("expected key idem:orders:abc")
	}
}

func TestKeyLayout_CustomPrefix(t *testing.T) {
	mr, s := newStore(t, idempotency.WithKeyPrefix("svc:dedupe:"))
	defer s.Close()
	if _, err := s.TryClaim(context.Background(), "orders", "abc", time.Minute); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("svc:dedupe:orders:abc") {
		t.Fatal("expected custom-prefixed key")
	}
}

func TestKeyLayout_CustomBuilder(t *testing.T) {
	mr, s := newStore(t, idempotency.WithKeyBuilder(func(topic, id string) string {
		return "k|" + topic + "|" + id
	}))
	defer s.Close()
	if _, err := s.TryClaim(context.Background(), "orders", "abc", time.Minute); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("k|orders|abc") {
		t.Fatal("custom builder key not used")
	}
}

func TestTTL_AppliedToKey(t *testing.T) {
	mr, s := newStore(t)
	defer s.Close()
	if _, err := s.TryClaim(context.Background(), "t", "m", 30*time.Second); err != nil {
		t.Fatal(err)
	}
	ttl := mr.TTL("idem:t:m")
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("unexpected ttl: %v", ttl)
	}
}
