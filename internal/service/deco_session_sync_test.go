package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/service"
)

type decoSyncSessionRepo struct {
	batchSessionRepo
	messages []biz.ChatMessage
}

func (r *decoSyncSessionRepo) AppendChatMessage(_ context.Context, sessionID string, msg biz.ChatMessage, _ bool) error {
	msg.SessionID = sessionID
	r.messages = append(r.messages, msg)
	return nil
}

func (r *decoSyncSessionRepo) BumpSessionRevision(_ context.Context, sessionID string) (int64, error) {
	s, ok := r.sessions[sessionID]
	if !ok {
		return 0, apierror.NotFound(apierror.DomainSession, "not found")
	}
	s.SessionRevision++
	r.sessions[sessionID] = s
	return s.SessionRevision, nil
}

func (r *decoSyncSessionRepo) ListMessagesAfterRevision(_ context.Context, _ string, afterRevision int64) ([]biz.ChatMessage, error) {
	if afterRevision <= 0 {
		out := make([]biz.ChatMessage, len(r.messages))
		copy(out, r.messages)
		return out, nil
	}
	var out []biz.ChatMessage
	for _, msg := range r.messages {
		if int64(msg.TurnNumber) > afterRevision {
			out = append(out, msg)
		}
	}
	return out, nil
}

func TestDECO01_SessionRevisionChannelToWebSync(t *testing.T) {
	const sessionID = "sess-deco-01"
	repo := &decoSyncSessionRepo{
		batchSessionRepo: batchSessionRepo{sessions: map[string]biz.Session{
			sessionID: {ID: sessionID, SessionRevision: 0},
		}},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil)
	proj := service.NewSessionProjectionAdapter(uc, nil)
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{SessionID: sessionID, BufferSize: 4})
	defer unsub()

	ctx := context.Background()
	runID := "run-feishu-1"
	userMsg := biz.ChatMessage{ID: "u1", Role: "user", ContentMarkdown: "你好", TurnNumber: 1, TurnID: "t1"}
	if err := uc.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
		t.Fatal(err)
	}
	revSync, err := uc.BumpSessionRevision(ctx, sessionID)
	if err != nil || revSync != 1 {
		t.Fatalf("sync bump: rev=%d err=%v", revSync, err)
	}
	event.PublishSessionRevisionEnvelope(bus, sessionID, runID, userMsg.ID, "channel", revSync, event.SessionRunStatusSync)

	select {
	case env := <-ch:
		if env.SessionRevision != 1 || env.Metadata["status"] != event.SessionRunStatusSync {
			t.Fatalf("sync envelope: rev=%d status=%v", env.SessionRevision, env.Metadata["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting sync envelope")
	}

	msgs, err := proj.GetMessagesAfterRevision(ctx, sessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].ID != "u1" {
		t.Fatalf("after rev0: %+v", msgs)
	}

	assistantMsg := biz.ChatMessage{ID: "a1", Role: "assistant", ContentMarkdown: "你好！", TurnNumber: 1, TurnID: "t1"}
	if err := uc.AppendChatMessage(ctx, sessionID, assistantMsg, false); err != nil {
		t.Fatal(err)
	}
	revDone, err := uc.BumpSessionRevision(ctx, sessionID)
	if err != nil || revDone != 2 {
		t.Fatalf("completed bump: rev=%d err=%v", revDone, err)
	}
	event.PublishSessionRevisionEnvelope(bus, sessionID, runID, userMsg.ID, "channel", revDone, event.SessionRunStatusCompleted)

	select {
	case env := <-ch:
		if env.SessionRevision != 2 || env.Metadata["status"] != event.SessionRunStatusCompleted {
			t.Fatalf("completed envelope: rev=%d status=%v", env.SessionRevision, env.Metadata["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting completed envelope")
	}

	delta, err := proj.GetMessagesAfterRevision(ctx, sessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta) != 2 || delta[0].ID != "u1" || delta[1].ID != "a1" {
		t.Fatalf("completed turn messages: %+v", delta)
	}

	repo.messages = append(repo.messages,
		biz.ChatMessage{ID: "u2", Role: "user", ContentMarkdown: "继续", TurnNumber: 2, TurnID: "t2"},
		biz.ChatMessage{ID: "a2", Role: "assistant", ContentMarkdown: "好的", TurnNumber: 2, TurnID: "t2"},
	)
	repo.sessions[sessionID] = biz.Session{ID: sessionID, SessionRevision: 2}
	incr, err := proj.GetMessagesAfterRevision(ctx, sessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(incr) != 2 || incr[0].ID != "u2" {
		t.Fatalf("after rev1 incremental: %+v", incr)
	}
	latest, err := proj.GetLatestRevision(ctx, sessionID)
	if err != nil || latest != 2 {
		t.Fatalf("latest rev=%d err=%v", latest, err)
	}
}
