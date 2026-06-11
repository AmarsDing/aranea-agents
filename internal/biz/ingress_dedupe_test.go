package biz

import (
	"testing"
	"time"
)

func TestIngressMessageDedupe_Claim(t *testing.T) {
	d := NewIngressMessageDedupe(5 * time.Minute)
	now := time.Now()

	if !d.claim("key1", now) {
		t.Fatal("first claim should succeed")
	}
	if d.claim("key1", now) {
		t.Fatal("duplicate claim should fail within TTL")
	}
	if !d.claim("key2", now) {
		t.Fatal("different key should succeed")
	}
}

func TestIngressMessageDedupe_ClaimExpired(t *testing.T) {
	d := NewIngressMessageDedupe(1 * time.Second)
	past := time.Now().Add(-2 * time.Second)

	if !d.claim("key1", past) {
		t.Fatal("first claim should succeed")
	}
	now := time.Now()
	if !d.claim("key1", now) {
		t.Fatal("claim after TTL expiry should succeed")
	}
}

func TestIngressMessageDedupe_NilReceiver(t *testing.T) {
	var d *IngressMessageDedupe
	if !d.claim("key1", time.Now()) {
		t.Fatal("nil dedupe should always allow")
	}
}

func TestIngressMessageDedupe_EmptyKey(t *testing.T) {
	d := NewIngressMessageDedupe(5 * time.Minute)
	if !d.claim("", time.Now()) {
		t.Fatal("empty key should always allow")
	}
}

func TestIngressMessageDedupe_ZeroTTL(t *testing.T) {
	d := NewIngressMessageDedupe(0)
	if d.ttl != DefaultMessageDedupeTTL {
		t.Fatalf("zero TTL should default to %v, got %v", DefaultMessageDedupeTTL, d.ttl)
	}
}

func TestIngressMessageDedupe_claimWithinTTL(t *testing.T) {
	d := NewIngressMessageDedupe(time.Minute)
	now := time.Now()
	if !d.claim("ch:msg-1", now) {
		t.Fatal("first claim should succeed")
	}
	if d.claim("ch:msg-1", now.Add(30*time.Second)) {
		t.Fatal("duplicate within TTL should fail")
	}
}

func TestIngressMessageDedupe_claimAfterTTL(t *testing.T) {
	d := NewIngressMessageDedupe(time.Minute)
	now := time.Now()
	if !d.claim("ch:msg-1", now) {
		t.Fatal("first claim should succeed")
	}
	if !d.claim("ch:msg-1", now.Add(2*time.Minute)) {
		t.Fatal("claim after TTL should succeed")
	}
}

func TestIngressMessageDedupe_ClaimMessage(t *testing.T) {
	d := NewIngressMessageDedupe(5 * time.Minute)
	if !d.ClaimMessage("ch1", "msg1") {
		t.Fatal("first claim should succeed")
	}
	if d.ClaimMessage("ch1", "msg1") {
		t.Fatal("duplicate claim should fail")
	}
	if !d.ClaimMessage("ch1", "msg2") {
		t.Fatal("different message should succeed")
	}
}

func TestIngressMessageDedupe_Inflight(t *testing.T) {
	d := NewIngressMessageDedupe(5 * time.Minute)
	if !d.TryAcquireInflight("key1") {
		t.Fatal("first acquire should succeed")
	}
	if d.TryAcquireInflight("key1") {
		t.Fatal("duplicate acquire should fail")
	}
	d.ReleaseInflight("key1")
	if !d.TryAcquireInflight("key1") {
		t.Fatal("acquire after release should succeed")
	}
}

func TestIngressMessageDedupe_Stop(t *testing.T) {
	d := NewIngressMessageDedupe(5 * time.Minute)
	d.Stop()
}
