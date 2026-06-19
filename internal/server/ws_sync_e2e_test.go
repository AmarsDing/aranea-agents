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
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
)

// e2eSyncChatSender simulates a ChatService that persists envelopes to EventStore
// and publishes them to the event bus, mimicking production WBPF behavior.
type e2eSyncChatSender struct {
	bus      event.Bus
	repo     biz.EventStoreRepo
	session  string
	revision int64
}

func (s *e2eSyncChatSender) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	s.revision++
	runID := "run-sync-e2e"

	// run_status=running envelope (persisted + published)
	running := event.NewEnvelope(contract.EnvelopeTypeRunStatus, "e2e-sync", s.session)
	running.Channel = "chat"
	running.Metadata = map[string]any{"status": "running", "run_id": runID}
	running.SessionRevision = s.revision
	s.persistAndPublish(ctx, running)

	// text_delta envelope (persisted + published)
	delta := event.NewEnvelope(contract.EnvelopeTypeTextDelta, "e2e-sync-agent", s.session)
	delta.Channel = "chat"
	delta.InvocationID = runID
	delta.Content = &event.EnvelopeContent{Text: "Hello from sync E2E", IsPartial: true}
	s.revision++
	delta.SessionRevision = s.revision
	s.persistAndPublish(ctx, delta)

	// run_status=completed envelope (persisted + published)
	done := event.NewEnvelope(contract.EnvelopeTypeRunStatus, "e2e-sync", s.session)
	done.Channel = "chat"
	done.Metadata = map[string]any{"status": "completed", "run_id": runID}
	s.revision++
	done.SessionRevision = s.revision
	s.persistAndPublish(ctx, done)

	return &chatv1.SendChatMessageResponse{}, nil
}

func (s *e2eSyncChatSender) persistAndPublish(ctx context.Context, env contract.Envelope) {
	data, _ := json.Marshal(env)
	_ = s.repo.Insert(ctx, biz.EventStoreRecord{
		ID:            env.ID,
		SessionID:     s.session,
		Type:          string(env.Type),
		Channel:       env.Channel,
		EnvelopeJSON:  string(data),
		CreatedAt:     time.Now().UTC(),
	})
	s.bus.Publish(ctx, env)
}

func (s *e2eSyncChatSender) EnqueueUserMessage(context.Context, *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
}

// TestWSE2E_SyncRequestReplayAfterReconnect verifies the full T3.4 flow:
// 1. Client connects, sends user_message, receives streamed envelopes
// 2. Client disconnects (simulating network drop)
// 3. Client reconnects
// 4. Client sends sync_request with after_revision
// 5. Server replays missed envelopes from EventStore
func TestWSE2E_SyncRequestReplayAfterReconnect(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "true")
	t.Setenv("DEPLOY_ENV", "dev")

	const sessionID = "sess-sync-e2e"
	bus := event.NewBus(nil)
	repo := &fakeEventStoreRepo{}
	sender := &e2eSyncChatSender{bus: bus, repo: repo, session: sessionID}

	// Build server with EventStoreUsecase wired (T3.4).
	uc := biz.NewEventStoreUsecase(repo)
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{SessionBus: bus, MonitorBus: bus, Buffer: event.NewBuffer()},
		nil, sender, nil, nil, loggateway.NewNoop(), uc,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", srv.handleWS)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/v1/ws?session_id=" + sessionID

	// Phase 1: Connect and send user_message, receive streamed envelopes.
	conn1, resp1, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 1: %v", err)
	}
	defer conn1.Close()
	defer resp1.Body.Close()

	// Read connected message.
	connected := readWSDownstream(t, conn1, 3*time.Second)
	if connected.Type != "connected" {
		t.Fatalf("expected connected, got %+v", connected)
	}

	// Send user_message to trigger envelope publishing.
	up, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "chat",
		Type:      "user_message",
		RequestID: "req-sync-1",
		Payload:   map[string]any{"content": "hi"},
	})
	if err := conn1.WriteMessage(websocket.TextMessage, up); err != nil {
		t.Fatal(err)
	}

	// Collect streamed envelopes and track the max revision received.
	maxRevision := int64(0)
	streamedTypes := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (!streamedTypes["run_status"] || !streamedTypes["text_delta"]) {
		msg := readWSDownstream(t, conn1, time.Until(deadline))
		if msg.Type == "connected" || msg.Type == "pong" {
			continue
		}
		if msg.Envelope != nil {
			streamedTypes[string(msg.Envelope.Type)] = true
			if msg.Envelope.SessionRevision > maxRevision {
				maxRevision = msg.Envelope.SessionRevision
			}
		}
	}
	if maxRevision == 0 {
		t.Fatalf("expected non-zero revision from streamed envelopes, got %d", maxRevision)
	}

	// Phase 2: Disconnect (simulate network drop).
	conn1.Close()

	// Phase 3: Reconnect.
	conn2, resp2, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial 2: %v", err)
	}
	defer conn2.Close()
	defer resp2.Body.Close()

	// Read connected message.
	connected2 := readWSDownstream(t, conn2, 3*time.Second)
	if connected2.Type != "connected" {
		t.Fatalf("expected connected on reconnect, got %+v", connected2)
	}

	// Phase 4: Send sync_request with after_revision = 1 (replay revisions > 1).
	// In production, the client tracks the max revision seen and sends after_revision = maxRevision.
	// Here we use after_revision = 1 to replay envelopes with revision 2 and 3.
	syncReq, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     sessionID,
			"after_revision": float64(1), // replay envelopes with revision > 1
		},
	})
	if err := conn2.WriteMessage(websocket.TextMessage, syncReq); err != nil {
		t.Fatal(err)
	}

	// Phase 5: Verify replayed envelopes arrive.
	replayedTypes := map[string]int{}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := readWSDownstreamNoFatal(t, conn2, time.Until(deadline))
		if err != nil {
			break
		}
		if msg.Type == "connected" || msg.Type == "pong" {
			continue
		}
		if msg.Type == "replay" && msg.Envelope != nil {
			replayedTypes[string(msg.Envelope.Type)]++
		}
	}

	// Expect at least 1 run_status and 1 text_delta replayed.
	if replayedTypes["run_status"] < 1 {
		t.Errorf("expected at least 1 run_status replay, got %d", replayedTypes["run_status"])
	}
	if replayedTypes["text_delta"] < 1 {
		t.Errorf("expected at least 1 text_delta replay, got %d", replayedTypes["text_delta"])
	}
}

// TestWSE2E_SyncRequestNoEventStoreIsNoop verifies that when EventStoreUsecase
// is not configured (nil), sync_request is a no-op (no crash, no replay).
func TestWSE2E_SyncRequestNoEventStoreIsNoop(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "true")
	t.Setenv("DEPLOY_ENV", "dev")

	const sessionID = "sess-noop"
	bus := event.NewBus(nil)
	sender := &e2eChatSender{bus: bus, sessionID: sessionID}
	// Note: EventStoreUsecase is nil (last param).
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{SessionBus: bus, MonitorBus: bus, Buffer: event.NewBuffer()},
		nil, sender, nil, nil, loggateway.NewNoop(), nil,
	)

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

	// Send sync_request — should be a no-op.
	syncReq, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     sessionID,
			"after_revision": float64(1),
		},
	})
	if err := conn.WriteMessage(websocket.TextMessage, syncReq); err != nil {
		t.Fatal(err)
	}

	// Verify no replay messages arrive within a short window.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		msg, err := readWSDownstreamNoFatal(t, conn, time.Until(deadline))
		if err != nil {
			break
		}
		if msg.Type == "replay" {
			t.Errorf("expected no replay when EventStore is nil, got %+v", msg)
		}
	}
}

// readWSDownstreamNoFatal is like readWSDownstream but returns error instead of fataling.
func readWSDownstreamNoFatal(t *testing.T, conn *websocket.Conn, wait time.Duration) (wsDownstream, error) {
	t.Helper()
	if wait <= 0 {
		wait = 200 * time.Millisecond
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return wsDownstream{}, err
	}
	var down wsDownstream
	if err := json.Unmarshal(raw, &down); err != nil {
		return wsDownstream{}, err
	}
	return down, nil
}
