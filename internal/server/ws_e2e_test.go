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
	"aranea-agents/internal/event"

	"github.com/gorilla/websocket"
)

// e2eChatSender simulates ChatService: publishes run_status + text_delta on turn start.
type e2eChatSender struct {
	bus       event.Bus
	sessionID string
}

func (s *e2eChatSender) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	runID := "run-e2e-1"
	running := event.NewEnvelope(event.EnvelopeTypeRunStatus, "e2e", s.sessionID)
	running.Metadata = map[string]any{"status": "running", "run_id": runID}
	s.bus.Publish(ctx, running)

	delta := event.NewEnvelope(event.EnvelopeTypeTextDelta, "e2e-agent", s.sessionID)
	delta.InvocationID = runID
	delta.Content = &event.EnvelopeContent{Text: "Hello", IsPartial: true}
	s.bus.Publish(ctx, delta)

	done := event.NewEnvelope(event.EnvelopeTypeRunStatus, "e2e", s.sessionID)
	done.Metadata = map[string]any{"status": "completed", "run_id": runID}
	s.bus.Publish(ctx, done)
	return &chatv1.SendChatMessageResponse{}, nil
}

func (s *e2eChatSender) EnqueueUserMessage(context.Context, *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
}

// TestWSE2E_UserMessageStream is a skeleton E2E: WS connect → user_message → streamed envelopes.
func TestWSE2E_UserMessageStream(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "true")
	t.Setenv("DEPLOY_ENV", "dev")

	const sessionID = "sess-e2e"
	bus := event.NewBus(nil)
	sender := &e2eChatSender{bus: bus, sessionID: sessionID}
	srv := newTestWSServer(bus, event.NewBuffer(), nil, sender)

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
		case "run_status":
			seen["run_status"] = true
		case "text_delta":
			seen["text_delta"] = true
		}
	}
	if !seen["run_status"] || !seen["text_delta"] {
		t.Fatalf("expected run_status and text_delta, seen=%v", seen)
	}
}

func wsMessageType(msg wsDownstream) string {
	if msg.Envelope != nil {
		return string(msg.Envelope.Type)
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
