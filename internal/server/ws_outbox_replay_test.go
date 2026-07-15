package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

type fakeOutboxRepo struct {
	mu   sync.Mutex
	rows []biz.EventDeliveryOutboxRow
}

func (f *fakeOutboxRepo) Insert(_ context.Context, row biz.EventDeliveryOutboxRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, row)
	return nil
}

func (f *fakeOutboxRepo) MarkPublished(context.Context, string, time.Time) error { return nil }

func (f *fakeOutboxRepo) ListAfter(_ context.Context, sessionID, afterEventID string, afterSeq int64, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cursor := afterSeq
	if afterEventID != "" {
		found := false
		for _, r := range f.rows {
			if r.SessionID == sessionID && r.EventID == afterEventID {
				cursor = r.Seq
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}
	if limit <= 0 {
		limit = 100
	}
	var out []biz.EventDeliveryOutboxRow
	for _, r := range f.rows {
		if r.SessionID != sessionID || r.Seq <= cursor {
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func TestWSServer_ReplayOutbox_FiltersByLastEventID(t *testing.T) {
	t.Parallel()
	repo := &fakeOutboxRepo{rows: []biz.EventDeliveryOutboxRow{
		{SessionID: "sess-1", Seq: 1, EventID: "evt-1", Payload: []byte(`{"event_id":"evt-1"}`)},
		{SessionID: "sess-1", Seq: 2, EventID: "evt-2", Payload: []byte(`{"event_id":"evt-2"}`)},
		{SessionID: "sess-1", Seq: 3, EventID: "evt-3", Payload: []byte(`{"event_id":"evt-3"}`)},
		{SessionID: "sess-other", Seq: 9, EventID: "evt-x", Payload: []byte(`{"event_id":"evt-x"}`)},
	}}

	srv := &WSServer{
		outbox:      repo,
		runtimeConf: &conf.Runtime{},
		lg:          loggateway.NewNoop(),
	}
	wcCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := srv.wsConfig()
	wc := &wsConn{
		sessionID:  "sess-1",
		queues:     newConnQueues(cfg),
		connCtx:    wcCtx,
		connCancel: cancel,
	}

	srv.replayOutbox(wc, "sess-1", "evt-1")

	var got [][]byte
drain:
	for {
		select {
		case msg := <-wc.queues.high:
			got = append(got, msg)
		default:
			break drain
		}
	}
	if len(got) != 2 {
		t.Fatalf("replay queued %d frames, want 2 (seq>1)", len(got))
	}
	if string(got[0]) != `{"event_id":"evt-2"}` || string(got[1]) != `{"event_id":"evt-3"}` {
		t.Fatalf("replay payloads = %s / %s", got[0], got[1])
	}
}

func TestWSServer_ReplayOutbox_SkipsGlobalAndEmpty(t *testing.T) {
	t.Parallel()
	repo := &fakeOutboxRepo{rows: []biz.EventDeliveryOutboxRow{
		{SessionID: "sess-1", Seq: 1, EventID: "evt-1", Payload: []byte(`x`)},
	}}
	srv := &WSServer{outbox: repo, runtimeConf: &conf.Runtime{}, lg: loggateway.NewNoop()}
	cfg := srv.wsConfig()
	wc := &wsConn{sessionID: "*", queues: newConnQueues(cfg)}

	srv.replayOutbox(wc, "*", "evt-1")
	select {
	case <-wc.queues.high:
		t.Fatal("global session must not replay")
	default:
	}

	srv.replayOutbox(wc, "sess-1", "")
	select {
	case <-wc.queues.high:
		t.Fatal("empty last_event_id must not replay")
	default:
	}
}
