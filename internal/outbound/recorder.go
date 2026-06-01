package outbound

import (
	"context"
	"strings"
	"sync"
)

type SentTextRecorder struct {
	mu      sync.RWMutex
	sent    map[sentTextKey]struct{}
	targets map[sentTargetKey]struct{}
}

type sentTextKey struct {
	Channel string
	Target  string
	Text    string
}

type sentTargetKey struct {
	Channel string
	Target  string
}

func NewSentTextRecorder() *SentTextRecorder {
	return &SentTextRecorder{
		sent:    make(map[sentTextKey]struct{}),
		targets: make(map[sentTargetKey]struct{}),
	}
}

type sentTextRecorderContextKey struct{}

func WithSentTextRecorder(ctx context.Context, recorder *SentTextRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, sentTextRecorderContextKey{}, recorder)
}

func (r *SentTextRecorder) Record(target DeliveryTarget, text string) {
	key, ok := sentTextKeyFor(target, text)
	if r == nil || !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sent == nil {
		r.sent = make(map[sentTextKey]struct{})
	}
	if r.targets == nil {
		r.targets = make(map[sentTargetKey]struct{})
	}
	r.sent[key] = struct{}{}
	r.targets[sentTargetKey{
		Channel: key.Channel,
		Target:  key.Target,
	}] = struct{}{}
}

func (r *SentTextRecorder) Contains(target DeliveryTarget, text string) bool {
	key, ok := sentTextKeyFor(target, text)
	if r == nil || !ok {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok = r.sent[key]
	return ok
}

func (r *SentTextRecorder) ContainsTarget(target DeliveryTarget) bool {
	key, ok := sentTargetKeyFor(target)
	if r == nil || !ok {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok = r.targets[key]
	return ok
}

func sentTextRecorderFromContext(ctx context.Context) (*SentTextRecorder, bool) {
	if ctx == nil {
		return nil, false
	}
	recorder, ok := ctx.Value(sentTextRecorderContextKey{}).(*SentTextRecorder)
	return recorder, ok && recorder != nil
}

func sentTextKeyFor(target DeliveryTarget, text string) (sentTextKey, bool) {
	targetKey, ok := sentTargetKeyFor(target)
	if !ok || text == "" {
		return sentTextKey{}, false
	}
	return sentTextKey{
		Channel: targetKey.Channel,
		Target:  targetKey.Target,
		Text:    text,
	}, true
}

func sentTargetKeyFor(target DeliveryTarget) (sentTargetKey, bool) {
	clean := sanitizeTarget(target)
	if clean.Channel == "" || clean.Target == "" {
		return sentTargetKey{}, false
	}
	return sentTargetKey{
		Channel: strings.TrimSpace(clean.Channel),
		Target:  strings.TrimSpace(clean.Target),
	}, true
}
