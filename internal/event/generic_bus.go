package event

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	frameworkbus "trpc.group/trpc-go/trpc-agent-go/event/bus"
)

// GenericBus is a type-safe wrapper around frameworkbus.Bus[T].
// It allows creating buses for any event type without import cycles,
// enabling the ActivityEventBus (which transports biz.ActivityEvent)
// to be created in packages that import biz.
type GenericBus[T any] struct {
	inner frameworkbus.Bus[T]
	lg    loggateway.Logger
	// flowBus is the optional flow-log emission target for system.bus.drop.
	// It is the MonitorBus wrapping this GenericBus (two-phase construction
	// in newMonitorBus); nil in tests.
	flowBus contract.MonitorBus
	subs    atomic.Int64
	// dropFlowLast is the unix-nano timestamp of the last emitted
	// system.bus.drop flow log (throttle state).
	dropFlowLast atomic.Int64
}

// GenericSubscribeOptions configures a GenericBus subscription.
type GenericSubscribeOptions[T any] struct {
	BufferSize int
	Filter     func(T) bool
}

// NewGenericBus creates a new GenericBus for the given event type.
// flowBus is the MonitorBus used to emit system.bus.drop flow logs when the
// framework bus drops events due to back-pressure; pass nil to disable.
func NewGenericBus[T any](lg loggateway.Logger, flowBus contract.MonitorBus) *GenericBus[T] {
	b := &GenericBus[T]{
		lg:      lg,
		flowBus: flowBus,
	}
	var dropLogger frameworkbus.DropLogger[T]
	if lg != nil {
		dropLogger = func(event T, policy string, totalDrops uint64) {
			lg.Warn("[generic_bus] drop",
				loggateway.Str("policy", policy),
				loggateway.Int64("total_drops", int64(totalDrops)),
			)
			b.emitBusDropFlow(event, policy, totalDrops)
		}
	}
	opts := []frameworkbus.Option[T]{}
	if dropLogger != nil {
		opts = append(opts, frameworkbus.WithDropLogger(dropLogger))
	}
	b.inner = frameworkbus.New[T](opts...)
	return b
}

// busDropFlowInterval throttles system.bus.drop flow logs: at most one entry
// per bus per interval.
const busDropFlowInterval = 30 * time.Second

// emitBusDropFlow emits a throttled system.bus.drop flow log. The publish is
// async (safego) because the framework bus invokes the drop logger while
// holding the subscriber lock — a synchronous Publish back into the same bus
// would self-deadlock on that subscriber's RWMutex.
func (b *GenericBus[T]) emitBusDropFlow(ev T, policy string, totalDrops uint64) {
	if b.flowBus == nil {
		return
	}
	now := time.Now().UnixNano()
	last := b.dropFlowLast.Load()
	if last != 0 && now-last < int64(busDropFlowInterval) {
		return
	}
	if !b.dropFlowLast.CompareAndSwap(last, now) {
		return
	}
	sessionID := ""
	if me, ok := any(ev).(contract.MonitorEvent); ok {
		sessionID = me.SessionID
	}
	flowBus := b.flowBus
	lg := b.lg
	safego.Go(appctx.Ctx(), "generic_bus.drop_flow", func() {
		flow := NewTraceEmitterForRun(TraceEmitterOpts{
			Ctx:       context.Background(),
			SessionID: sessionID,
			Domain:    TraceDomainSystem,
			LG:        lg,
			Infra:     NewInfraFromBus(flowBus),
		})
		flow.LogWarn("system.bus.drop", "", "订阅者缓冲区已满，事件被丢弃",
			P("policy", policy),
			P("total_drops", totalDrops),
		)
	})
}

// Publish broadcasts an event to all matching subscribers.
func (b *GenericBus[T]) Publish(ctx context.Context, event T) {
	b.inner.Publish(ctx, event)
}

// Subscribe registers a subscriber and returns a channel of events.
func (b *GenericBus[T]) Subscribe(opts GenericSubscribeOptions[T]) (<-chan T, func()) {
	fwOpts := frameworkbus.SubscribeOptions[T]{
		BufferSize: opts.BufferSize,
	}
	if opts.Filter != nil {
		fwOpts.Filter = opts.Filter
	}
	ch, unsubscribe := b.inner.Subscribe(fwOpts)
	b.subs.Add(1)
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			unsubscribe()
			b.subs.Add(-1)
		})
	}
}

// DropCount returns the total number of dropped events.
func (b *GenericBus[T]) DropCount() uint64 {
	return b.inner.DropCount()
}

// SubscriberCount returns the number of active subscribers.
func (b *GenericBus[T]) SubscriberCount() int {
	return int(b.subs.Load())
}
