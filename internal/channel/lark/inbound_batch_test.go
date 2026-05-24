package lark

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

type batchCapture struct {
	cap *captureHandler
}

func (b batchCapture) ProcessInbound(ctx context.Context, ch biz.Channel, ev port.InboundEvent) error {
	return b.cap.ProcessInbound(ctx, ch, ev)
}

type captureHandler struct {
	mu             sync.Mutex
	text           string
	idempotencyKey string
}

func (b *captureHandler) ProcessInbound(_ context.Context, _ biz.Channel, ev port.InboundEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text = ev.Text
	b.idempotencyKey = ev.IdempotencyKey
	return nil
}

func TestTextInboundBatcher_mergesMessages(t *testing.T) {
	batcher := NewTextInboundBatcher()
	batcher.debounce = 80 * time.Millisecond
	cap := &captureHandler{}
	ch := biz.Channel{ID: "ch-1"}
	ctx := context.Background()
	handler := batchCapture{cap: cap}

	batcher.Submit(ctx, handler, ch, port.InboundEvent{Text: "hello", PeerID: "p1"})
	batcher.Submit(ctx, handler, ch, port.InboundEvent{Text: "world", PeerID: "p1"})
	time.Sleep(150 * time.Millisecond)

	cap.mu.Lock()
	got := cap.text
	cap.mu.Unlock()
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("expected merged text, got %q", got)
	}
}

func TestTextInboundBatcher_mergedIdempotencyKey(t *testing.T) {
	batcher := NewTextInboundBatcher()
	batcher.debounce = 80 * time.Millisecond
	cap := &captureHandler{}
	ch := biz.Channel{ID: "ch-1"}
	ctx := context.Background()
	handler := batchCapture{cap: cap}

	batcher.Submit(ctx, handler, ch, port.InboundEvent{Text: "a", PeerID: "p1", IdempotencyKey: "feishu:om_1"})
	batcher.Submit(ctx, handler, ch, port.InboundEvent{Text: "b", PeerID: "p1", IdempotencyKey: "feishu:om_2"})
	time.Sleep(150 * time.Millisecond)

	cap.mu.Lock()
	key := cap.idempotencyKey
	cap.mu.Unlock()
	if key != "feishu:batch:feishu:om_1+feishu:om_2" {
		t.Fatalf("expected batch idempotency key, got %q", key)
	}
}

func TestBatchIdempotencyKey_helpers(t *testing.T) {
	if got := batchIdempotencyKey(nil); got != "" {
		t.Fatalf("empty keys: got %q", got)
	}
	if got := batchIdempotencyKey([]string{"feishu:om_1"}); got != "feishu:om_1" {
		t.Fatalf("single key: got %q", got)
	}
	if got := batchIdempotencyKey([]string{"feishu:om_1", "feishu:om_2"}); got != "feishu:batch:feishu:om_1+feishu:om_2" {
		t.Fatalf("merged key: got %q", got)
	}
	keys := appendBatchIdempotencyKey([]string{"feishu:om_1"}, "feishu:om_1")
	if len(keys) != 1 {
		t.Fatalf("dedupe failed: %v", keys)
	}
}
