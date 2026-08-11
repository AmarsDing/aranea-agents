package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubAgentLookupGet 返回固定 agent 的窄接口桩（C2 投机意图测试用）。
type stubAgentLookupGet struct {
	stubTeamAgentLookup
	ag biz.Agent
}

func (s stubAgentLookupGet) Get(context.Context, string) (biz.Agent, error) { return s.ag, nil }

// newSpeculatorFixture 构造带 session/agent 桩的 speculator；runIntentFn 由测试注入。
func newSpeculatorFixture(sess biz.Session, ag biz.Agent) (*ChatOrchestrator, *VoiceIntentSpeculator) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return sess, nil
	}}
	orch.core.TD.ReadDeps.AgentsUC = stubAgentLookupGet{ag: ag}
	sp := NewVoiceIntentSpeculator(&ChatService{orch: orch, lg: loggateway.NewNoop()})
	return orch, sp
}

// C2：nil ChatService / nil orchestrator → 返回 nil（接线方关闭投机）。
func TestNewVoiceIntentSpeculator_NilGuards(t *testing.T) {
	if got := NewVoiceIntentSpeculator(nil); got != nil {
		t.Fatalf("expected nil speculator for nil ChatService, got %v", got)
	}
	if got := NewVoiceIntentSpeculator(&ChatService{}); got != nil {
		t.Fatalf("expected nil speculator for nil orchestrator, got %v", got)
	}
}

// C2：会话不存在 → 快速返回，不存槽。
func TestSpeculateIntent_SessionMissing(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "not found")
	}}
	sp := NewVoiceIntentSpeculator(&ChatService{orch: orch, lg: loggateway.NewNoop()})
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		t.Fatal("intent must not run when session is missing")
		return nil
	}
	done := make(chan struct{})
	go func() {
		sp.SpeculateIntent(context.Background(), "sess-missing", "今天天气")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SpeculateIntent must return promptly when session is missing")
	}
	if n := sp.slotCount(); n != 0 {
		t.Fatalf("no slot must be stored for missing session, got %d", n)
	}
}

// C2：团队会话跳过（团队 Turn 走独立意图路径，不消费投机产物）。
func TestSpeculateIntent_SkipsTeamSession(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-team", OwnerType: "team", TeamID: "team-1"},
		biz.Agent{ID: "a1"},
	)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		t.Fatal("intent must not run for team sessions")
		return nil
	}
	sp.SpeculateIntent(context.Background(), "sess-team", "今天天气")
	if n := sp.slotCount(); n != 0 {
		t.Fatalf("team session must not store a slot, got %d", n)
	}
}

// C2：agent 关闭意图识别 → 跳过（与 Turn 侧 ShouldRun 门控同源）。
func TestSpeculateIntent_SkipsWhenIntentDisabled(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1", Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: false}},
	)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		t.Fatal("intent must not run when agent disabled the pass")
		return nil
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气")
	if n := sp.slotCount(); n != 0 {
		t.Fatalf("disabled intent pass must not store a slot, got %d", n)
	}
}

// C2：final 文本与投机 partial 一致 → 注入产物复用，槽位一次性消费。
func TestSpeculateResolve_HitInjectsArtifact(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	art := &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact { return art }

	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != art {
		t.Fatalf("matching final must inject the speculative artifact, got %v", got)
	}
	// 槽位已消费：再次 resolve 落空。
	ctx2 := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx2); got != nil {
		t.Fatalf("slot must be consumed after first resolve, got %v", got)
	}
}

// C2：final 文本失配 → 丢弃投机结果走常规路径（hash 一致性保障）。
func TestSpeculateResolve_MismatchDiscards(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气")
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "明天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("mismatched final must discard the speculative artifact, got %v", got)
	}
	if n := sp.slotCount(); n != 0 {
		t.Fatalf("mismatched slot must be discarded, got %d", n)
	}
}

// C2：final 到达时投机仍在途 → 有界等待其完成并复用（核心收益路径）。
func TestSpeculateResolve_WaitsForInflightCompletion(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	release := make(chan struct{})
	art := &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		<-release
		return art
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")

	type result struct {
		hit bool
	}
	done := make(chan result, 1)
	go func() {
		ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
		done <- result{hit: intent.SpeculativeArtifactFromContext(ctx) != nil}
	}()
	time.Sleep(100 * time.Millisecond) // 确保 resolve 已在等待
	close(release)
	select {
	case r := <-done:
		if !r.hit {
			t.Fatal("resolve must wait for in-flight speculation and inject on completion")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolve must return promptly after speculation completes")
	}
}

// C2：在途等待超 cap → 放弃投机走常规路径（有界等待，不无限阻塞 Turn 派发）。
func TestSpeculateResolve_WaitCapFallback(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	sp.waitCap = 50 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		<-release
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")

	start := time.Now()
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("wait-cap timeout must fall back without artifact, got %v", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("resolve must respect the wait cap, took %v", elapsed)
	}
}

// C2：无槽（未投机）→ 原样透传。
func TestSpeculateResolve_NoSlotPassthrough(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("no slot must pass through unchanged, got %v", got)
	}
}

// C2：投机失败（LLM 跳过/错误）→ 复用方走常规路径。
func TestSpeculateResolve_FailedSpeculationFallsBack(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact { return nil }
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("failed speculation must fall back, got %v", got)
	}
}

// P1-F：final 仅标点/空白差异（ASR 润色：补句号、去逗号）→ 归一化匹配命中，
// 复用投机产物。refined_goal 语义不变，零误复用风险。
func TestSpeculateResolve_PunctuationVariantReuses(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	art := &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact { return art }

	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气，怎么样")
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样。")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != art {
		t.Fatalf("punctuation-variant final must reuse the speculative artifact, got %v", got)
	}
}

// P1-F：同一语句的标点变体重触发 → 归一化去重，不重复调用 LLM。
func TestSpeculateIntent_PunctuationVariantSkipsRedundantCall(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	calls := make(chan string, 4)
	sp.runIntentFn = func(_ context.Context, _ biz.Agent, _ biz.Session, _, text string) *intent.Artifact {
		calls <- text
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样。")
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("first speculation must run")
	}
	select {
	case <-calls:
		t.Fatal("punctuation-variant stable text must not trigger a second LLM call")
	case <-time.After(300 * time.Millisecond):
	}
}

// P1-F：文本实体差异（用户改口/ASR 纠错）归一化后仍不同 → 丢弃走常规路径。
func TestSpeculateResolve_EntityMismatchStillDiscards(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	sp.runIntentFn = func(context.Context, biz.Agent, biz.Session, string, string) *intent.Artifact {
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "明天天气怎么样。")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("entity-mismatched final must discard the artifact, got %v", got)
	}
}

// C2：同一稳定文本重复触发 → 去重，不重复调用 LLM（稳定计时器重入场景）。
func TestSpeculateIntent_DuplicateTextSkipsRedundantCall(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	calls := make(chan string, 4)
	sp.runIntentFn = func(_ context.Context, _ biz.Agent, _ biz.Session, _, text string) *intent.Artifact {
		calls <- text
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("first speculation must run")
	}
	select {
	case <-calls:
		t.Fatal("duplicate stable text must not trigger a second LLM call")
	case <-time.After(300 * time.Millisecond):
	}
}

// C2：新 partial 取代旧槽（用户继续说话，旧投机文本失效）。
func TestSpeculateIntent_NewerPartialSupersedes(t *testing.T) {
	_, sp := newSpeculatorFixture(
		biz.Session{ID: "sess-1", OwnerType: "agent", AgentID: "a1"},
		biz.Agent{ID: "a1"},
	)
	sp.runIntentFn = func(_ context.Context, _ biz.Agent, _ biz.Session, _, _ string) *intent.Artifact {
		return &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	}
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气")
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	// 旧文本 resolve → 失配丢弃。
	ctx := sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气")
	if got := intent.SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("superseded slot text must miss, got %v", got)
	}
	// 新文本 resolve → 命中。
	sp.SpeculateIntent(context.Background(), "sess-1", "今天天气怎么样")
	ctx = sp.WithSpeculativeIntent(context.Background(), "sess-1", "今天天气怎么样")
	if got := intent.SpeculativeArtifactFromContext(ctx); got == nil {
		t.Fatal("latest slot text must hit")
	}
}
