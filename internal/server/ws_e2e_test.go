package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/activityevent"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
)

// e2eChatSender simulates ChatService: publishes created/streaming/completed
// ActivityEvents on turn start (AF pipeline).
type e2eChatSender struct {
	activityBus *activityevent.Bus
	sessionID   string
}

func (s *e2eChatSender) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	runID := "run-e2e-1"
	// Created: turn started (replaces legacy run_status running envelope).
	s.activityBus.Publish(ctx, biz.ActivityEvent{
		Event: biz.ActivityEventCreated,
		Activity: biz.Activity{
			ID:        "act-e2e-1",
			Kind:      biz.ActivityKindTask,
			Status:    biz.ActivityStatusRunning,
			SessionID: s.sessionID,
			Meta:      map[string]any{"status": "running", "run_id": runID},
		},
		Domain: biz.ActivityDomainChat,
	})

	// Streaming: text append (replaces legacy execution_progress envelope).
	s.activityBus.Publish(ctx, biz.ActivityEvent{
		Event:      biz.ActivityEventStreaming,
		DeltaField: "content",
		DeltaChunk: "Hello",
		Activity: biz.Activity{
			ID:        "act-e2e-1",
			Kind:      biz.ActivityKindReply,
			SessionID: s.sessionID,
			Content:   "Hello",
		},
		Domain: biz.ActivityDomainChat,
	})

	// Completed: turn done (replaces legacy run_status completed envelope).
	s.activityBus.Publish(ctx, biz.ActivityEvent{
		Event: biz.ActivityEventCompleted,
		Activity: biz.Activity{
			ID:        "act-e2e-1",
			Kind:      biz.ActivityKindTask,
			Status:    biz.ActivityStatusCompleted,
			SessionID: s.sessionID,
			Meta:      map[string]any{"status": "completed", "run_id": runID},
		},
		Domain: biz.ActivityDomainChat,
	})
	return &chatv1.SendChatMessageResponse{}, nil
}

func (s *e2eChatSender) EnqueueUserMessage(context.Context, *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
}

// TestWSE2E_UserMessageStream is a skeleton E2E: WS connect → user_message → streamed ActivityEvents.
func TestWSE2E_UserMessageStream(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "true")
	t.Setenv("DEPLOY_ENV", "dev")

	const sessionID = "sess-e2e"
	activityBus := activityevent.New(loggateway.NewNoop())
	sender := &e2eChatSender{activityBus: activityBus, sessionID: sessionID}
	srv := newTestWSServerWithActivity(nil, sender, activityBus)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", srv.handleWS)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/v1/ws?session_id=" + sessionID
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	defer resp.Body.Close()

	connected := readWSDownstream(t, conn, 3*time.Second)
	if connected.Type != "connected" {
		t.Fatalf("expected connected, got %+v", connected)
	}

	up, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		RequestID: "req-e2e-1",
		Payload: map[string]any{
			"content": "hi",
		},
	})
	if err := conn.WriteMessage(websocket.TextMessage, up); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(seen) < 2 {
		msg := readWSDownstream(t, conn, time.Until(deadline))
		if msg.Type == "connected" || msg.Type == "pong" {
			continue
		}
		typ := wsMessageType(msg)
		switch typ {
		case string(biz.ActivityEventCreated):
			seen["created"] = true
		case string(biz.ActivityEventStreaming):
			seen["streaming"] = true
		}
	}
	if !seen["created"] || !seen["streaming"] {
		t.Fatalf("expected created and streaming events, seen=%v", seen)
	}
}

func wsMessageType(msg wsDownstream) string {
	if msg.ActivityEvent != nil {
		return string(msg.ActivityEvent.Event)
	}
	return msg.Type
}

func readWSDownstream(t *testing.T, conn *websocket.Conn, wait time.Duration) wsDownstream {
	t.Helper()
	if wait <= 0 {
		wait = time.Second
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var down wsDownstream
	if err := json.Unmarshal(raw, &down); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return down
}
