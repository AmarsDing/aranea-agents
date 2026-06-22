package lark

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

const (
	defaultInboundDebounce     = 600 * time.Millisecond
	defaultInboundLongDebounce = 2 * time.Second
	inboundLongTextRunes       = 4000
)

type inboundBatchKey struct {
	channelID string
	peerKey   string
}

type inboundBatchEntry struct {
	ch              biz.Channel
	ev              port.InboundEvent
	timer           *time.Timer
	longText        bool
	idempotencyKeys []string
}

// TextInboundBatcher merges consecutive Feishu text messages per peer (F-04).
type TextInboundBatcher struct {
	debounce     time.Duration
	longDebounce time.Duration
	lg           loggateway.Logger
	mu           sync.Mutex
	pending      map[inboundBatchKey]*inboundBatchEntry
}

func NewTextInboundBatcher(lg loggateway.Logger) *TextInboundBatcher {
	return &TextInboundBatcher{
		debounce:     defaultInboundDebounce,
		longDebounce: defaultInboundLongDebounce,
		lg:           lg,
		pending:      map[inboundBatchKey]*inboundBatchEntry{},
	}
}

func (b *TextInboundBatcher) flush(ctx context.Context, handler port.InboundHandler, key inboundBatchKey) {
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
	ev.IdempotencyKey = batchIdempotencyKey(entry.idempotencyKeys)
	b.mu.Unlock()
	if handler != nil {
		if err := handler.ProcessInbound(ctx, ch, ev); err != nil {
			b.lg.Warn("飞书入站处理失败",
				loggateway.StepID("channel.feishu.inbound_failed"),
				loggateway.Err(err),
			)
		}
	}
}

func (b *TextInboundBatcher) Submit(
	ctx context.Context,
	handler port.InboundHandler,
	ch biz.Channel,
	ev port.InboundEvent,
	lg loggateway.Logger,
) {
	if b == nil {
		if handler != nil {
			if err := handler.ProcessInbound(ctx, ch, ev); err != nil {
				lg.Warn("飞书入站处理失败",
					loggateway.StepID("channel.feishu.inbound_failed"),
					loggateway.Err(err),
				)
			}
		}
		return
	}
	if handler == nil {
		return
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		if err := handler.ProcessInbound(ctx, ch, ev); err != nil {
			b.lg.Warn("飞书入站处理失败",
				loggateway.StepID("channel.feishu.inbound_failed"),
				loggateway.Err(err),
			)
		}
		return
	}
	peerKey := strings.TrimSpace(ev.PeerKey)
	if peerKey == "" {
		peerKey = strings.TrimSpace(ev.PeerID)
	}
	key := inboundBatchKey{channelID: ch.ID, peerKey: peerKey}
	longText := len([]rune(text)) >= inboundLongTextRunes
	wait := b.debounce
	if longText {
		wait = b.longDebounce
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if cur, ok := b.pending[key]; ok {
		parts := []string{strings.TrimSpace(cur.ev.Text), text}
		cur.ev.Text = strings.TrimSpace(strings.Join(parts, "\n"))
		cur.idempotencyKeys = appendBatchIdempotencyKey(cur.idempotencyKeys, ev.IdempotencyKey)
		cur.ev.IdempotencyKey = batchIdempotencyKey(cur.idempotencyKeys)
		if longText {
			cur.longText = true
		}
		if cur.timer != nil {
			cur.timer.Stop()
		}
		delay := b.debounce
		if cur.longText {
			delay = b.longDebounce
		}
		cur.timer = time.AfterFunc(delay, func() {
			b.flush(context.WithoutCancel(ctx), handler, key)
		})
		return
	}
	evCopy := ev
	evCopy.Text = text
	keys := appendBatchIdempotencyKey(nil, ev.IdempotencyKey)
	evCopy.IdempotencyKey = batchIdempotencyKey(keys)
	entry := &inboundBatchEntry{ch: ch, ev: evCopy, longText: longText, idempotencyKeys: keys}
	entry.timer = time.AfterFunc(wait, func() {
		b.flush(context.WithoutCancel(ctx), handler, key)
	})
	b.pending[key] = entry
}
