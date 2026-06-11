package a2a

import (
	"context"
	"testing"
)

func TestMemoryLimiter_AllowWithinWindow(t *testing.T) {
	lim := NewInvokeLimiter(2, 0)
	ok, err := lim.Allow(context.Background(), "a", "b")
	if err != nil || !ok {
		t.Fatalf("expected first invoke allowed, ok=%v err=%v", ok, err)
	}
	ok, err = lim.Allow(context.Background(), "a", "b")
	if err != nil || !ok {
		t.Fatalf("expected second invoke allowed, ok=%v err=%v", ok, err)
	}
	ok, err = lim.Allow(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatal("expected third invoke blocked")
	}
	ok, err = lim.Allow(context.Background(), "a", "c")
	if err != nil || !ok {
		t.Fatalf("expected different callee allowed, ok=%v err=%v", ok, err)
	}
}

func TestNewLimiter_PicksMemoryWhenRedisNil(t *testing.T) {
	lim := NewLimiter(DefaultLimiterConfig(), nil, nil)
	if _, ok := lim.(*memorySlidingWindowLimiter); !ok {
		t.Fatalf("expected memory limiter, got %T", lim)
	}
	ok, err := lim.Allow(context.Background(), "x", "y")
	if err != nil || !ok {
		t.Fatalf("expected first allow, ok=%v err=%v", ok, err)
	}
}

func TestNewRedisSlidingWindowLimiter_NilClient(t *testing.T) {
	// A nil-client limiter must not panic and must allow all calls; this
	// is the safe behaviour when a misconfigured Redis is propagated.
	rl := NewRedisSlidingWindowLimiter(nil, DefaultLimiterConfig())
	ok, err := rl.Allow(context.Background(), "x", "y")
	if err != nil || !ok {
		t.Fatalf("expected nil-client to allow, ok=%v err=%v", ok, err)
	}
}

func TestLimiterConfig_ApplyDefaults(t *testing.T) {
	cfg := LimiterConfig{}.applyDefaults()
	if cfg.WindowSize <= 0 || cfg.MaxInvokes <= 0 || cfg.KeyPrefix == "" {
		t.Fatalf("expected defaults applied, got %+v", cfg)
	}
}

func TestUniqueMember_Distinct(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		m, err := uniqueMember(int64(i))
		if err != nil {
			t.Fatalf("uniqueMember err: %v", err)
		}
		if _, dup := seen[m]; dup {
			t.Fatalf("duplicate member %q at i=%d", m, i)
		}
		seen[m] = struct{}{}
	}
}
