package event

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/event/contract"
	arametrics "aranea-agents/internal/metrics"
)

// Re-export contract types for backward compatibility.
type (
	DropPolicy       = contract.DropPolicy
	ChannelPriority  = contract.ChannelPriority
	SubscribeOptions = contract.SubscribeOptions
	Bus              = contract.Bus
)

const (
	DropOldest              = contract.DropOldest
	DropNewest              = contract.DropNewest
	BlockUpTo               = contract.BlockUpTo
	ChannelPriorityCritical = contract.ChannelPriorityCritical
	ChannelPriorityNormal   = contract.ChannelPriorityNormal
)

type bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber
	nextID      uint64
	dropCount   atomic.Uint64
}

type subscriber struct {
	mu     sync.RWMutex
	ch     chan Envelope
	opts   SubscribeOptions
	closed bool
}

// NewBus returns a new in-process event bus.
func NewBus() Bus {
	return &bus{
		subscribers: make(map[uint64]*subscriber),
	}
}

// criticalTypes returns the set of event types that must never be silently dropped.
// These are persisted session events where loss causes observable data corruption.
func criticalTypes() map[EnvelopeType]struct{} {
	return map[EnvelopeType]struct{}{
		EnvelopeTypeToolResult:       {},
		EnvelopeTypeError:            {},
		EnvelopeTypeRunnerCompletion: {},
		EnvelopeTypeContextUsage:     {},
		EnvelopeTypeGraphNodeEnd:     {},
		EnvelopeTypeTeamRunFinished:  {},
		EnvelopeTypeTeamRunFailed:    {},
	}
}

func (b *bus) Publish(ctx context.Context, env Envelope) {
	if env.Channel == "" {
		env.Channel = RouteChannel(env)
	}
	arametrics.EventBusPublished.WithLabelValues(string(env.Type)).Inc()
	_, isCritical := criticalTypes()[env.Type]

	b.mu.RLock()
	criticalSubs := make([]*subscriber, 0, len(b.subscribers))
	normalSubs := make([]*subscriber, 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		if sub.opts.Priority == ChannelPriorityCritical {
			criticalSubs = append(criticalSubs, sub)
		} else {
			normalSubs = append(normalSubs, sub)
		}
	}
	b.mu.RUnlock()

	for _, sub := range criticalSubs {
		b.deliverToSubscriber(sub, env, isCritical)
	}
	for _, sub := range normalSubs {
		b.deliverToSubscriber(sub, env, isCritical)
	}
}

func (b *bus) deliverToSubscriber(sub *subscriber, env Envelope, isCritical bool) {
	if !b.matchSubscriber(sub.opts, env) {
		return
	}
	if sub.opts.Selector != nil && !sub.opts.Selector(env.Type) {
		return
	}

	policy := sub.opts.DropPolicy
	blockFor := sub.opts.BlockFor

	if sub.opts.Reliable || isCritical {
		policy = BlockUpTo
		if blockFor <= 0 {
			blockFor = 100 * time.Millisecond
		}
	}

	switch policy {
	case BlockUpTo:
		b.deliverBlockUpTo(sub, env, blockFor)
	case DropNewest:
		b.deliverDropNewest(sub, env)
	default:
		b.deliverDropOldest(sub, env)
	}
}

func (b *bus) deliverBlockUpTo(sub *subscriber, env Envelope, blockFor time.Duration) {
	sub.mu.RLock()
	defer sub.mu.RUnlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- env:
		return
	default:
	}
	if blockFor <= 0 {
		blockFor = 100 * time.Millisecond
	}
	timer := time.NewTimer(blockFor)
	defer timer.Stop()
	for {
		select {
		case sub.ch <- env:
			return
		case <-timer.C:
			b.deliverDropOldestLocked(sub, env)
			return
		}
	}
}

func (b *bus) deliverDropOldest(sub *subscriber, env Envelope) {
	sub.mu.RLock()
	defer sub.mu.RUnlock()
	if sub.closed {
		return
	}
	b.deliverDropOldestLocked(sub, env)
}

func (b *bus) deliverDropOldestLocked(sub *subscriber, env Envelope) {
	select {
	case sub.ch <- env:
	default:
		select {
		case <-sub.ch:
			select {
			case sub.ch <- env:
			default:
				b.dropCount.Add(1)
				arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_oldest").Inc()
				SessionSysLogWarn(context.Background(), env.SessionID, "system.bus.drop", "事件总线丢弃消息（drop_oldest）",
					P("type", string(env.Type)), P("channel", env.Channel), P("policy", "drop_oldest"), P("total_drops", b.dropCount.Load()))
			}
		default:
			b.dropCount.Add(1)
			arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_oldest").Inc()
			SessionSysLogWarn(context.Background(), env.SessionID, "system.bus.drop", "事件总线丢弃消息（drop_oldest）",
				P("type", string(env.Type)), P("channel", env.Channel), P("policy", "drop_oldest"), P("total_drops", b.dropCount.Load()))
		}
	}
}

func (b *bus) deliverDropNewest(sub *subscriber, env Envelope) {
	sub.mu.RLock()
	defer sub.mu.RUnlock()
	if sub.closed {
		return
	}
	select {
	case sub.ch <- env:
	default:
		b.dropCount.Add(1)
		arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_newest").Inc()
		SessionSysLogWarn(context.Background(), env.SessionID, "system.bus.drop", "事件总线丢弃消息（drop_newest）",
			P("type", string(env.Type)), P("channel", env.Channel), P("policy", "drop_newest"), P("total_drops", b.dropCount.Load()))
	}
}

func (b *bus) DropCount() uint64 {
	return b.dropCount.Load()
}

func (b *bus) Subscribe(opts SubscribeOptions) (<-chan Envelope, func()) {
	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = 128
	}
	if bufSize > 512 {
		bufSize = 512
	}
	ch := make(chan Envelope, bufSize)
	id := atomic.AddUint64(&b.nextID, 1)
	b.mu.Lock()
	b.subscribers[id] = &subscriber{ch: ch, opts: opts}
	b.mu.Unlock()
	unsubscribe := func() {
		b.mu.Lock()
		sub, ok := b.subscribers[id]
		if !ok {
			b.mu.Unlock()
			return
		}
		delete(b.subscribers, id)
		b.mu.Unlock()

		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
		}
		sub.mu.Unlock()
	}
	return ch, unsubscribe
}

func (b *bus) matchSubscriber(opts SubscribeOptions, env Envelope) bool {
	if opts.SessionID != "" && opts.SessionID != env.SessionID {
		return false
	}
	if opts.TeamID != "" && opts.TeamID != env.TeamID {
		return false
	}
	if opts.Channel != "" && opts.Channel != env.Channel {
		return false
	}
	if opts.FilterKey != "" && !MatchFilterKey(opts.FilterKey, env.FilterKey) {
		return false
	}
	if len(opts.EventTypes) > 0 {
		found := false
		for _, t := range opts.EventTypes {
			if t == env.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if opts.LevelFilter != "" && env.Type == EnvelopeTypeLog {
		level, _ := env.Metadata["level"].(string)
		if !matchLevelFilter(opts.LevelFilter, level) {
			return false
		}
	}
	return true
}

func matchLevelFilter(filter, level string) bool {
	if filter == "" || level == "" {
		return true
	}
	levelOrder := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}
	minLevel, ok := levelOrder[strings.ToUpper(filter)]
	if !ok {
		for _, f := range strings.Split(filter, "|") {
			if strings.EqualFold(strings.TrimSpace(f), level) {
				return true
			}
		}
		return false
	}
	eventLevel, ok := levelOrder[strings.ToUpper(level)]
	if !ok {
		return true
	}
	return eventLevel >= minLevel
}
