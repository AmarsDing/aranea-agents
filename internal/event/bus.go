package event

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	arametrics "aranea-agents/internal/metrics"
)

// DropPolicy controls what happens when a subscriber's buffer is full.
type DropPolicy int

const (
	// DropOldest evicts the oldest event in the buffer to make room for the new one.
	DropOldest DropPolicy = iota
	// DropNewest silently discards the new event when the buffer is full.
	DropNewest
	// BlockUpTo blocks for a configurable duration before falling back to DropOldest.
	BlockUpTo DropPolicy = 2
)

// SubscribeOptions configures a single Bus subscription.
type SubscribeOptions struct {
	// Routing filters — at least one must match for an event to be delivered.
	SessionID   string
	TeamID      string
	Channel     string
	FilterKey   string
	EventTypes  []EnvelopeType
	LevelFilter string

	// Backpressure controls.
	BufferSize int                     // channel capacity (default 128, capped at 512)
	Reliable   bool                    // shorthand: sets DropPolicy=BlockUpTo(100ms) for known critical types
	DropPolicy DropPolicy              // default DropOldest
	BlockFor   time.Duration           // used when DropPolicy=BlockUpTo (default 100ms if Reliable=true)
	Selector   func(EnvelopeType) bool // additional per-type filter (nil = accept all)
}

// Bus is the in-process event fanout hub.
type Bus interface {
	Publish(ctx context.Context, envelope Envelope)
	Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
	DropCount() uint64
}

type bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber
	nextID      uint64
	dropCount   atomic.Uint64
}

type subscriber struct {
	ch   chan Envelope
	opts SubscribeOptions
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
		EnvelopeTypeGraphNodeEnd:     {},
		EnvelopeTypeTeamRunFinished:  {},
		EnvelopeTypeTeamRunFailed:    {},
	}
}

func (b *bus) Publish(ctx context.Context, env Envelope) {
	if env.Channel == "" {
		env.Channel = RouteChannel(env)
	}
	arametrics.EventBusPublished.WithLabelValues(string(env.Type)).Inc() // EP-OBS-04
	_, isCritical := criticalTypes()[env.Type]

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subscribers {
		if !b.matchSubscriber(sub.opts, env) {
			continue
		}
		if sub.opts.Selector != nil && !sub.opts.Selector(env.Type) {
			continue
		}

		policy := sub.opts.DropPolicy
		blockFor := sub.opts.BlockFor

		// Reliable subscriptions or critical event types get block-up-to semantics.
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
		default: // DropOldest
			b.deliverDropOldest(sub, env)
		}
	}
}

func (b *bus) deliverBlockUpTo(sub *subscriber, env Envelope, blockFor time.Duration) {
	select {
	case sub.ch <- env:
		return
	default:
	}
	if blockFor <= 0 {
		blockFor = 100 * time.Millisecond
	}
	deadline := time.Now().Add(blockFor)
	for time.Now().Before(deadline) {
		select {
		case sub.ch <- env:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	// Deadline exceeded — fall back to evicting oldest.
	b.deliverDropOldest(sub, env)
}

func (b *bus) deliverDropOldest(sub *subscriber, env Envelope) {
	select {
	case sub.ch <- env:
	default:
		select {
		case <-sub.ch:
			select {
			case sub.ch <- env:
			default:
				b.dropCount.Add(1)
				arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_oldest").Inc() // EP-OBS-04
			}
		default:
			b.dropCount.Add(1)
			arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_oldest").Inc() // EP-OBS-04
		}
	}
}

func (b *bus) deliverDropNewest(sub *subscriber, env Envelope) {
	select {
	case sub.ch <- env:
	default:
		b.dropCount.Add(1)
		arametrics.EventBusDropped.WithLabelValues(string(env.Type), "drop_newest").Inc() // EP-OBS-04
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
		if _, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(ch)
		}
		b.mu.Unlock()
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
