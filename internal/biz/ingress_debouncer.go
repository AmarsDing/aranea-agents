package biz

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

type ingressPeerKey struct {
	channelID string
	peerKey   string
}

type ingressPeerBatch struct {
	channelID string
	peerID    string
	peerKey   string
	text      string
	timer     *time.Timer
	keys      []string
}

// IngressPeerDebouncer merges rapid sequential messages from the same peer
// into a single batch before execution.
type IngressPeerDebouncer struct {
	delay   time.Duration
	mu      sync.Mutex
	pending map[ingressPeerKey]*ingressPeerBatch
	lg      loggateway.Logger
}

// NewIngressPeerDebouncer creates a new debouncer with the given delay.
func NewIngressPeerDebouncer(delay time.Duration, lg loggateway.Logger) *IngressPeerDebouncer {
	if delay <= 0 {
		delay = DefaultIngressDebounce
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &IngressPeerDebouncer{
		delay:   delay,
		pending: make(map[ingressPeerKey]*ingressPeerBatch),
		lg:      lg,
	}
}

// Submit implements PeerDebouncer.
func (b *IngressPeerDebouncer) Submit(ctx context.Context, channelID, peerID, peerKey, text, idempotencyKey string, run InboundProcessFunc) {
	if b == nil || run == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		if err := run(ctx); err != nil {
			b.lg.Warn("ingress debounce immediate run failed", loggateway.StepID("ingress.debounce.run"), loggateway.Err(err))
		}
		return
	}
	peerKey = strings.TrimSpace(peerKey)
	if peerKey == "" {
		peerKey = strings.TrimSpace(peerID)
	}
	key := ingressPeerKey{channelID: channelID, peerKey: peerKey}

	b.mu.Lock()
	defer b.mu.Unlock()
	if cur, ok := b.pending[key]; ok {
		parts := []string{strings.TrimSpace(cur.text), text}
		cur.text = strings.TrimSpace(strings.Join(parts, "\n"))
		cur.keys = append(cur.keys, strings.TrimSpace(idempotencyKey))
		cur.peerID = peerID
		if cur.timer != nil {
			cur.timer.Stop()
		}
		cur.timer = time.AfterFunc(b.delay, func() {
			b.flush(context.WithoutCancel(ctx), key, run)
		})
		return
	}
	keys := []string{strings.TrimSpace(idempotencyKey)}
	entry := &ingressPeerBatch{
		channelID: channelID,
		peerID:    peerID,
		peerKey:   peerKey,
		text:      text,
		keys:      keys,
	}
	entry.timer = time.AfterFunc(b.delay, func() {
		b.flush(context.WithoutCancel(ctx), key, run)
	})
	b.pending[key] = entry
}

func (b *IngressPeerDebouncer) flush(ctx context.Context, key ingressPeerKey, run InboundProcessFunc) {
	b.mu.Lock()
	entry, ok := b.pending[key]
	if !ok {
		b.mu.Unlock()
		return
	}
	delete(b.pending, key)
	if entry.timer != nil {
		entry.timer.Stop()
	}
	b.mu.Unlock()
	if err := run(ctx); err != nil {
		b.lg.Warn("ingress debounce flush run failed", loggateway.StepID("ingress.debounce.flush"), loggateway.Err(err))
	}
}
