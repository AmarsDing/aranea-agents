package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// C1：nil ChatService（或 nil orchestrator）→ 返回 nil，接线方跳过预热。
func TestNewVoiceTurnPrewarmer_NilService(t *testing.T) {
	if got := NewVoiceTurnPrewarmer(nil, nil); got != nil {
		t.Fatalf("expected nil prewarmer for nil ChatService, got %v", got)
	}
	if got := NewVoiceTurnPrewarmer(&ChatService{}, nil); got != nil {
		t.Fatalf("expected nil prewarmer for nil orchestrator, got %v", got)
	}
}

// C1：会话不存在 → 快速返回（非阻断容错），不 panic。
func TestVoiceTurnPrewarmer_SessionNotFound(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found")
	}}
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()}, nil)
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
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()}, nil)
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
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()}, nil)
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

// ---------------------------------------------------------------------------
// C3（2026-08-11）：voice.start 并列触发 embedding 冷启动预热
// ---------------------------------------------------------------------------

// countingEmbedPrewarmer 记录 Prewarm 调用次数（窄接口 embedPrewarmer 桩）。
type countingEmbedPrewarmer struct{ calls int32 }

func (c *countingEmbedPrewarmer) Prewarm(context.Context) error {
	atomic.AddInt32(&c.calls, 1)
	return nil
}

// C3：embedding 预热不依赖会话/agent 构建——即使会话获取失败（预热主流程
// 容错返回），voice.start 也必须已触发一次 embedding ping。
func TestVoiceTurnPrewarmer_PrewarmsEmbedderEvenWhenSessionMissing(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found")
	}}
	emb := &countingEmbedPrewarmer{}
	pw := NewVoiceTurnPrewarmer(&ChatService{orch: orch, lg: loggateway.NewNoop()}, emb)
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
	if got := atomic.LoadInt32(&emb.calls); got != 1 {
		t.Fatalf("expected embedder prewarm to run exactly once, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// 修复 C（2026-08-11）：PrewarmSpiritAgent 启动预构建守卫测试
// ---------------------------------------------------------------------------

// stubTeamAgentLookup 仅实现窄接口 biz.TeamAgentLookup（无 GetByAgentKey），
// 用于验证断言失败时启动预热优雅跳过。
type stubTeamAgentLookup struct{}

func (stubTeamAgentLookup) Get(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, apierror.NotFound(apierror.DomainAgent, "not found")
}
func (stubTeamAgentLookup) GetEffectiveTools(context.Context, string) (biz.AgentEffectiveTools, error) {
	return biz.AgentEffectiveTools{}, nil
}
func (stubTeamAgentLookup) BatchHydrateForBuild(_ context.Context, agents []biz.Agent) ([]biz.Agent, error) {
	return agents, nil
}

// stubSpiritResolver 在窄接口之上额外实现 GetByAgentKey（模拟生产
// *biz.AgentUsecase），返回错误以验证预热容错路径。
type stubSpiritResolver struct {
	stubTeamAgentLookup
	err error
}

func (s stubSpiritResolver) GetByAgentKey(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, s.err
}

// 修复 C：nil ChatService / nil orchestrator / 无 AgentsUC → 快速返回，不 panic。
func TestPrewarmSpiritAgent_NilGuards(t *testing.T) {
	cases := map[string]*ChatService{
		"nil service":      nil,
		"nil orchestrator": {},
		"no AgentsUC":      {orch: newSubmitAwaitReplyTestOrch(nil), lg: loggateway.NewNoop()},
	}
	for name, svc := range cases {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				svc.PrewarmSpiritAgent(context.Background())
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("PrewarmSpiritAgent must return promptly on nil guards")
			}
		})
	}
}

// 修复 C：AgentsUC 仅为窄接口（无 GetByAgentKey，测试桩场景）→ 断言失败跳过预热。
func TestPrewarmSpiritAgent_SkipsWhenResolverUnsupported(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.ReadDeps.AgentsUC = stubTeamAgentLookup{}
	svc := &ChatService{orch: orch, lg: loggateway.NewNoop()}
	done := make(chan struct{})
	go func() {
		svc.PrewarmSpiritAgent(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prewarm must skip promptly when AgentsUC lacks GetByAgentKey")
	}
}

// 修复 C：spirit agent 解析失败 → Warn 容错返回（K3），不 panic 不阻塞启动。
func TestPrewarmSpiritAgent_ToleratesResolverError(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.ReadDeps.AgentsUC = stubSpiritResolver{
		err: apierror.NotFound(apierror.DomainAgent, "spirit not seeded"),
	}
	svc := &ChatService{orch: orch, lg: loggateway.NewNoop()}
	done := make(chan struct{})
	go func() {
		svc.PrewarmSpiritAgent(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prewarm must tolerate resolver errors and return promptly")
	}
}
