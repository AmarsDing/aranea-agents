package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestMemoryCueResult_IsEmpty(t *testing.T) {
	if (&MemoryCueResult{}).IsEmpty() != true {
		t.Error("empty result should be empty")
	}
	if (&MemoryCueResult{L1Cue: "test"}).IsEmpty() != false {
		t.Error("result with L1Cue should not be empty")
	}
	if (&MemoryCueResult{RecallCue: "test"}).IsEmpty() != false {
		t.Error("result with RecallCue should not be empty")
	}
}

func TestMemoryCueResult_JoinCues(t *testing.T) {
	r := &MemoryCueResult{L1Cue: "L1", RecallCue: "Recall"}
	if r.JoinCues() != "L1\n\nRecall" {
		t.Errorf("unexpected JoinCues result: %q", r.JoinCues())
	}
	if (&MemoryCueResult{L1Cue: "L1"}).JoinCues() != "L1" {
		t.Errorf("unexpected JoinCues result with only L1: %q", (&MemoryCueResult{L1Cue: "L1"}).JoinCues())
	}
}

// P2-3（2026-08-16）：per-turn 召回缓存。同一 invocation 的工具循环续轮
// （keyword 不变）复用首轮召回结果（fresh=false，embed+多路检索每 turn 只做
// 一次，副作用由调用方按 fresh 门控为 once-per-turn）；keyword 变化或新
// invocation 触发新一轮真实召回。
func TestBuildRuntimeMemoryCue_TurnCacheReuse(t *testing.T) {
	ag := biz.Agent{ID: "ag-1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true}}
	deps := TRPCBuilderDeps{}
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	msgs := []trpcmodel.Message{trpcmodel.NewUserMessage("数据库慢查询怎么排查")}
	first, fresh := buildRuntimeMemoryCue(ctx, deps, ag, msgs)
	if !fresh {
		t.Fatal("first call in a turn must be a fresh recall")
	}

	// 工具循环续轮：尾部追加 assistant 消息，末条 user 消息不变 → keyword 不变。
	loopMsgs := append(msgs, trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "calling tool"})
	second, fresh := buildRuntimeMemoryCue(ctx, deps, ag, loopMsgs)
	if fresh {
		t.Fatal("tool-loop re-entry with unchanged keyword must hit the turn cache")
	}
	if second.JoinCues() != first.JoinCues() {
		t.Fatalf("cached turn must replay the same cue: %q vs %q", second.JoinCues(), first.JoinCues())
	}

	// keyword 变化（新 user 消息）→ 缓存失效，重新召回。
	changed := append(loopMsgs, trpcmodel.NewUserMessage("换个话题"))
	if _, fresh = buildRuntimeMemoryCue(ctx, deps, ag, changed); !fresh {
		t.Fatal("keyword change must trigger a fresh recall")
	}

	// 新 invocation（新 turn）→ 缓存不跨 turn。
	inv2 := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "s1", UserID: "u1"}}
	ctx2 := trpcagent.NewInvocationContext(context.Background(), inv2)
	if _, fresh = buildRuntimeMemoryCue(ctx2, deps, ag, msgs); !fresh {
		t.Fatal("a new invocation has no turn cache and must recall fresh")
	}
}

func TestIsMemoryInjectMessage(t *testing.T) {
	msg := asDynamicCue(memoryInjectCueContent("test cue"))
	if !isMemoryInjectMessage(msg) {
		t.Error("message with marker should be identified")
	}
	plainMsg := trpcmodel.NewSystemMessage("plain content")
	if isMemoryInjectMessage(plainMsg) {
		t.Error("plain message should not be identified as memory inject")
	}
}

// TestMemoryRuntimeContext_TeamIDResolution verifies the C5 team_id chain:
// session state takes priority; when absent, fall back to the invocation's
// RunOptions.RuntimeState (injected by the team graph runtime).
func TestMemoryRuntimeContext_TeamIDResolution(t *testing.T) {
	ag := biz.Agent{ID: "ag-1"}

	// Case 1: session state wins over RuntimeState.
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{
			UserID: "u1",
			State:  map[string][]byte{"team_id": []byte("team-from-session")},
		},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": "team-from-runtime"},
		},
	}
	if rt := memoryRuntimeContext(inv, ag); rt.TeamID != "team-from-session" {
		t.Fatalf("session state should win, TeamID=%q", rt.TeamID)
	}

	// Case 2: fallback to RuntimeState when session state lacks team_id.
	inv2 := &trpcagent.Invocation{
		Session: &trpcsession.Session{UserID: "u1"},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": "team-from-runtime"},
		},
	}
	if rt := memoryRuntimeContext(inv2, ag); rt.TeamID != "team-from-runtime" {
		t.Fatalf("RuntimeState fallback failed, TeamID=%q", rt.TeamID)
	}

	// Case 3: neither source → empty TeamID.
	inv3 := &trpcagent.Invocation{Session: &trpcsession.Session{UserID: "u1"}}
	if rt := memoryRuntimeContext(inv3, ag); rt.TeamID != "" {
		t.Fatalf("expected empty TeamID, got %q", rt.TeamID)
	}

	// Case 4: non-string RuntimeState value is ignored.
	inv4 := &trpcagent.Invocation{
		Session: &trpcsession.Session{UserID: "u1"},
		RunOptions: trpcagent.RunOptions{
			RuntimeState: map[string]any{"team_id": 42},
		},
	}
	if rt := memoryRuntimeContext(inv4, ag); rt.TeamID != "" {
		t.Fatalf("non-string team_id should be ignored, got %q", rt.TeamID)
	}
}

// ── memory_recalled notice (R4: chat-level recall transparency) ─────────

// noticeRecorder implements biz.ActivityEmitter capturing EmitNotice calls.
type noticeRecorder struct {
	calls []noticeCall
}

type noticeCall struct{ content, noticeType string }

func (r *noticeRecorder) EmitNotice(_ context.Context, content, noticeType string) error {
	r.calls = append(r.calls, noticeCall{content: content, noticeType: noticeType})
	return nil
}
func (r *noticeRecorder) EmitConfirmRequest(context.Context, biz.ActivityConfirmParams) (string, error) {
	return "", nil
}
func (r *noticeRecorder) EmitConfirmResult(context.Context, string, bool) error { return nil }
func (r *noticeRecorder) EmitConfirmTimeout(context.Context, string) error      { return nil }

func TestEmitMemoryRecalledNotice_EmitsJSONPayload(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	hits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "用户偏好 XX 餐厅", Score: 0.91, FactID: "f-1", Confidence: 0.88, Version: 3},
		{Layer: "L2", Line: "上次聚餐点了日料", Score: 0.72},
	}
	ctx = emitMemoryRecalledNotice(ctx, hits)
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(rec.calls))
	}
	if rec.calls[0].noticeType != memoryRecalledNoticeType {
		t.Fatalf("noticeType = %q, want %q", rec.calls[0].noticeType, memoryRecalledNoticeType)
	}
	var payload memoryRecalledNoticePayload
	if err := json.Unmarshal([]byte(rec.calls[0].content), &payload); err != nil {
		t.Fatalf("notice content is not valid JSON: %v", err)
	}
	if len(payload.Hits) != 2 {
		t.Fatalf("payload hits = %d, want 2", len(payload.Hits))
	}
	if payload.Hits[0].Layer != "L3" || payload.Hits[0].FactID != "f-1" || payload.Hits[0].Score != 0.91 {
		t.Fatalf("unexpected first hit: %+v", payload.Hits[0])
	}
}

func TestEmitMemoryRecalledNotice_DeduplicatesWithinInvocation(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	hits := []biz.CompositeRecallHit{{Layer: "L3", Line: "fact", Score: 0.9}}
	ctx = emitMemoryRecalledNotice(ctx, hits)
	// Second call with the marked ctx (tool-loop re-entry) must not emit again.
	ctx = emitMemoryRecalledNotice(ctx, hits)
	if len(rec.calls) != 1 {
		t.Fatalf("expected dedup to 1 notice, got %d", len(rec.calls))
	}
}

func TestEmitMemoryRecalledNotice_NilEmitterNoPanic(t *testing.T) {
	ctx := emitMemoryRecalledNotice(context.Background(), []biz.CompositeRecallHit{{Layer: "L3", Line: "x"}})
	if ctx == nil {
		t.Fatal("ctx must be returned even without emitter")
	}
}

func TestEmitMemoryRecalledNotice_NoHitsSkips(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	emitMemoryRecalledNotice(ctx, nil)
	if len(rec.calls) != 0 {
		t.Fatalf("no hits must not emit, got %d", len(rec.calls))
	}
}

func TestEmitMemoryRecalledNotice_TruncatesLongLines(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	long := strings.Repeat("记", 300)
	emitMemoryRecalledNotice(ctx, []biz.CompositeRecallHit{{Layer: "L3", Line: long, Score: 0.9}})
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(rec.calls))
	}
	if strings.Contains(rec.calls[0].content, long) {
		t.Fatal("300-rune line must be truncated in notice payload")
	}
}

// ── L4 reconsolidation trigger (design §15.7, FR-10.5) ─────────────────

// reconsolidatorRecorder implements biz.L4Reconsolidator capturing OnRecall
// calls. wg (optional) signals async completion for safego.Go-based triggers.
type reconsolidatorRecorder struct {
	mu    sync.Mutex
	calls map[string][]string
	wg    *sync.WaitGroup
}

func (r *reconsolidatorRecorder) OnRecall(_ context.Context, nodeID string, recalledWith []string) error {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = map[string][]string{}
	}
	r.calls[nodeID] = append([]string(nil), recalledWith...)
	r.mu.Unlock()
	if r.wg != nil {
		r.wg.Done()
	}
	return nil
}

func (r *reconsolidatorRecorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *reconsolidatorRecorder) coRecalled(nodeID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[nodeID]
}

func TestTriggerL4Reconsolidation_FiresPerEntityWithCoRecalled(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(3)
	rec := &reconsolidatorRecorder{wg: &wg}
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{MemoryReconsolidator: rec}}
	ctx := triggerL4Reconsolidation(context.Background(), deps, []string{"e1", "e2", "e3"})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("OnRecall not invoked for all entities, got %d calls", rec.callCount())
	}
	if got := rec.coRecalled("e1"); len(got) != 2 || !containsAll(strings.Join(got, ","), "e2", "e3") {
		t.Fatalf("e1 co-recalled = %v, want [e2 e3]", got)
	}
	if got := rec.coRecalled("e2"); len(got) != 2 || !containsAll(strings.Join(got, ","), "e1", "e3") {
		t.Fatalf("e2 co-recalled = %v, want [e1 e3]", got)
	}
	// The returned ctx must carry the once-per-turn marker.
	if ctx.Value(l4ReconsolidatedKey{}) == nil {
		t.Fatal("ctx must carry the reconsolidation marker")
	}
}

func TestTriggerL4Reconsolidation_DeduplicatesWithinTurn(t *testing.T) {
	rec := &reconsolidatorRecorder{}
	deps := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{MemoryReconsolidator: rec}}
	ctx := triggerL4Reconsolidation(context.Background(), deps, []string{"e1"})
	// Second call with the marked ctx (tool-loop re-entry) must not re-trigger.
	triggerL4Reconsolidation(ctx, deps, []string{"e1"})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if rec.callCount() > 1 {
			t.Fatalf("expected at most 1 OnRecall, got %d", rec.callCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTriggerL4Reconsolidation_SkipsWhenNilOrEmpty(t *testing.T) {
	rec := &reconsolidatorRecorder{}
	withRec := TRPCBuilderDeps{TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{MemoryReconsolidator: rec}}
	// Empty IDs → no trigger, no marker.
	ctx := triggerL4Reconsolidation(context.Background(), withRec, nil)
	if ctx.Value(l4ReconsolidatedKey{}) != nil {
		t.Fatal("empty IDs must not mark ctx")
	}
	// Nil reconsolidator → no trigger, no marker.
	noRec := TRPCBuilderDeps{}
	ctx = triggerL4Reconsolidation(context.Background(), noRec, []string{"e1"})
	if ctx.Value(l4ReconsolidatedKey{}) != nil {
		t.Fatal("nil reconsolidator must not mark ctx")
	}
	if rec.callCount() != 0 {
		t.Fatalf("expected 0 OnRecall calls, got %d", rec.callCount())
	}
}
