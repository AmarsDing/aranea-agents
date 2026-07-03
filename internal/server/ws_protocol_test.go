package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func newTestWSServer(canceller RunCanceller, sender ChatSender) *WSServer {
	return NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		canceller, sender, nil, nil, loggateway.NewNoop(), nil, nil,
	)
}

// newTestWSServerWithActivity wires a server with a real ActivityEventBus so
// tests can subscribe to biz.ActivityEvent published by the WS handler.
func newTestWSServerWithActivity(canceller RunCanceller, sender ChatSender, activityBus biz.ActivityEventBus) *WSServer {
	return NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		canceller, sender, nil, nil, loggateway.NewNoop(), activityBus, nil,
	)
}

func TestCountGlobalMonitorConnsExcludesProbe(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	srv.store.conns["*"] = []*wsConn{
		{probeMode: true},
		{probeMode: false},
		{probeMode: false},
		{probeMode: false},
	}
	if n := srv.countGlobalMonitorConns(); n != 3 {
		t.Fatalf("expected 3 monitor conns, got %d", n)
	}
}

func TestWSUpstreamPingProducesPong(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"system": true},
		send:     make(chan []byte, 4),
		queues:   newConnQueues(conf.RuntimeWSConfig{}),
	}

	raw, err := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "ping",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.handleUpstream(wc, raw)

	select {
	case out := <-wc.queues.normal:
		var down wsDownstream
		if err := json.Unmarshal(out, &down); err != nil {
			t.Fatal(err)
		}
		if down.Type != "pong" || down.Direction != "server_to_client" {
			t.Fatalf("unexpected downstream: %+v", down)
		}
	default:
		t.Fatal("expected pong on send channel")
	}
}

func TestWSUpstreamSubscribeAddsChannel(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"chat": true, "system": true},
		send:     make(chan []byte, 1),
	}

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "subscribe",
		Payload:   map[string]any{"channel": "monitor"},
	})
	srv.handleUpstream(wc, raw)
	if !wc.channels["monitor"] {
		t.Fatal("expected monitor channel subscribed")
	}
}

func TestWSUpstreamCancelInvokesCanceller(t *testing.T) {
	canceller := &stubRunCanceller{}
	srv := newTestWSServer(canceller, nil)
	wc := &wsConn{
		sessionID: "sess-1",
		channels:  map[string]bool{"system": true},
		send:      make(chan []byte, 1),
	}

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "cancel",
	})
	srv.handleUpstream(wc, raw)

	if !canceller.called {
		t.Fatal("expected CancelRun to be invoked")
	}
}

func TestWSUpstreamUnsubscribeRemovesChannel(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"chat": true, "monitor": true, "system": true},
		send:     make(chan []byte, 1),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Type:      "unsubscribe",
		Payload:   map[string]any{"channel": "monitor"},
	})
	srv.handleUpstream(wc, raw)
	if wc.channels["monitor"] {
		t.Fatal("expected monitor channel removed")
	}
	if !wc.channels["chat"] || !wc.channels["system"] {
		t.Fatal("expected other channels kept")
	}
}

func TestWSUpstreamBadDirectionIgnored(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := &wsConn{
		channels: map[string]bool{"system": true},
		send:     make(chan []byte, 1),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "server_to_client",
		Type:      "ping",
	})
	srv.handleUpstream(wc, raw)
	select {
	case <-wc.send:
		t.Fatal("unexpected downstream for bad direction")
	default:
	}
}

func TestWSUpstreamEnqueueMessageAccepted(t *testing.T) {
	srv := newTestWSServer(nil, nil)
	wc := &wsConn{
		sessionID: "sess-enq",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "enqueue_message",
		Payload: map[string]any{
			"session_id": "sess-enq",
			"content":    "follow up",
		},
	})
	srv.handleUpstream(wc, raw)
	// Should not panic; may or may not send ack depending on gateway wiring.
}

type stubRunCanceller struct {
	called bool
}

func (s *stubRunCanceller) CancelRun(_ context.Context, _ string) bool {
	s.called = true
	return true
}

type stubChatSender struct {
	sendErr error
}

func (s stubChatSender) SendChatMessage(_ context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &chatv1.SendChatMessageResponse{}, nil
}

func (stubChatSender) EnqueueUserMessage(_ context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
}

type stubTurnExecutor struct {
	err error
}

func (s stubTurnExecutor) ExecuteTurn(_ context.Context, _ WSTurnInput) error {
	return s.err
}

func TestWSUpstreamUserMessagePublishesErrorWithRequestID(t *testing.T) {
	// Phase 3b-D: publishWSErrorActivity now uses v2 EventBus + bridge.
	v2Bus := event.NewV2Bus()
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil,
		stubChatSender{sendErr: context.Canceled},
		nil,
		nil,
		loggateway.NewNoop(),
		nil,
		v2Bus,
	)
	wc := &wsConn{
		sessionID: "sess-user",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
	}
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		RequestID: "pending-user-abc",
		Payload: map[string]any{
			"content": "hello",
		},
	})
	srv.handleUpstream(wc, raw)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			bridge, ok := e.(*biz.ActivityBridgeEvent)
			if !ok {
				continue
			}
			ev := bridge.Event
			if ev.Event != biz.ActivityEventFailed {
				continue
			}
			if got, _ := ev.Activity.Meta["request_id"].(string); got != "pending-user-abc" {
				t.Fatalf("request_id = %q, want pending-user-abc", got)
			}
			if got, _ := ev.Activity.Meta["error_type"].(string); got != "send_failed" {
				t.Fatalf("error_type = %q, want send_failed", got)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for ActivityBridgeEvent (Failed)")
		}
	}
}

func TestWSUpstreamTurnGatewayErrorPublishesEnvelope(t *testing.T) {
	// Phase 3b-D: publishWSErrorActivity now publishes ActivityBridgeEvent on
	// v2 EventBus. Subscribe to v2 EventBus and extract the v1 ActivityEvent
	// from the bridge to assert on the original payload (Kind/Status/Meta).
	v2Bus := event.NewV2Bus()
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil,
		stubChatSender{},
		stubTurnExecutor{err: errors.New("provider raw error")},
		nil,
		loggateway.NewNoop(),
		nil,
		v2Bus,
	)
	wc := &wsConn{
		sessionID: "sess-turn",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
		connCtx:   context.Background(),
	}
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		RequestID: "pending-user-abc",
		Payload: map[string]any{
			"content": "hello",
		},
	})
	srv.handleUpstream(wc, raw)

	select {
	case e := <-ch:
		bridge, ok := e.(*biz.ActivityBridgeEvent)
		if !ok {
			t.Fatalf("expected *ActivityBridgeEvent, got %T", e)
		}
		ev := bridge.Event
		if ev.Event != biz.ActivityEventFailed {
			t.Fatalf("expected ActivityEventFailed, got event=%s", ev.Event)
		}
		if ev.Activity.SessionID != "sess-turn" {
			t.Fatalf("expected sessionID=sess-turn, got %s", ev.Activity.SessionID)
		}
		if got, _ := ev.Activity.Meta["request_id"].(string); got != "pending-user-abc" {
			t.Fatalf("expected requestID=pending-user-abc, got %s", got)
		}
		if got, _ := ev.Activity.Meta["error_type"].(string); got != "send_failed" {
			t.Fatalf("expected error_type=send_failed, got %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ActivityBridgeEvent from turn gateway failure")
	}
}

func TestWSUpstreamUserMessageAccepted(t *testing.T) {
	srv := newTestWSServer(nil, stubChatSender{})
	wc := &wsConn{
		sessionID: "sess-ok",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		Payload:   map[string]any{"content": "hi"},
	})
	srv.handleUpstream(wc, raw)
	time.Sleep(50 * time.Millisecond)
}
