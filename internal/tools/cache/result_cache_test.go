package cache

import (
	"testing"
	"time"
)

func TestResultCache_hitMiss(t *testing.T) {
	c := NewResultCache(8)
	args := []byte(`{"q":"hello"}`)
	if _, ok := c.Get("search", args); ok {
		t.Fatal("expected miss")
	}
	c.Put("search", args, map[string]any{"ok": true}, 0)
	if _, ok := c.Get("search", args); ok {
		t.Fatal("zero ttl should not store")
	}
	c.Put("search", args, "result", 30*time.Second)
	got, ok := c.Get("search", args)
	if !ok || got != "result" {
		t.Fatalf("expected hit, got %v ok=%v", got, ok)
	}
}

func TestPolicyFromToolJSON(t *testing.T) {
	p := PolicyFromToolJSON(`{"cache_enabled":true,"cache_ttl_sec":120}`, "{}")
	if !p.Enabled || p.TTL != 120*time.Second {
		t.Fatalf("unexpected policy: %+v", p)
	}
}
