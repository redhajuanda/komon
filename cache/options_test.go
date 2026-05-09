package cache

import (
	"testing"
	"time"
)

func TestWithTTL(t *testing.T) {
	opts := DefaultOptions()
	WithTTL(5 * time.Minute)(opts)

	if opts.expire != 5*time.Minute {
		t.Errorf("WithTTL() = %v, want %v", opts.expire, 5*time.Minute)
	}
}

func TestWithTTL_Zero(t *testing.T) {
	opts := DefaultOptions()
	originalExpire := opts.expire
	WithTTL(0)(opts)

	if opts.expire != originalExpire {
		t.Errorf("WithTTL(0) should not change expire, got %v", opts.expire)
	}
}

func TestWithTTLJitter(t *testing.T) {
	baseDuration := 10 * time.Minute

	t.Run("default jitter 10%", func(t *testing.T) {
		minExpected := time.Duration(float64(baseDuration) * 0.9)
		maxExpected := time.Duration(float64(baseDuration) * 1.1)

		for range 100 {
			opts := DefaultOptions()
			WithTTLJitter(baseDuration)(opts)
			if opts.expire < minExpected || opts.expire > maxExpected {
				t.Errorf("WithTTLJitter() = %v, want between %v and %v", opts.expire, minExpected, maxExpected)
			}
		}
	})

	t.Run("custom jitter 20%", func(t *testing.T) {
		minExpected := time.Duration(float64(baseDuration) * 0.8)
		maxExpected := time.Duration(float64(baseDuration) * 1.2)

		for range 100 {
			opts := DefaultOptions()
			WithTTLJitter(baseDuration, 0.20)(opts)
			if opts.expire < minExpected || opts.expire > maxExpected {
				t.Errorf("WithTTLJitter() = %v, want between %v and %v", opts.expire, minExpected, maxExpected)
			}
		}
	})

	t.Run("zero jitter percent uses default", func(t *testing.T) {
		minExpected := time.Duration(float64(baseDuration) * 0.9)
		maxExpected := time.Duration(float64(baseDuration) * 1.1)

		opts := DefaultOptions()
		WithTTLJitter(baseDuration, 0)(opts)
		if opts.expire < minExpected || opts.expire > maxExpected {
			t.Errorf("WithTTLJitter() = %v, want between %v and %v", opts.expire, minExpected, maxExpected)
		}
	})
}

func TestWithMsgPack(t *testing.T) {
	opts := DefaultOptions()
	WithMsgPack()(opts)

	if opts.serialize != SerializeMessagePack {
		t.Errorf("WithMsgPack() = %v, want %v", opts.serialize, SerializeMessagePack)
	}
}

func TestWithJSON(t *testing.T) {
	opts := DefaultOptions()
	opts.serialize = SerializeMessagePack
	WithJSON()(opts)

	if opts.serialize != SerializeJSON {
		t.Errorf("WithJSON() = %v, want %v", opts.serialize, SerializeJSON)
	}
}

func TestGetExpire_DefaultGuard(t *testing.T) {
	t.Run("returns default when zero", func(t *testing.T) {
		opts := &Options{expire: 0}
		got := opts.GetExpire()
		if got != DefaultExpire {
			t.Errorf("GetExpire() with zero = %v, want %v", got, DefaultExpire)
		}
	})

	t.Run("returns default when negative", func(t *testing.T) {
		opts := &Options{expire: -1 * time.Second}
		got := opts.GetExpire()
		if got != DefaultExpire {
			t.Errorf("GetExpire() with negative = %v, want %v", got, DefaultExpire)
		}
	})

	t.Run("returns set value when positive", func(t *testing.T) {
		opts := &Options{expire: 5 * time.Minute}
		got := opts.GetExpire()
		if got != 5*time.Minute {
			t.Errorf("GetExpire() = %v, want %v", got, 5*time.Minute)
		}
	})

	t.Run("DefaultOptions() returns DefaultExpire via GetExpire", func(t *testing.T) {
		got := DefaultOptions().GetExpire()
		if got != DefaultExpire {
			t.Errorf("DefaultOptions().GetExpire() = %v, want %v", got, DefaultExpire)
		}
	})
}

func TestApplyJitter(t *testing.T) {
	baseDuration := 10 * time.Minute
	minExpected := time.Duration(float64(baseDuration) * 0.9)
	maxExpected := time.Duration(float64(baseDuration) * 1.1)

	t.Run("applies jitter within range", func(t *testing.T) {
		for range 100 {
			result := ApplyJitter(baseDuration)
			if result < minExpected || result > maxExpected {
				t.Errorf("ApplyJitter() = %v, want between %v and %v", result, minExpected, maxExpected)
			}
		}
	})

	t.Run("zero duration returns zero", func(t *testing.T) {
		result := ApplyJitter(0)
		if result != 0 {
			t.Errorf("ApplyJitter(0) = %v, want 0", result)
		}
	})

	t.Run("negative duration returns same", func(t *testing.T) {
		result := ApplyJitter(-5 * time.Minute)
		if result != -5*time.Minute {
			t.Errorf("ApplyJitter(-5m) = %v, want -5m", result)
		}
	})

	t.Run("produces varied results for batch scenarios", func(t *testing.T) {
		results := make(map[time.Duration]bool)
		for range 50 {
			result := ApplyJitter(baseDuration)
			results[result] = true
		}
		if len(results) < 10 {
			t.Errorf("ApplyJitter() produced only %d unique values in 50 iterations, expected more variance", len(results))
		}
	})
}
