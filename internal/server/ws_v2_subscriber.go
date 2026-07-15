package server

import (
	"context"
	"encoding/json"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// WSV2Subscriber listens on a v2 EventBus and forwards events to WS clients
// subscribed to a given spirit_session_id.
//
// Each v2 Event is wrapped in a wsEnvelope{Type:"v2_event"} before being pushed
// to the hub. The v2 frontend will consume the envelope's payload directly;
// the legacy v1 frontend ignores events with type="v2_event" (it only
// recognizes v1 ActivityEvent shapes pushed via the existing activityEventPump).
//
// Phase 1: created and tested standalone (not wired via Wire). Production
// wiring will be done in a follow-up once V2Bus is injected into the Wire
// graph. See docs/superpowers/plans/2026-07-02-llm-activity-ordering-phase1.md
// Task 16.
type WSV2Subscriber struct {
	bus       biz.EventBus
	hub       WSMessageBroadcaster
	lg        loggateway.Logger
	cancel    context.CancelFunc
	cancelSub func()
	wg        sync.WaitGroup
}

// WSMessageBroadcaster is the minimal interface to push raw bytes to WS clients
// subscribed to a given spirit session.
// Implemented by *WSServer.BroadcastToSession and the test fakeHub.
type WSMessageBroadcaster interface {
	BroadcastToSession(spiritSessionID string, msg []byte)
	// BroadcastCriticalToSession delivers terminal lifecycle events on the
	// high-priority WS lane (BlockUpTo; close conn on timeout). B-06.
	BroadcastCriticalToSession(spiritSessionID string, msg []byte)
}

// NewWSV2Subscriber constructs and starts a subscriber goroutine.
// The caller must call Close to stop the goroutine and avoid leaks.
// A nil logger is replaced with a Noop logger.
//
// Subscribe is called synchronously in the constructor so that events
// published after NewWSV2Subscriber returns are not missed (avoids the
// pub/sub race where Publish fires before Subscribe registers).
func NewWSV2Subscriber(bus biz.EventBus, hub WSMessageBroadcaster, lg loggateway.Logger) *WSV2Subscriber {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// Subscribe synchronously so we don't miss events published between
	// constructor return and goroutine startup.
	ch, cancelSub := bus.Subscribe(biz.EventSubscribeOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	s := &WSV2Subscriber{
		bus:       bus,
		hub:       hub,
		lg:        lg.With(loggateway.Domain("ws_v2_subscriber")),
		cancel:    cancel,
		cancelSub: cancelSub,
	}
	s.wg.Add(1)
	go s.run(ctx, ch)
	return s
}

// run is the subscriber loop: drains the bus channel and forwards each event.
func (s *WSV2Subscriber) run(ctx context.Context, ch <-chan biz.Event) {
	defer s.wg.Done()
	defer s.cancelSub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			s.forward(e)
		}
	}
}

// forward serializes the v2 Event as JSON and pushes to the WS hub
// (broadcasting to clients subscribed to the event's SpiritSessionID).
func (s *WSV2Subscriber) forward(e biz.Event) {
	sessionID := e.SpiritSessionID()
	envelope := wsEnvelope{
		Type:      "v2_event",
		Kind:      string(e.EventKind()),
		SessionID: sessionID,
		Payload:   e,
	}
	msg, err := json.Marshal(envelope)
	if err != nil {
		s.lg.Warn("ws v2 marshal failed",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(err))
		return
	}
	// B-06: critical lifecycle events use the high-priority lane so they are
	// not DropNewest'd when the normal queue is saturated.
	if biz.IsCriticalDeliveryEvent(e) {
		s.hub.BroadcastCriticalToSession(sessionID, msg)
		return
	}
	s.hub.BroadcastToSession(sessionID, msg)
}

// Close stops the subscriber goroutine and waits for it to exit.
// Idempotent: safe to call multiple times (cancel + cancelSub are safe to call
// multiple times per context spec).
func (s *WSV2Subscriber) Close() error {
	s.cancel()
	s.cancelSub()
	s.wg.Wait()
	return nil
}

// wsEnvelope is the wire format for v2 events on the WS channel.
//
// SessionID is duplicated at the envelope root so global (session_id=*)
// subscribers can route without digging into unexported spiritSessionID
// fields inside the payload (E2E-P1-06).
//
// Phase 2 frontend will consume `payload` directly; v1 frontend will ignore
// events with `type == "v2_event"` (it only recognizes v1 ActivityEvent shapes
// pushed via the existing activityEventPump in ws_io_pump.go).
type wsEnvelope struct {
	Type      string `json:"type"`                 // "v2_event" or "v1_activity_event"
	Kind      string `json:"kind"`                 // EventKind value (e.g. "task.created")
	SessionID string `json:"session_id,omitempty"` // SpiritSessionID for routing
	Payload   any    `json:"payload"`              // the Event or ActivityEvent
}

// BroadcastToSession enqueues a raw message to all WS connections subscribed to
// the given session. Implements WSMessageBroadcaster for *WSServer.
//
// Non-terminal / system messages use the normal lane (DropNewest under load).
func (s *WSServer) BroadcastToSession(sessionID string, msg []byte) {
	if s == nil || s.store == nil {
		return
	}
	s.store.forEachConnForSession(sessionID, func(wc *wsConn) {
		wc.queues.enqueueSystem(msg)
		wc.wakeWriter()
	})
}

// BroadcastCriticalToSession delivers terminal lifecycle events on the high
// priority lane (BlockUpTo). If the high queue still times out, the connection
// is closed so the client reconnects and hydrates authoritative state (B-06).
func (s *WSServer) BroadcastCriticalToSession(sessionID string, msg []byte) {
	if s == nil || s.store == nil {
		return
	}
	cfg := s.wsConfig()
	s.store.forEachConnForSession(sessionID, func(wc *wsConn) {
		if ok := wc.queues.enqueue(cfg, wsPriorityHigh, msg); !ok {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 高优先级队列超时，关闭连接",
				loggateway.StepID("ws.critical_queue_timeout"))
			wc.closeSend()
			return
		}
		wc.wakeWriter()
	})
}
