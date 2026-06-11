package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

type ingressPeerKey struct {
	channelID string
	peerKey   string
}

type ingressPeerBatch struct {
	ch     biz.Channel
	ev     port.InboundEvent
	timer  *time.Timer
	keys   []string
}

type ingressPeerDebouncer struct {
	delay   time.Duration
	mu      sync.Mutex
	pending map[ingressPeerKey]*ingressPeerBatch
	lg      loggateway.Logger
}

func newIngressPeerDebouncer(delay time.Duration, lg loggateway.Logger) *ingressPeerDebouncer {
	if delay <= 0 {
		delay = defaultIngressDebounce
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ingressPeerDebouncer{
		delay:   delay,
		pending: make(map[ingressPeerKey]*ingressPeerBatch),
		lg:      lg,
	}
}

type ingressProcessFunc func(context.Context, biz.Channel, port.InboundEvent) error

// Submit implements biz.PeerDebouncer.
// It translates biz-level string parameters into concrete types for the internal debouncer.
func (b *ingressPeerDebouncer) Submit(ctx context.Context, channelID, peerID, peerKey, text, idempotencyKey string, run biz.InboundProcessFunc) {
	if b == nil || run == nil {
		return
	}
	ch := biz.Channel{ID: channelID}
	ev := port.InboundEvent{
		PeerID:         peerID,
		PeerKey:        peerKey,
		Text:           text,
		IdempotencyKey: idempotencyKey,
	}
	processFunc := func(ctx context.Context, _ biz.Channel, _ port.InboundEvent) error {
		return run(ctx)
	}
	b.submit(ctx, ch, ev, processFunc)
}

func (b *ingressPeerDebouncer) submit(ctx context.Context, ch biz.Channel, ev port.InboundEvent, run ingressProcessFunc) {
	if b == nil || run == nil {
		return
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		if err := run(ctx, ch, ev); err != nil {
			b.lg.Warn("ingress debounce immediate run failed", loggateway.StepID("ingress.debounce.run"), loggateway.Err(err))
		}
		return
	}
	peerKey := strings.TrimSpace(ev.PeerKey)
	if peerKey == "" {
		peerKey = strings.TrimSpace(ev.PeerID)
	}
	key := ingressPeerKey{channelID: ch.ID, peerKey: peerKey}

	b.mu.Lock()
	defer b.mu.Unlock()
	if cur, ok := b.pending[key]; ok {
		parts := []string{strings.TrimSpace(cur.ev.Text), text}
		cur.ev.Text = strings.TrimSpace(strings.Join(parts, "\n"))
		cur.keys = append(cur.keys, strings.TrimSpace(ev.IdempotencyKey))
		cur.ev.IdempotencyKey = biz.MergeIngressIdempotencyKeys(cur.keys)
		if cur.timer != nil {
			cur.timer.Stop()
		}
		cur.timer = time.AfterFunc(b.delay, func() {
			b.flush(context.WithoutCancel(ctx), key, run)
		})
		return
	}
	evCopy := ev
	evCopy.Text = text
	keys := []string{strings.TrimSpace(ev.IdempotencyKey)}
	evCopy.IdempotencyKey = biz.MergeIngressIdempotencyKeys(keys)
	entry := &ingressPeerBatch{ch: ch, ev: evCopy, keys: keys}
	entry.timer = time.AfterFunc(b.delay, func() {
		b.flush(context.WithoutCancel(ctx), key, run)
	})
	b.pending[key] = entry
}

func (b *ingressPeerDebouncer) flush(ctx context.Context, key ingressPeerKey, run ingressProcessFunc) {
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
	ch, ev := entry.ch, entry.ev
	b.mu.Unlock()
	if err := run(ctx, ch, ev); err != nil {
		b.lg.Warn("ingress debounce flush run failed", loggateway.StepID("ingress.debounce.flush"), loggateway.Err(err))
	}
}
