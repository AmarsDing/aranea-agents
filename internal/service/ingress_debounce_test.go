package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func TestIngressMessageDedupe_claimWithinTTL(t *testing.T) {
	d := newIngressMessageDedupe(time.Minute)
	now := time.Now()
	if !d.claim("ch:msg-1", now) {
		t.Fatal("first claim should succeed")
	}
	if d.claim("ch:msg-1", now.Add(30*time.Second)) {
		t.Fatal("duplicate within TTL should fail")
	}
}

func TestIngressMessageDedupe_claimAfterTTL(t *testing.T) {
	d := newIngressMessageDedupe(time.Minute)
	now := time.Now()
	if !d.claim("ch:msg-1", now) {
		t.Fatal("first claim should succeed")
	}
	if !d.claim("ch:msg-1", now.Add(2*time.Minute)) {
		t.Fatal("claim after TTL should succeed")
	}
}

func TestShouldSkipRecentDuplicate(t *testing.T) {
	now := time.Now()
	if !biz.ShouldSkipRecentDuplicate(now.Add(-30*time.Second), time.Minute, now) {
		t.Fatal("expected skip within TTL")
	}
	if biz.ShouldSkipRecentDuplicate(now.Add(-2*time.Minute), time.Minute, now) {
		t.Fatal("expected allow after TTL")
	}
}

func TestMergeIngressIdempotencyKeys(t *testing.T) {
	got := biz.MergeIngressIdempotencyKeys([]string{"a", "b"})
	if got != "a+b" {
		t.Fatalf("got %q", got)
	}
}

func TestIngressPeerDebouncer_mergesMessages(t *testing.T) {
	b := newIngressPeerDebouncer(50*time.Millisecond, nil)
	done := make(chan port.InboundEvent, 1)
	ch := biz.Channel{ID: "ch-1"}
	b.submit(context.Background(), ch, port.InboundEvent{PeerID: "p1", Text: "hi", IdempotencyKey: "m1", PlatformType: "slack"}, func(_ context.Context, _ biz.Channel, ev port.InboundEvent) error {
		done <- ev
		return nil
	})
	b.submit(context.Background(), ch, port.InboundEvent{PeerID: "p1", Text: "there", IdempotencyKey: "m2", PlatformType: "slack"}, func(_ context.Context, _ biz.Channel, ev port.InboundEvent) error {
		done <- ev
		return nil
	})
	select {
	case ev := <-done:
		if ev.Text != "hi\nthere" {
			t.Fatalf("text=%q", ev.Text)
		}
		if ev.IdempotencyKey != "m1+m2" {
			t.Fatalf("key=%q", ev.IdempotencyKey)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced flush")
	}
}
