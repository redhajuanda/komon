package dlock_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/redhajuanda/komon/dlock"
)

func newLocker(t *testing.T, opts ...dlock.Option) (*miniredis.Miniredis, dlock.DLocker) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return mr, dlock.NewRedisClient(cli, opts...)
}

// fastOpts shrinks retry timing so blocking-Lock tests do not waste seconds.
func fastOpts() []dlock.Option {
	return []dlock.Option{
		dlock.WithTries(50),
		dlock.WithRetryDelay(5 * time.Millisecond),
	}
}

func TestTryLock_AcquiresWhenFree(t *testing.T) {
	_, l := newLocker(t)
	defer l.Close()

	if err := l.TryLock(context.Background(), "job", time.Second); err != nil {
		t.Fatalf("TryLock free: %v", err)
	}
	if err := l.Unlock(context.Background(), "job"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestTryLock_FailsWhenHeld(t *testing.T) {
	mr, l1 := newLocker(t)
	defer l1.Close()
	cli2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli2.Close()
	l2 := dlock.NewRedisClient(cli2)
	defer l2.Close()

	if err := l1.TryLock(context.Background(), "job", time.Second); err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	err := l2.TryLock(context.Background(), "job", time.Second)
	if !errors.Is(err, dlock.ErrNotAcquired) {
		t.Fatalf("want ErrNotAcquired, got %v", err)
	}
}

func TestTryLock_SameInstanceTwiceFails(t *testing.T) {
	_, l := newLocker(t)
	defer l.Close()

	if err := l.TryLock(context.Background(), "job", time.Second); err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if err := l.TryLock(context.Background(), "job", time.Second); !errors.Is(err, dlock.ErrNotAcquired) {
		t.Fatalf("want ErrNotAcquired on re-entry, got %v", err)
	}
}

func TestLock_BlocksUntilReleased(t *testing.T) {
	mr, l1 := newLocker(t, fastOpts()...)
	defer l1.Close()
	cli2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli2.Close()
	l2 := dlock.NewRedisClient(cli2, fastOpts()...)
	defer l2.Close()

	if err := l1.TryLock(context.Background(), "job", 5*time.Second); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	acquired := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		acquired <- l2.Lock(ctx, "job", 500*time.Millisecond)
	}()

	// Give the blocked Lock time to enter its retry loop, then release.
	time.Sleep(50 * time.Millisecond)
	if err := l1.Unlock(context.Background(), "job"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("blocked Lock returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Lock did not acquire after release")
	}
	_ = l2.Unlock(context.Background(), "job")
}

func TestLock_RespectsContextCancel(t *testing.T) {
	mr, l1 := newLocker(t, fastOpts()...)
	defer l1.Close()
	cli2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli2.Close()
	l2 := dlock.NewRedisClient(cli2, fastOpts()...)
	defer l2.Close()

	if err := l1.TryLock(context.Background(), "job", 5*time.Second); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	defer l1.Unlock(context.Background(), "job")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- l2.Lock(ctx, "job", time.Second)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Lock did not return after ctx cancel")
	}
}

func TestUnlock_UnknownID(t *testing.T) {
	_, l := newLocker(t)
	defer l.Close()

	err := l.Unlock(context.Background(), "never-locked")
	if !errors.Is(err, dlock.ErrLockNotHeld) {
		t.Fatalf("want ErrLockNotHeld, got %v", err)
	}
}

func TestUnlock_AfterTTLExpired(t *testing.T) {
	mr, l := newLocker(t)
	defer l.Close()

	if err := l.TryLock(context.Background(), "job", 100*time.Millisecond); err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	mr.FastForward(200 * time.Millisecond)

	err := l.Unlock(context.Background(), "job")
	if !errors.Is(err, dlock.ErrLockExpired) {
		t.Fatalf("want ErrLockExpired, got %v", err)
	}
	// Subsequent Unlock should report not-held (state was cleared).
	if err := l.Unlock(context.Background(), "job"); !errors.Is(err, dlock.ErrLockNotHeld) {
		t.Fatalf("second Unlock: want ErrLockNotHeld, got %v", err)
	}
}

func TestTryLock_AfterTTLExpiresNaturally(t *testing.T) {
	mr, l1 := newLocker(t)
	defer l1.Close()
	cli2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli2.Close()
	l2 := dlock.NewRedisClient(cli2)
	defer l2.Close()

	if err := l1.TryLock(context.Background(), "job", 100*time.Millisecond); err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	mr.FastForward(200 * time.Millisecond)

	if err := l2.TryLock(context.Background(), "job", time.Second); err != nil {
		t.Fatalf("post-expiry TryLock: %v", err)
	}
	_ = l2.Unlock(context.Background(), "job")
}

func TestConcurrent_TryLock_OnlyOneWins(t *testing.T) {
	mr, _ := newLocker(t)

	const N = 32
	var attempts sync.WaitGroup
	var done sync.WaitGroup
	start := make(chan struct{})
	release := make(chan struct{})
	var wins atomic.Int64
	for i := 0; i < N; i++ {
		attempts.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer cli.Close()
			l := dlock.NewRedisClient(cli)
			defer l.Close()
			<-start
			err := l.TryLock(context.Background(), "shared", 5*time.Second)
			attempts.Done()
			switch {
			case err == nil:
				wins.Add(1)
				<-release
				_ = l.Unlock(context.Background(), "shared")
			case errors.Is(err, dlock.ErrNotAcquired):
			default:
				t.Errorf("unexpected err: %v", err)
			}
		}()
	}
	close(start)
	attempts.Wait()
	close(release)
	done.Wait()
	if got := wins.Load(); got != 1 {
		t.Fatalf("want exactly 1 winner, got %d", got)
	}
}

func TestDifferentIDsIndependent(t *testing.T) {
	_, l := newLocker(t)
	defer l.Close()

	if err := l.TryLock(context.Background(), "a", time.Second); err != nil {
		t.Fatalf("lock a: %v", err)
	}
	if err := l.TryLock(context.Background(), "b", time.Second); err != nil {
		t.Fatalf("lock b: %v", err)
	}
	if err := l.Unlock(context.Background(), "a"); err != nil {
		t.Fatalf("unlock a: %v", err)
	}
	if err := l.Unlock(context.Background(), "b"); err != nil {
		t.Fatalf("unlock b: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	_, l := newLocker(t)
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestOperationsAfterClose(t *testing.T) {
	_, l := newLocker(t)
	_ = l.Close()
	if err := l.TryLock(context.Background(), "job", time.Second); !errors.Is(err, dlock.ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestLock_RaceUnderContention(t *testing.T) {
	mr, _ := newLocker(t, fastOpts()...)

	const workers = 8
	const perWorker = 25
	var counter int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer cli.Close()
			l := dlock.NewRedisClient(cli, fastOpts()...)
			defer l.Close()
			for j := 0; j < perWorker; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := l.Lock(ctx, "counter", 500*time.Millisecond); err != nil {
					cancel()
					t.Errorf("Lock: %v", err)
					return
				}
				// Critical section: read-modify-write.
				v := atomic.LoadInt64(&counter)
				atomic.StoreInt64(&counter, v+1)
				if err := l.Unlock(context.Background(), "counter"); err != nil {
					cancel()
					t.Errorf("Unlock: %v", err)
					return
				}
				cancel()
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&counter); got != workers*perWorker {
		t.Fatalf("counter race: want %d, got %d", workers*perWorker, got)
	}
}

func TestKeyPrefix_AppliedInRedis(t *testing.T) {
	mr, l := newLocker(t, dlock.WithKeyPrefix("custom:"))
	defer l.Close()

	if err := l.TryLock(context.Background(), "job", time.Second); err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !mr.Exists("custom:job") {
		t.Fatal("expected key custom:job to exist in redis")
	}
	_ = l.Unlock(context.Background(), "job")
}
