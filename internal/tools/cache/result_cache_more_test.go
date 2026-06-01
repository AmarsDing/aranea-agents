package cache

import (
	"testing"
	"time"
)

func TestEvictExpiredLocked(t *testing.T) {
	t.Run("expired_entries_removed", func(t *testing.T) {
		c := NewResultCache(8)
		now := time.Now()
		c.mu.Lock()
		c.items["expired1"] = entry{result: "old1", expiresAt: now.Add(-time.Hour), accessedAt: now}
		c.items["expired2"] = entry{result: "old2", expiresAt: now.Add(-time.Minute), accessedAt: now}
		c.items["valid"] = entry{result: "new", expiresAt: now.Add(time.Hour), accessedAt: now}
		c.mu.Unlock()

		c.mu.Lock()
		EvictExpiredLocked(c, now)
		c.mu.Unlock()

		c.mu.RLock()
		_, ok1 := c.items["expired1"]
		_, ok2 := c.items["expired2"]
		_, ok3 := c.items["valid"]
		c.mu.RUnlock()

		if ok1 {
			t.Fatal("expected expired1 to be removed")
		}
		if ok2 {
			t.Fatal("expected expired2 to be removed")
		}
		if !ok3 {
			t.Fatal("expected valid to remain")
		}
	})

	t.Run("empty_cache", func(t *testing.T) {
		c := NewResultCache(8)
		c.mu.Lock()
		EvictExpiredLocked(c, time.Now())
		c.mu.Unlock()
	})

	t.Run("all_expired", func(t *testing.T) {
		c := NewResultCache(8)
		now := time.Now()
		c.mu.Lock()
		c.items["a"] = entry{result: "va", expiresAt: now.Add(-time.Hour), accessedAt: now}
		c.items["b"] = entry{result: "vb", expiresAt: now.Add(-time.Minute), accessedAt: now}
		c.mu.Unlock()

		c.mu.Lock()
		EvictExpiredLocked(c, now)
		c.mu.Unlock()

		c.mu.RLock()
		got := len(c.items)
		c.mu.RUnlock()

		if got != 0 {
			t.Fatalf("expected 0 items, got %d", got)
		}
	})

	t.Run("none_expired", func(t *testing.T) {
		c := NewResultCache(8)
		now := time.Now()
		c.mu.Lock()
		c.items["a"] = entry{result: "va", expiresAt: now.Add(time.Hour), accessedAt: now}
		c.items["b"] = entry{result: "vb", expiresAt: now.Add(time.Minute), accessedAt: now}
		c.mu.Unlock()

		c.mu.Lock()
		EvictExpiredLocked(c, now)
		c.mu.Unlock()

		c.mu.RLock()
		got := len(c.items)
		c.mu.RUnlock()

		if got != 2 {
			t.Fatalf("expected 2 items, got %d", got)
		}
	})
}

func TestEvictLRULocked(t *testing.T) {
	t.Run("evicts_oldest_accessed", func(t *testing.T) {
		c := NewResultCache(8)
		now := time.Now()
		c.mu.Lock()
		c.items["old"] = entry{result: "old", expiresAt: now.Add(time.Hour), accessedAt: now.Add(-time.Hour)}
		c.items["mid"] = entry{result: "mid", expiresAt: now.Add(time.Hour), accessedAt: now.Add(-time.Minute)}
		c.items["recent"] = entry{result: "recent", expiresAt: now.Add(time.Hour), accessedAt: now}
		c.mu.Unlock()

		c.mu.Lock()
		EvictLRULocked(c)
		c.mu.Unlock()

		c.mu.RLock()
		_, oldOk := c.items["old"]
		_, midOk := c.items["mid"]
		_, recentOk := c.items["recent"]
		c.mu.RUnlock()

		if oldOk {
			t.Fatal("expected old to be evicted")
		}
		if !midOk {
			t.Fatal("expected mid to remain")
		}
		if !recentOk {
			t.Fatal("expected recent to remain")
		}
	})

	t.Run("empty_cache", func(t *testing.T) {
		c := NewResultCache(8)
		c.mu.Lock()
		EvictLRULocked(c)
		c.mu.Unlock()
	})

	t.Run("single_entry_evicted", func(t *testing.T) {
		c := NewResultCache(8)
		now := time.Now()
		c.mu.Lock()
		c.items["only"] = entry{result: "only", expiresAt: now.Add(time.Hour), accessedAt: now}
		c.mu.Unlock()

		c.mu.Lock()
		EvictLRULocked(c)
		c.mu.Unlock()

		c.mu.RLock()
		_, ok := c.items["only"]
		c.mu.RUnlock()

		if ok {
			t.Fatal("expected only entry to be evicted")
		}
	})
}

func TestResultCache_TTLExpiration(t *testing.T) {
	c := NewResultCache(8)
	c.Put("tool", []byte("args"), "value", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := c.Get("tool", []byte("args"))
	if ok {
		t.Fatal("expected miss after TTL expiration")
	}
}

func TestResultCache_LRUEviction(t *testing.T) {
	c := NewResultCache(2)
	c.Put("tool1", []byte("a"), "v1", time.Hour)
	c.Put("tool2", []byte("b"), "v2", time.Hour)
	time.Sleep(time.Millisecond)
	_, _ = c.Get("tool1", []byte("a"))
	c.Put("tool3", []byte("c"), "v3", time.Hour)

	if _, ok := c.Get("tool2", []byte("b")); ok {
		t.Fatal("expected tool2 to be evicted (LRU)")
	}
	if _, ok := c.Get("tool1", []byte("a")); !ok {
		t.Fatal("expected tool1 to remain (recently accessed)")
	}
	if _, ok := c.Get("tool3", []byte("c")); !ok {
		t.Fatal("expected tool3 to remain (just added)")
	}
}

func TestResultCache_Overwrite(t *testing.T) {
	c := NewResultCache(8)
	c.Put("tool", []byte("args"), "value1", time.Hour)
	c.Put("tool", []byte("args"), "value2", 2*time.Hour)
	got, ok := c.Get("tool", []byte("args"))
	if !ok {
		t.Fatal("expected hit")
	}
	if got != "value2" {
		t.Fatalf("expected value2, got %v", got)
	}

	k := c.key("tool", []byte("args"))
	c.mu.RLock()
	e := c.items[k]
	c.mu.RUnlock()

	now := time.Now()
	if e.expiresAt.Before(now.Add(90 * time.Minute)) {
		t.Fatal("expected TTL to be refreshed to 2 hours")
	}
}

func TestPolicyFromObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want CachePolicy
	}{
		{
			name: "valid_enabled_with_ttl",
			raw:  `{"cache_enabled":true,"cache_ttl_sec":120}`,
			want: CachePolicy{Enabled: true, TTL: 120 * time.Second},
		},
		{
			name: "only_ttl_sec_enables",
			raw:  `{"cache_ttl_sec":60}`,
			want: CachePolicy{Enabled: true, TTL: 60 * time.Second},
		},
		{
			name: "only_enabled_uses_default_ttl",
			raw:  `{"cache_enabled":true}`,
			want: CachePolicy{Enabled: true, TTL: 300 * time.Second},
		},
		{
			name: "both_disabled",
			raw:  `{"cache_enabled":false,"cache_ttl_sec":0}`,
			want: CachePolicy{},
		},
		{
			name: "empty_string",
			raw:  "",
			want: CachePolicy{},
		},
		{
			name: "invalid_json",
			raw:  "not-json",
			want: CachePolicy{},
		},
		{
			name: "empty_object",
			raw:  "{}",
			want: CachePolicy{},
		},
		{
			name: "enabled_true_ttl_zero_uses_default",
			raw:  `{"cache_enabled":true,"cache_ttl_sec":0}`,
			want: CachePolicy{Enabled: true, TTL: 300 * time.Second},
		},
		{
			name: "whitespace_trimmed_before_parse",
			raw:  `  {"cache_enabled":true,"cache_ttl_sec":60}`,
			want: CachePolicy{Enabled: true, TTL: 60 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PolicyFromObject(tt.raw)
			if got.Enabled != tt.want.Enabled {
				t.Fatalf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
			}
			if got.TTL != tt.want.TTL {
				t.Fatalf("TTL = %v, want %v", got.TTL, tt.want.TTL)
			}
		})
	}
}

func TestTrimJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"leading_spaces", "  hello", "hello"},
		{"leading_newlines", "\nhello", "hello"},
		{"mixed_leading", "  \n  hello", "hello"},
		{"already_trimmed", "hello", "hello"},
		{"tab_not_trimmed", "\thello", "\thello"},
		{"trailing_not_trimmed", "hello  ", "hello  "},
		{"empty_string", "", ""},
		{"only_spaces", "   ", ""},
		{"only_newlines", "\n\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimJSON(tt.input)
			if got != tt.want {
				t.Fatalf("TrimJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBoolish(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"bool_true", true, true},
		{"bool_false", false, false},
		{"float64_nonzero", float64(1), true},
		{"float64_zero", float64(0), false},
		{"float64_negative", float64(-1), true},
		{"nil", nil, false},
		{"string_true", "true", false},
		{"string_1", "1", false},
		{"int_1", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Boolish(tt.input)
			if got != tt.want {
				t.Fatalf("Boolish(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNumberish(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  int
	}{
		{"int_value", int(5), 5},
		{"float64_truncated", float64(5.7), 5},
		{"float64_whole", float64(5.0), 5},
		{"string_value", "5", 0},
		{"nil_value", nil, 0},
		{"int_negative", int(-3), -3},
		{"float64_negative", float64(-2.9), -2},
		{"float64_zero", float64(0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Numberish(tt.input)
			if got != tt.want {
				t.Fatalf("Numberish(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
