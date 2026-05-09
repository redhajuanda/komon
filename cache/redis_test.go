package cache_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redhajuanda/komon/cache"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T, useMsgPack bool) (*miniredis.Miniredis, *cache.Redis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return mr, cache.NewRedisClient(cli, useMsgPack)
}

type mgetRow struct {
	name    string
	keys    []string
	seed    func(ctx context.Context, r *cache.Redis)
	dest    func() any
	wantErr error
	check   func(t *testing.T, dest any)
}

func TestRedis_MGet_table(t *testing.T) {
	ctx := context.Background()

	type item struct {
		N int `json:"n"`
	}

	tests := []mgetRow{
		{
			name: "empty_keys_leaves_dest_unchanged",
			keys: nil,
			seed: func(ctx context.Context, r *cache.Redis) {},
			dest: func() any {
				s := []item{{N: 9}}
				return &s
			},
			wantErr: nil,
			check: func(t *testing.T, dest any) {
				got := dest.(*[]item)
				if len(*got) != 1 || (*got)[0].N != 9 {
					t.Fatalf("dest mutated on empty keys: %#v", *got)
				}
			},
		},
		{
			name: "aligned_hits_value_slice",
			keys: []string{"k1", "k2"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "k1", item{N: 1})
				_ = r.Set(ctx, "k2", item{N: 2})
			},
			dest:    func() any { s := []item{}; return &s },
			wantErr: nil,
			check: func(t *testing.T, dest any) {
				got := dest.(*[]item)
				if len(*got) != 2 || (*got)[0].N != 1 || (*got)[1].N != 2 {
					t.Fatalf("got %#v", *got)
				}
			},
		},
		{
			name: "aligned_misses_zero_and_hit",
			keys: []string{"k1", "missing", "k3"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "k1", item{N: 1})
				_ = r.Set(ctx, "k3", item{N: 3})
			},
			dest:    func() any { s := []item{{N: 99}}; return &s },
			wantErr: nil,
			check: func(t *testing.T, dest any) {
				got := dest.(*[]item)
				if len(*got) != 3 {
					t.Fatalf("len got %d want 3", len(*got))
				}
				if (*got)[0].N != 1 || (*got)[1].N != 0 || (*got)[2].N != 3 {
					t.Fatalf("got %#v want [1 0 3]", *got)
				}
			},
		},
		{
			name: "aligned_misses_nil_ptr_slice",
			keys: []string{"a", "gone", "b"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "a", item{N: 10})
				_ = r.Set(ctx, "b", item{N: 20})
			},
			dest:    func() any { s := []*item{}; return &s },
			wantErr: nil,
			check: func(t *testing.T, dest any) {
				got := dest.(*[]*item)
				if len(*got) != 3 {
					t.Fatalf("len %d", len(*got))
				}
				if (*got)[0] == nil || (*got)[0].N != 10 {
					t.Fatalf("idx0 %#v", (*got)[0])
				}
				if (*got)[1] != nil {
					t.Fatalf("idx1 want nil got %#v", (*got)[1])
				}
				if (*got)[2] == nil || (*got)[2].N != 20 {
					t.Fatalf("idx2 %#v", (*got)[2])
				}
			},
		},
		{
			name: "invalid_dest_not_pointer",
			keys: []string{"k1"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "k1", item{N: 1})
			},
			dest:    func() any { var s []item; return s },
			wantErr: cache.ErrInvalidSliceDestination,
			check:   nil,
		},
		{
			name: "invalid_dest_nil_pointer",
			keys: []string{"k1"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "k1", item{N: 1})
			},
			dest:    func() any { return (*[]item)(nil) },
			wantErr: cache.ErrInvalidSliceDestination,
			check:   nil,
		},
		{
			name: "unmarshal_error_bad_payload",
			keys: []string{"bad"},
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Client.Set(ctx, "bad", "not-json", time.Hour).Err()
			},
			dest:  func() any { s := []item{}; return &s },
			check: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			tt.seed(ctx, r)
			dest := tt.dest()
			err := r.MGet(ctx, tt.keys, dest)
			switch {
			case tt.wantErr == cache.ErrInvalidSliceDestination:
				if !errors.Is(err, cache.ErrInvalidSliceDestination) {
					t.Fatalf("err=%v want %v", err, cache.ErrInvalidSliceDestination)
				}
			case tt.name == "unmarshal_error_bad_payload":
				if err == nil {
					t.Fatal("expected unmarshal error")
				}
			case tt.wantErr != nil:
				t.Fatalf("unexpected wantErr branch")
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if tt.check != nil {
				tt.check(t, dest)
			}
		})
	}
}

func TestRedis_HGetAll_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name    string
		seed    func(ctx context.Context, r *cache.Redis)
		wantErr error
		check   func(t *testing.T, dest any)
	}
	type item struct {
		N int `json:"n"`
	}
	tests := []row{
		{
			name:    "missing_or_empty_hash",
			seed:    func(ctx context.Context, r *cache.Redis) {},
			wantErr: cache.ErrNotFound,
			check:   nil,
		},
		{
			name: "values_sorted_by_field_name",
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.HSet(ctx, "h1", []*cache.DataSet{
					{Key: "z", Value: item{N: 1}},
					{Key: "a", Value: item{N: 2}},
					{Key: "m", Value: item{N: 3}},
				})
			},
			wantErr: nil,
			check: func(t *testing.T, dest any) {
				got := dest.(*[]item)
				if len(*got) != 3 {
					t.Fatalf("len %d", len(*got))
				}
				// sorted field names: a, m, z → unmarshaled order
				if (*got)[0].N != 2 || (*got)[1].N != 3 || (*got)[2].N != 1 {
					t.Fatalf("got %#v want N order [2 3 1]", *got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			tt.seed(ctx, r)
			var dest []item
			err := r.HGetAll(ctx, "h1", &dest)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err=%v want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, &dest)
			}
		})
	}
}

func TestRedis_GetMember_table(t *testing.T) {
	ctx := context.Background()
	type item struct {
		N int `json:"n"`
	}
	type row struct {
		name    string
		seed    func(ctx context.Context, r *cache.Redis)
		wantErr error
		check   func(t *testing.T, n int, dest any)
	}
	tests := []row{
		{
			name:    "empty_set",
			seed:    func(ctx context.Context, r *cache.Redis) {},
			wantErr: cache.ErrNotFound,
		},
		{
			name: "resolves_member_keys_via_mget",
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Set(ctx, "obj:1", item{N: 10})
				_ = r.Set(ctx, "obj:2", item{N: 20})
				_ = r.SetMember(ctx, "idx", []*cache.DataSet{
					{Key: "obj:1", Value: "ignored"},
					{Key: "obj:2", Value: struct{}{}},
				})
			},
			wantErr: nil,
			check: func(t *testing.T, n int, dest any) {
				if n != 2 {
					t.Fatalf("count %d want 2", n)
				}
				got := dest.(*[]item)
				if len(*got) != 2 {
					t.Fatalf("len %d", len(*got))
				}
				ns := map[int]bool{(*got)[0].N: true, (*got)[1].N: true}
				if !ns[10] || !ns[20] || len(ns) != 2 {
					t.Fatalf("got %#v", *got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			tt.seed(ctx, r)
			var dest []item
			n, err := r.GetMember(ctx, "idx", &dest)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err=%v want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, n, &dest)
			}
		})
	}
}

func TestRedis_HMGetPipelined_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name    string
		seed    func(ctx context.Context, r *cache.Redis)
		req     map[string]string
		cancel  bool
		wantErr bool
		check   func(t *testing.T, got map[string][]byte)
	}
	tests := []row{
		{
			name: "empty_requests",
			seed: func(ctx context.Context, r *cache.Redis) {},
			req:  map[string]string{},
			check: func(t *testing.T, got map[string][]byte) {
				if len(got) != 0 {
					t.Fatalf("got %#v", got)
				}
			},
		},
		{
			name: "hits_and_misses_omitted",
			seed: func(ctx context.Context, r *cache.Redis) {
				_ = r.Client.HSet(ctx, "hk1", "f", "v1").Err()
				_ = r.Client.HSet(ctx, "hk2", "f", "v2").Err()
			},
			req: map[string]string{"hk1": "f", "missing": "f"},
			check: func(t *testing.T, got map[string][]byte) {
				if string(got["hk1"]) != "v1" {
					t.Fatalf("hk1 got %q", got["hk1"])
				}
				if _, ok := got["missing"]; ok {
					t.Fatal("expected miss omitted")
				}
			},
		},
		{
			name:    "cancelled_context_returns_error",
			seed:    func(ctx context.Context, r *cache.Redis) {},
			req:     map[string]string{"k": "f"},
			cancel:  true,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			tt.seed(ctx, r)
			cctx := ctx
			if tt.cancel {
				var cancel context.CancelFunc
				cctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got, err := r.HMGetPipelined(cctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestRedis_DeleteWithPattern_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name  string
		count int
	}
	tests := []row{
		{name: "batch_under_limit", count: 50},
		{name: "batch_over_limit", count: 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			for i := 0; i < tt.count; i++ {
				k := fmt.Sprintf("pat:%d", i)
				if err := r.Client.Set(ctx, k, "1", 0).Err(); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.DeleteWithPattern(ctx, "pat:*"); err != nil {
				t.Fatal(err)
			}
			left, err := r.Client.Keys(ctx, "pat:*").Result()
			if err != nil {
				t.Fatal(err)
			}
			if len(left) != 0 {
				t.Fatalf("keys left over: %v", left)
			}
		})
	}
}

func TestRedis_Increment_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name     string
		ttl      time.Duration
		pre      func(ctx context.Context, r *cache.Redis)
		want     int64
		checkTTL bool
	}
	tests := []row{
		{
			name:     "creates_and_sets_ttl",
			ttl:      time.Hour,
			checkTTL: true,
			want:     1,
		},
		{
			name: "increments_existing",
			pre: func(ctx context.Context, r *cache.Redis) {
				_, _ = r.Increment(ctx, "ctr", cache.WithTTL(30*time.Minute))
			},
			ttl:  time.Hour,
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			if tt.pre != nil {
				tt.pre(ctx, r)
			}
			n, err := r.Increment(ctx, "ctr", cache.WithTTL(tt.ttl))
			if err != nil {
				t.Fatal(err)
			}
			if n != tt.want {
				t.Fatalf("got %d want %d", n, tt.want)
			}
			if tt.checkTTL {
				pttl, err := r.PTTL(ctx, "ctr").Result()
				if err != nil {
					t.Fatal(err)
				}
				if pttl <= 0 {
					t.Fatalf("pttl=%d want >0", pttl)
				}
			}
		})
	}
}

func TestRedis_SetGet_roundtrip_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name  string
		value any
	}
	type payload struct {
		X string `json:"x"`
	}
	tests := []row{
		{name: "struct_json", value: payload{X: "hi"}},
		{name: "primitive_int", value: 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			key := "k:" + tt.name
			if err := r.Set(ctx, key, tt.value); err != nil {
				t.Fatal(err)
			}
			switch v := tt.value.(type) {
			case payload:
				var got payload
				if err := r.Get(ctx, key, &got); err != nil {
					t.Fatal(err)
				}
				if got.X != v.X {
					t.Fatalf("got %+v", got)
				}
			case int:
				got, err := r.GetInt(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				if got != int64(v) {
					t.Fatalf("got %d", got)
				}
			}
		})
	}
}

func TestRedis_ErrNotFound_table(t *testing.T) {
	ctx := context.Background()
	type row struct {
		name string
		act  func(t *testing.T, c *cache.Redis) error
	}
	tests := []row{
		{
			name: "Get_missing_key",
			act: func(t *testing.T, c *cache.Redis) error {
				var v struct {
					X int `json:"x"`
				}
				return c.Get(ctx, "nope", &v)
			},
		},
		{
			name: "GetString_missing_key",
			act: func(t *testing.T, c *cache.Redis) error {
				_, err := c.GetString(ctx, "nope")
				return err
			},
		},
		{
			name: "GetInt_missing_key",
			act: func(t *testing.T, c *cache.Redis) error {
				_, err := c.GetInt(ctx, "nope")
				return err
			},
		},
		{
			name: "HGet_missing_field",
			act: func(t *testing.T, c *cache.Redis) error {
				var v struct {
					X int `json:"x"`
				}
				return c.HGet(ctx, "h", "f", &v)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := newTestRedis(t, false)
			err := tt.act(t, r)
			if !errors.Is(err, cache.ErrNotFound) {
				t.Fatalf("err=%v (%T) want ErrNotFound", err, err)
			}
		})
	}
}

// TestRedis_SetGet_Primitives_RoundTrip pins the B1 fix: Set must route
// non-string/[]byte values through the configured serializer so Get can
// unmarshal them. Previously Set wrote raw "true"/"42" while Get always ran
// JSON/msgpack Unmarshal, breaking round-trips.
func TestRedis_SetGet_Primitives_RoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, useMP := range []bool{false, true} {
		name := "json"
		if useMP {
			name = "msgpack"
		}
		t.Run(name, func(t *testing.T) {
			t.Run("bool", func(t *testing.T) {
				_, r := newTestRedis(t, useMP)
				if err := r.Set(ctx, "b", true); err != nil {
					t.Fatal(err)
				}
				var got bool
				if err := r.Get(ctx, "b", &got); err != nil {
					t.Fatal(err)
				}
				if !got {
					t.Fatalf("got %v", got)
				}
			})
			t.Run("int", func(t *testing.T) {
				_, r := newTestRedis(t, useMP)
				if err := r.Set(ctx, "i", 1234); err != nil {
					t.Fatal(err)
				}
				var got int
				if err := r.Get(ctx, "i", &got); err != nil {
					t.Fatal(err)
				}
				if got != 1234 {
					t.Fatalf("got %d", got)
				}
			})
			t.Run("float", func(t *testing.T) {
				_, r := newTestRedis(t, useMP)
				if err := r.Set(ctx, "f", 3.14); err != nil {
					t.Fatal(err)
				}
				var got float64
				if err := r.Get(ctx, "f", &got); err != nil {
					t.Fatal(err)
				}
				if got != 3.14 {
					t.Fatalf("got %v", got)
				}
			})
			t.Run("string_raw", func(t *testing.T) {
				_, r := newTestRedis(t, useMP)
				if err := r.Set(ctx, "s", "hello"); err != nil {
					t.Fatal(err)
				}
				// Strings are written raw, so GetString remains the canonical reader.
				got, err := r.GetString(ctx, "s")
				if err != nil {
					t.Fatal(err)
				}
				if got != "hello" {
					t.Fatalf("got %q", got)
				}
			})
			t.Run("struct_msgpack", func(t *testing.T) {
				_, r := newTestRedis(t, useMP)
				type T struct {
					A int    `json:"a" msgpack:"a"`
					B string `json:"b" msgpack:"b"`
				}
				in := T{A: 7, B: "x"}
				if err := r.Set(ctx, "t", in); err != nil {
					t.Fatal(err)
				}
				var out T
				if err := r.Get(ctx, "t", &out); err != nil {
					t.Fatal(err)
				}
				if out != in {
					t.Fatalf("got %+v", out)
				}
			})
		})
	}
}

// TestRedis_Exists_table covers the B2 fix: error is now propagated and the
// happy path returns (true|false, nil).
func TestRedis_Exists_table(t *testing.T) {
	ctx := context.Background()
	mr, r := newTestRedis(t, false)

	// false when no keys.
	got, err := r.Exists(ctx, "missing")
	if err != nil || got {
		t.Fatalf("missing: got=%v err=%v", got, err)
	}

	// true when key exists.
	if err := r.Set(ctx, "present", "v"); err != nil {
		t.Fatal(err)
	}
	got, err = r.Exists(ctx, "present")
	if err != nil || !got {
		t.Fatalf("present: got=%v err=%v", got, err)
	}

	// empty keys list returns (false, nil) without touching redis.
	got, err = r.Exists(ctx)
	if err != nil || got {
		t.Fatalf("no-keys: got=%v err=%v", got, err)
	}

	// closed server surfaces the error.
	mr.Close()
	_, err = r.Exists(ctx, "present")
	if err == nil {
		t.Fatal("expected error when redis is unreachable")
	}
}

// TestRedis_SetMember_VariadicFix pins the B6 fix: SAdd must add members
// individually, not as one packed slice value. With the bug, SMEMBERS returned
// a single comma/space-formatted value.
func TestRedis_SetMember_VariadicFix(t *testing.T) {
	ctx := context.Background()
	type item struct {
		N int `json:"n"`
	}
	_, r := newTestRedis(t, false)
	_ = r.Set(ctx, "a", item{N: 1})
	_ = r.Set(ctx, "b", item{N: 2})
	_ = r.Set(ctx, "c", item{N: 3})

	if err := r.SetMember(ctx, "idx", []*cache.DataSet{
		{Key: "a"}, {Key: "b"}, {Key: "c"},
	}); err != nil {
		t.Fatal(err)
	}

	members, err := r.Client.SMembers(ctx, "idx").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("want 3 members, got %d (%v)", len(members), members)
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m] = true
	}
	if !(seen["a"] && seen["b"] && seen["c"]) {
		t.Fatalf("members not stored individually: %v", members)
	}
}

// TestRedis_IncrementFixed pins the B4 documentation/behavior split.
func TestRedis_IncrementFixed(t *testing.T) {
	ctx := context.Background()
	mr, r := newTestRedis(t, false)

	if _, err := r.IncrementFixed(ctx, "ctr", cache.WithTTL(time.Hour)); err != nil {
		t.Fatal(err)
	}
	firstTTL := mr.TTL("ctr")
	if firstTTL <= 0 {
		t.Fatalf("ttl not set on first call: %v", firstTTL)
	}

	mr.FastForward(10 * time.Minute)
	if _, err := r.IncrementFixed(ctx, "ctr", cache.WithTTL(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Fixed-window TTL should NOT be refreshed: remaining TTL must be
	// roughly firstTTL - 10m, not back at ~1h.
	secondTTL := mr.TTL("ctr")
	if secondTTL > firstTTL-5*time.Minute {
		t.Fatalf("IncrementFixed reset TTL: first=%v second=%v", firstTTL, secondTTL)
	}
}

// TestRedis_Increment_Sliding documents B4: Increment refreshes TTL on every
// call. This test pins the documented behavior so accidental changes are loud.
func TestRedis_Increment_Sliding(t *testing.T) {
	ctx := context.Background()
	mr, r := newTestRedis(t, false)

	if _, err := r.Increment(ctx, "ctr", cache.WithTTL(time.Hour)); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(30 * time.Minute)
	if _, err := r.Increment(ctx, "ctr", cache.WithTTL(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ttl := mr.TTL("ctr")
	// Sliding window: TTL should be ~1h, certainly more than 45m.
	if ttl < 45*time.Minute {
		t.Fatalf("expected sliding TTL ~1h, got %v", ttl)
	}
}
