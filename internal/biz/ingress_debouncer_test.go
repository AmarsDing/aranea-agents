package biz

import (
	"context"
	"testing"
	"time"
)

func TestIngressPeerDebouncer_mergesMessages(t *testing.T) {
	b := NewIngressPeerDebouncer(50*time.Millisecond, nil)
	done := make(chan string, 1)
	run := func(ctx context.Context) error {
		done <- "flushed"
		return nil
	}
	b.Submit(context.Background(), "ch-1", "p1", "p1", "hi", "m1", run)
	b.Submit(context.Background(), "ch-1", "p1", "p1", "there", "m2", run)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for debounced flush")
	}
}

func TestIngressPeerDebouncer_immediateEmptyText(t *testing.T) {
	b := NewIngressPeerDebouncer(50*time.Millisecond, nil)
	done := make(chan string, 1)
	run := func(ctx context.Context) error {
		done <- "immediate"
		return nil
	}
	b.Submit(context.Background(), "ch-1", "p1", "p1", "", "m1", run)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for immediate flush of empty text")
	}
}

func TestIngressPeerDebouncer_nilReceiver(t *testing.T) {
	var b *IngressPeerDebouncer
	b.Submit(context.Background(), "ch-1", "p1", "p1", "hi", "m1", nil)
}

func TestIngressPeerDebouncer_nilRun(t *testing.T) {
	b := NewIngressPeerDebouncer(50*time.Millisecond, nil)
	b.Submit(context.Background(), "ch-1", "p1", "p1", "hi", "m1", nil)
}
