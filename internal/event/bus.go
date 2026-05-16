package event

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Bus interface {
	Publish(ctx context.Context, envelope Envelope)
	Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
	DropCount() uint64
}

type SubscribeOptions struct {
	SessionID   string
	TeamID      string
	Channel     string
	FilterKey   string
	EventTypes  []EnvelopeType
	LevelFilter string
	BufferSize  int
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

func NewBus() Bus {
	return &bus{
		subscribers: make(map[uint64]*subscriber),
	}
}

func reliableTypes() map[EnvelopeType]struct{} {
	return map[EnvelopeType]struct{}{
		EnvelopeTypeToolResult:      {},
		EnvelopeTypeError:           {},
		EnvelopeTypeRunnerCompletion: {},
		EnvelopeTypeGraphNodeEnd:    {},
		EnvelopeTypeTeamRunFinished: {},
		EnvelopeTypeTeamRunFailed:   {},
	}
}

func (b *bus) Publish(ctx context.Context, env Envelope) {
	if env.Channel == "" {
		env.Channel = RouteChannel(env)
	}
	_, isReliable := reliableTypes()[env.Type]

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subscribers {
		if !b.matchSubscriber(sub.opts, env) {
			continue
		}
		if isReliable {
			b.deliverReliable(sub, env)
		} else {
			b.deliverLossy(sub, env)
		}
	}
}

func (b *bus) deliverReliable(sub *subscriber, env Envelope) {
	select {
	case sub.ch <- env:
		return
	default:
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case sub.ch <- env:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	b.dropCount.Add(1)
}

func (b *bus) deliverLossy(sub *subscriber, env Envelope) {
	select {
	case sub.ch <- env:
	default:
		select {
		case <-sub.ch:
			select {
			case sub.ch <- env:
			default:
				b.dropCount.Add(1)
			}
		default:
		}
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
