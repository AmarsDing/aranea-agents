package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// C1：nil ChatService（或 nil orchestrator）→ 返回 nil，接线方跳过预热。
func TestNewVoiceTurnPrewarmer_NilService(t *testing.T) {
	if got := NewVoiceTurnPrewarmer(nil); got != nil {
		t.Fatalf("expected nil prewarmer for nil ChatService, got %v", got)
	}
	if got := NewVoiceTurnPrewarmer(&ChatService{}); got != nil {
		t.Fatalf("expected nil prewarmer for nil orchestrator, got %v", got)
	}
}

// C1：会话不存在 → 快速返回（非阻断容错），不 panic。
func TestVoiceTurnPrewarmer_SessionNotFound(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found")
	}}
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()})
	if pw == nil {
		t.Fatal("expected non-nil prewarmer")
	}
	done := make(chan struct{})
	go func() {
		pw.PrewarmTurn(context.Background(), "sess-missing")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prewarm must return promptly when session is missing")
	}
}

// C1：团队会话走独立构建路径（executeTeamTurnViaHooks），预热直接跳过——
// 不触碰 agent 构建（裸 orchestrator 无 AgentsUC，若误入会 panic）。
func TestVoiceTurnPrewarmer_SkipsTeamSession(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, OwnerType: "team", TeamID: "team-1"}, nil
	}}
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()})
	done := make(chan struct{})
	go func() {
		pw.PrewarmTurn(context.Background(), "sess-team")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prewarm must skip team sessions promptly")
	}
}

// C1：无 agent_id 的会话 → 直接跳过。
func TestVoiceTurnPrewarmer_SkipsSessionWithoutAgent(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(_ context.Context, id string) (biz.Session, error) {
		return biz.Session{ID: id, OwnerType: "agent"}, nil
	}}
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()})
	done := make(chan struct{})
	go func() {
		pw.PrewarmTurn(context.Background(), "sess-no-agent")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prewarm must skip agentless sessions promptly")
	}
}
