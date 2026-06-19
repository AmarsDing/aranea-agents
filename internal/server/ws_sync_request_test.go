package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// fakeEventStoreRepo is an in-memory EventStoreRepo for testing.
type fakeEventStoreRepo struct {
	records []biz.EventStoreRecord
}

func (r *fakeEventStoreRepo) Insert(_ context.Context, rec biz.EventStoreRecord) error {
	r.records = append(r.records, rec)
	return nil
}

func (r *fakeEventStoreRepo) List(_ context.Context, q biz.EventStoreQuery) (biz.EventStoreListResult, error) {
	var items []biz.EventStoreRecord
	for _, rec := range r.records {
		if rec.SessionID != q.SessionID {
			continue
		}
		if !q.Since.IsZero() && rec.CreatedAt.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && rec.CreatedAt.After(q.Until) {
			continue
		}
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		items = append(items, rec)
	}
	total := len(items)
	if q.Offset > 0 && q.Offset < total {
		items = items[q.Offset:]
	}
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}
	return biz.EventStoreListResult{Items: items, Total: total}, nil
}

func (r *fakeEventStoreRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (r *fakeEventStoreRepo) ExistsByID(_ context.Context, id string) bool {
	for _, rec := range r.records {
		if rec.ID == id {
			return true
		}
	}
	return false
}

// makeEnvelopeJSON builds a JSON envelope for testing.
func makeEnvelopeJSON(id, sessionID string, envType contract.EnvelopeType, revision int64) string {
	env := contract.Envelope{
		ID:              id,
		Type:            envType,
		SessionID:       sessionID,
		Channel:         "chat",
		SessionRevision: revision,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, _ := json.Marshal(env)
	return string(data)
}

func newSyncTestServer(t *testing.T, repo biz.EventStoreRepo) *WSServer {
	t.Helper()
	uc := biz.NewEventStoreUsecase(repo)
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{SessionBus: event.NewBus(nil), MonitorBus: event.NewBus(nil), Buffer: event.NewBuffer()},
		nil, nil, nil, nil, loggateway.NewNoop(), uc,
	)
	if srv == nil {
		t.Fatal("expected WSServer")
	}
	return srv
}

func TestHandleSyncRequest_ReplaysEnvelopesAfterRevision(t *testing.T) {
	repo := &fakeEventStoreRepo{}
	now := time.Now().UTC()
	// Populate with envelopes at revisions 1, 2, 3, 5.
	records := []biz.EventStoreRecord{
		{ID: "evt-1", SessionID: "sess-sync", Type: "run_status", Channel: "chat", EnvelopeJSON: makeEnvelopeJSON("evt-1", "sess-sync", contract.EnvelopeTypeRunStatus, 1), CreatedAt: now.Add(-3 * time.Hour)},
		{ID: "evt-2", SessionID: "sess-sync", Type: "run_status", Channel: "chat", EnvelopeJSON: makeEnvelopeJSON("evt-2", "sess-sync", contract.EnvelopeTypeRunStatus, 2), CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "evt-3", SessionID: "sess-sync", Type: "run_status", Channel: "chat", EnvelopeJSON: makeEnvelopeJSON("evt-3", "sess-sync", contract.EnvelopeTypeRunStatus, 3), CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "evt-5", SessionID: "sess-sync", Type: "run_status", Channel: "chat", EnvelopeJSON: makeEnvelopeJSON("evt-5", "sess-sync", contract.EnvelopeTypeRunStatus, 5), CreatedAt: now.Add(-30 * time.Minute)},
	}
	repo.records = records

	srv := newSyncTestServer(t, repo)
	wc := &wsConn{
		sessionID: "sess-sync",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 10),
		queues:    newConnQueues(conf.RuntimeWSConfig{}),
	}

	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     "sess-sync",
			"after_revision": float64(2),
		},
	})
	srv.handleUpstream(wc, raw)

	// Drain the normal queue and count replay messages.
	replayed := 0
	deadline := time.After(1 * time.Second)
	for {
		select {
		case data := <-wc.queues.normal:
			if data == nil {
				continue
			}
			var msg wsDownstream
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.Type == "replay" && msg.Envelope != nil {
				replayed++
			}
		case <-deadline:
			// Expect envelopes with revision 3 and 5 (revision > 2).
			if replayed != 2 {
				t.Fatalf("expected 2 replayed envelopes (revision > 2), got %d", replayed)
			}
			return
		}
	}
}

func TestHandleSyncRequest_NoEventStoreIsNoop(t *testing.T) {
	// WSServer with nil EventStoreUsecase — sync_request should be silently ignored.
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{SessionBus: event.NewBus(nil), MonitorBus: event.NewBus(nil), Buffer: event.NewBuffer()},
		nil, nil, nil, nil, loggateway.NewNoop(), nil,
	)
	wc := &wsConn{
		sessionID: "sess-noop",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 10),
		queues:    newConnQueues(conf.RuntimeWSConfig{}),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     "sess-noop",
			"after_revision": float64(1),
		},
	})
	// Should not panic.
	srv.handleUpstream(wc, raw)

	// No replay messages should be enqueued.
	select {
	case data := <-wc.queues.normal:
		if data != nil {
			t.Fatalf("expected no replay when EventStore is nil, got data")
		}
	default:
		// OK — no messages enqueued.
	}
}

func TestHandleSyncRequest_ZeroAfterRevisionIsNoop(t *testing.T) {
	repo := &fakeEventStoreRepo{}
	repo.records = []biz.EventStoreRecord{
		{ID: "evt-1", SessionID: "sess-zero", Type: "run_status", Channel: "chat", EnvelopeJSON: makeEnvelopeJSON("evt-1", "sess-zero", contract.EnvelopeTypeRunStatus, 1), CreatedAt: time.Now().UTC()},
	}
	srv := newSyncTestServer(t, repo)
	wc := &wsConn{
		sessionID: "sess-zero",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 10),
		queues:    newConnQueues(conf.RuntimeWSConfig{}),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     "sess-zero",
			"after_revision": float64(0),
		},
	})
	srv.handleUpstream(wc, raw)

	// No replay messages — after_revision=0 means event-ID-based replay handles it.
	select {
	case data := <-wc.queues.normal:
		if data != nil {
			t.Fatalf("expected no replay when after_revision=0, got data")
		}
	default:
		// OK.
	}
}

func TestHandleSyncRequest_FiltersByChannelSubscription(t *testing.T) {
	repo := &fakeEventStoreRepo{}
	now := time.Now().UTC()
	// Envelope on "monitor" channel (not subscribed by wc).
	envJSON := makeEnvelopeJSON("evt-mon", "sess-chan", contract.EnvelopeTypeRunStatus, 5)
	// Override channel to "monitor".
	var env contract.Envelope
	_ = json.Unmarshal([]byte(envJSON), &env)
	env.Channel = "monitor"
	data, _ := json.Marshal(env)
	repo.records = []biz.EventStoreRecord{
		{ID: "evt-mon", SessionID: "sess-chan", Type: "run_status", Channel: "monitor", EnvelopeJSON: string(data), CreatedAt: now},
	}
	srv := newSyncTestServer(t, repo)
	// wc only subscribes to chat+system, not monitor.
	wc := &wsConn{
		sessionID: "sess-chan",
		channels:  map[string]bool{"chat": true, "system": true},
		send:      make(chan []byte, 10),
		queues:    newConnQueues(conf.RuntimeWSConfig{}),
	}
	raw, _ := json.Marshal(wsUpstream{
		Direction: "client_to_server",
		Channel:   "system",
		Type:      "sync_request",
		Payload: map[string]any{
			"session_id":     "sess-chan",
			"after_revision": float64(1),
		},
	})
	srv.handleUpstream(wc, raw)

	// No replay messages — envelope is on monitor channel, wc not subscribed.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case d := <-wc.queues.normal:
			if d == nil {
				continue
			}
			var msg wsDownstream
			if err := json.Unmarshal(d, &msg); err != nil {
				continue
			}
			if msg.Type == "replay" {
				t.Fatalf("expected no replay for unsubscribed channel, got envelope id=%s", msg.Envelope.ID)
			}
		case <-deadline:
			return
		}
	}
}

func TestParseEnvelopeJSON_Valid(t *testing.T) {
	envJSON := makeEnvelopeJSON("evt-x", "sess", contract.EnvelopeTypeRunStatus, 42)
	env, err := parseEnvelopeJSON(envJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.ID != "evt-x" {
		t.Fatalf("id = %s, want evt-x", env.ID)
	}
	if env.SessionRevision != 42 {
		t.Fatalf("revision = %d, want 42", env.SessionRevision)
	}
}

func TestParseEnvelopeJSON_MissingID(t *testing.T) {
	// Envelope without ID should fail.
	raw := `{"type":"run_status","session_id":"s","channel":"chat","session_revision":1}`
	_, err := parseEnvelopeJSON(raw)
	if err == nil {
		t.Fatal("expected error for envelope without ID")
	}
}

func TestParseEnvelopeJSON_InvalidJSON(t *testing.T) {
	_, err := parseEnvelopeJSON("{not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
