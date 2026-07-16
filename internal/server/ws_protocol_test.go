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
		v2Bus,
		nil,
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
			notice, ok := e.(*biz.SystemNoticeEvent)
			if !ok {
				continue
			}
			if notice.NoticeType != "ws_error" {
				continue
			}
			if got, _ := notice.Meta["request_id"].(string); got != "pending-user-abc" {
				t.Fatalf("request_id = %q, want pending-user-abc", got)
			}
			if got, _ := notice.Meta["error_type"].(string); got != "send_failed" {
				t.Fatalf("error_type = %q, want send_failed", got)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for system.notice ws_error")
		}
	}
}

func TestWSUpstreamTurnGatewayErrorPublishesEnvelope(t *testing.T) {
	// publishWSErrorActivity publishes system.notice (ws_error) plus synthetic
	// Task/Turn/Step failed events on the v2 EventBus.
	v2Bus := event.NewV2Bus()
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil,
		stubChatSender{},
		stubTurnExecutor{err: errors.New("provider raw error")},
		nil,
		loggateway.NewNoop(),
		v2Bus,
		nil,
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

	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-ch:
			notice, ok := e.(*biz.SystemNoticeEvent)
			if !ok {
				continue
			}
			if notice.NoticeType != "ws_error" {
				continue
			}
			if notice.SpiritSessionID() != "sess-turn" {
				t.Fatalf("expected sessionID=sess-turn, got %s", notice.SpiritSessionID())
			}
			if got, _ := notice.Meta["request_id"].(string); got != "pending-user-abc" {
				t.Fatalf("expected requestID=pending-user-abc, got %s", got)
			}
			if got, _ := notice.Meta["error_type"].(string); got != "send_failed" {
				t.Fatalf("expected error_type=send_failed, got %s", got)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for system.notice ws_error from turn gateway failure")
		}
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
		Payload:   map[string]any{"content": "hi"},
	})
	srv.handleUpstream(wc, raw)
	time.Sleep(50 * time.Millisecond)
}

// TestWSUpstreamErrorPublishesSyntheticV2Failure verifies that
// publishWSErrorActivity (P2-05 fix) emits the synthetic v2 Task/Turn/Step
// failure events so the v2 timeline can render a stable ErrorBlock.
//
// Without this, WS send failures only flow through the v1 bridge path and
// the v2-only ActivityStream has no entity to render.
func TestWSUpstreamErrorPublishesSyntheticV2Failure(t *testing.T) {
	v2Bus := event.NewV2Bus()
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil,
		stubChatSender{sendErr: context.Canceled},
		nil,
		nil,
		loggateway.NewNoop(),
		v2Bus,
		nil,
	)
	wc := &wsConn{
		sessionID: "sess-syn",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 2),
	}
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		RequestID: "req-syn-1",
		Payload:   map[string]any{"content": "hello"},
	})
	srv.handleUpstream(wc, raw)

	// Collect events published within 2 seconds. Expect at least:
	//   1 *ActivityBridgeEvent (v1 compat)
	//   1 *TaskFailedEvent (synthetic)
	//   1 *TurnFailedEvent (synthetic)
	//   1 *StepCreatedEvent (synthetic error Step)
	//   1 *StepCompletedEvent (synthetic error Step)
	deadline := time.After(2 * time.Second)
	var gotTaskFailed, gotTurnFailed, gotStepCreated, gotStepCompleted bool
	var synTaskID, synTurnID, synStepID string
	var synTask *biz.TaskFailedEvent
	var synTurn *biz.TurnFailedEvent
	var synStepCreated *biz.StepCreatedEvent
	var synStepCompleted *biz.StepCompletedEvent
	for !gotTaskFailed || !gotTurnFailed || !gotStepCreated || !gotStepCompleted {
		select {
		case e := <-ch:
			switch ev := e.(type) {
			case *biz.TaskFailedEvent:
				gotTaskFailed = true
				synTask = ev
				synTaskID = ev.Task.ID
			case *biz.TurnFailedEvent:
				gotTurnFailed = true
				synTurn = ev
				synTurnID = ev.Turn.ID
			case *biz.StepCreatedEvent:
				gotStepCreated = true
				synStepCreated = ev
				synStepID = ev.Step.ID
			case *biz.StepCompletedEvent:
				gotStepCompleted = true
				synStepCompleted = ev
			}
		case <-deadline:
			t.Fatalf("timeout — got taskFailed=%v turnFailed=%v stepCreated=%v stepCompleted=%v",
				gotTaskFailed, gotTurnFailed, gotStepCreated, gotStepCompleted)
		}
	}

	// Verify synthetic ID consistency: Task.ID, Turn.ID (Task + "-turn"),
	// Step.ID (Turn + "-s1").
	const wantTaskID = "ws-err-req-syn-1"
	if synTaskID != wantTaskID {
		t.Errorf("synthetic Task.ID = %q, want %q", synTaskID, wantTaskID)
	}
	const wantTurnID = wantTaskID + "-turn"
	if synTurnID != wantTurnID {
		t.Errorf("synthetic Turn.ID = %q, want %q", synTurnID, wantTurnID)
	}
	const wantStepID = wantTurnID + "-s1"
	if synStepID != wantStepID {
		t.Errorf("synthetic Step.ID = %q, want %q", synStepID, wantStepID)
	}

	// Verify Task fields.
	if synTask.Task.SessionID != "sess-syn" {
		t.Errorf("synthetic Task.SessionID = %q, want sess-syn", synTask.Task.SessionID)
	}
	if synTask.Task.Status != biz.TaskStatusFailed {
		t.Errorf("synthetic Task.Status = %q, want failed", synTask.Task.Status)
	}
	if synTask.Task.Version != 1 {
		t.Errorf("synthetic Task.Version = %d, want 1", synTask.Task.Version)
	}
	if synTask.Task.CompletedAt == nil {
		t.Error("synthetic Task.CompletedAt should be non-nil")
	}

	// Verify Turn fields.
	if synTurn.Turn.TaskID != wantTaskID {
		t.Errorf("synthetic Turn.TaskID = %q, want %q", synTurn.Turn.TaskID, wantTaskID)
	}
	if synTurn.Turn.SessionID != "sess-syn" {
		t.Errorf("synthetic Turn.SessionID = %q, want sess-syn", synTurn.Turn.SessionID)
	}
	if synTurn.Turn.SpiritSessionID != "sess-syn" {
		t.Errorf("synthetic Turn.SpiritSessionID = %q, want sess-syn", synTurn.Turn.SpiritSessionID)
	}
	if synTurn.Turn.Status != biz.TurnStatusFailed {
		t.Errorf("synthetic Turn.Status = %q, want failed", synTurn.Turn.Status)
	}

	// Verify Step fields (Kind=error carries error details for the frontend).
	if synStepCreated.Step.TurnID != wantTurnID {
		t.Errorf("synthetic Step.TurnID = %q, want %q", synStepCreated.Step.TurnID, wantTurnID)
	}
	if synStepCreated.Step.TaskID != wantTaskID {
		t.Errorf("synthetic Step.TaskID = %q, want %q", synStepCreated.Step.TaskID, wantTaskID)
	}
	if synStepCreated.Step.Kind != biz.StepKindError {
		t.Errorf("synthetic Step.Kind = %q, want error", synStepCreated.Step.Kind)
	}
	if synStepCreated.Step.Status != biz.StepStatusCompleted {
		t.Errorf("synthetic Step.Status = %q, want completed", synStepCreated.Step.Status)
	}
	// The Content should carry the error message (from stubChatSender.sendErr
	// = context.Canceled → message "context canceled" comes from send_failed
	// error type + err.Error()).
	if synStepCreated.Step.Content == "" {
		t.Error("synthetic Step.Content should be non-empty (carries error message)")
	}
	if synStepCreated.Step.ToolErrorCode != "send_failed" {
		t.Errorf("synthetic Step.ToolErrorCode = %q, want send_failed", synStepCreated.Step.ToolErrorCode)
	}

	// StepCompleted event should carry the same Step (same ID, same content).
	if synStepCompleted.Step.ID != synStepCreated.Step.ID {
		t.Errorf("StepCompleted.Step.ID = %q, want %q (same as StepCreated)",
			synStepCompleted.Step.ID, synStepCreated.Step.ID)
	}
}
