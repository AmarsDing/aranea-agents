package service_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// decoSyncSessionRepo overrides BumpSessionRevision to actually increment
// (batchSessionRepo.BumpSessionRevision is hardcoded to return 1).
// Phase 1c-3: AppendChatMessage and ListMessagesAfterRevision overrides removed —
// AppendChatMessage is noop in production (ActivityProjector persists), and
// ListMessagesAfterRevision goes through ActivityMessageReader (full replay).
type decoSyncSessionRepo struct {
	batchSessionRepo
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

func TestDECO01_SessionRevisionChannelToWebSync(t *testing.T) {
	const sessionID = "sess-deco-01"
	repo := &decoSyncSessionRepo{
		batchSessionRepo: batchSessionRepo{
			sessions: map[string]biz.Session{
				sessionID: {ID: sessionID, SessionRevision: 0},
			},
		},
	}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop())
	proj := service.NewSessionProjectionAdapter(uc, nil)
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{SessionID: sessionID, BufferSize: 4})
	defer unsub()

	ctx := context.Background()
	runID := "run-feishu-1"
	userMsg := biz.ChatMessage{ID: "u1", SessionID: sessionID, Role: "user", ContentMarkdown: "你好", TurnNumber: 1, TurnID: "t1"}
	// Phase 1c-3: uc.AppendChatMessage is noop (ActivityProjector persists messages).
	// Store directly in the repo fixture for test verification.
	repo.messages = append(repo.messages, userMsg)
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

	assistantMsg := biz.ChatMessage{ID: "a1", SessionID: sessionID, Role: "assistant", ContentMarkdown: "你好！", TurnNumber: 1, TurnID: "t1"}
	repo.messages = append(repo.messages, assistantMsg)
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
		biz.ChatMessage{ID: "u2", SessionID: sessionID, Role: "user", ContentMarkdown: "继续", TurnNumber: 2, TurnID: "t2"},
		biz.ChatMessage{ID: "a2", SessionID: sessionID, Role: "assistant", ContentMarkdown: "好的", TurnNumber: 2, TurnID: "t2"},
	)
	repo.sessions[sessionID] = biz.Session{ID: sessionID, SessionRevision: 2}
	incr, err := proj.GetMessagesAfterRevision(ctx, sessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Phase 1c-3: full replay — all 4 messages returned regardless of revision.
	if len(incr) != 4 || incr[0].ID != "u1" {
		t.Fatalf("after rev1 full replay: %+v", incr)
	}
	latest, err := proj.GetLatestRevision(ctx, sessionID)
	if err != nil || latest != 2 {
		t.Fatalf("latest rev=%d err=%v", latest, err)
	}
}
