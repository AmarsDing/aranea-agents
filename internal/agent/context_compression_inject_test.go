package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- partitionMessagesByTokenBudget tests ---

// convBudgetFor computes a token budget that keeps every system message
// (head static run + dynamic tail cues — matching partition's fixed cost)
// plus exactly keepLastN newest conversation messages of msgs.
func convBudgetFor(msgs []trpcmodel.Message, keepLastN int) int {
	_, conv, _ := splitPromptZones(msgs)
	total := 0
	for _, m := range msgs {
		if isPromptFixedMessage(m) {
			total += estTokensFromChars(messageCharLen(m))
		}
	}
	for _, m := range conv[len(conv)-keepLastN:] {
		total += estTokensFromChars(messageCharLen(m))
	}
	return total
}

// TestPartitionByTokenBudget_ToolPairSafety verifies that the partition never
// splits a tool-call / tool-result pair across the keep/evicted boundary.
func TestPartitionByTokenBudget_ToolPairSafety(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{{ID: "tc1", Function: trpcmodel.FunctionDefinitionParam{Name: "read"}}}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolID: "tc1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
		{Role: trpcmodel.RoleAssistant, Content: "a2"},
	}
	// Budget that would naively evict u1+a1 but keep r1 → forces boundary snap.
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 2))
	assertNoOrphanToolPairs(t, keep, "keep")
	assertNoOrphanToolPairs(t, evicted, "evicted")
}

// TestPartitionByTokenBudget_ToolPairBoundaryAtKeepSide verifies pairs stay
// together on the keep side when the boundary would split them.
func TestPartitionByTokenBudget_ToolPairBoundaryAtKeepSide(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{{ID: "tc1", Function: trpcmodel.FunctionDefinitionParam{Name: "read"}}}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolID: "tc1"},
		{Role: trpcmodel.RoleUser, Content: "u3"},
	}
	// Budget keeps only u3 → naive eviction splits a1/r1.
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 1))
	assertNoOrphanToolPairs(t, keep, "keep")
	assertNoOrphanToolPairs(t, evicted, "evicted")
}

// TestPartitionByTokenBudget_MultipleToolCalls verifies all tool calls from a
// single assistant message stay together with their results.
func TestPartitionByTokenBudget_MultipleToolCalls(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{
			{ID: "tc1", Function: trpcmodel.FunctionDefinitionParam{Name: "read"}},
			{ID: "tc2", Function: trpcmodel.FunctionDefinitionParam{Name: "write"}},
		}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolID: "tc1"},
		{Role: trpcmodel.RoleTool, Content: "r2", ToolID: "tc2"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 1))
	assertNoOrphanToolPairs(t, keep, "keep")
	assertNoOrphanToolPairs(t, evicted, "evicted")
}

// TestPartitionByTokenBudget_KeepsHeadAndTailSystem verifies both the static
// head system block and the dynamic tail cue system block are never evicted
// by the history-truncation pass.
func TestPartitionByTokenBudget_KeepsHeadAndTailSystem(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "identity"},
		{Role: trpcmodel.RoleSystem, Content: "instructions"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
		{Role: trpcmodel.RoleAssistant, Content: "hi"},
		{Role: trpcmodel.RoleUser, Content: "how are you"},
		{Role: trpcmodel.RoleSystem, Content: "memory cue"},
		{Role: trpcmodel.RoleSystem, Content: "knowledge cue"},
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 1))
	sysCount := 0
	for _, m := range keep {
		if m.Role == trpcmodel.RoleSystem {
			sysCount++
		}
	}
	if sysCount != 4 {
		t.Fatalf("expected all 4 system messages (2 head + 2 tail) kept, got %d", sysCount)
	}
	for _, m := range evicted {
		if m.Role == trpcmodel.RoleSystem {
			t.Fatalf("evicted must not contain system messages, got %q", m.Content)
		}
	}
}

func TestPartitionByTokenBudget_KeepsTrailingUserCues(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "identity"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
		{Role: trpcmodel.RoleAssistant, Content: "hi"},
		{Role: trpcmodel.RoleUser, Content: "how are you"},
		asDynamicCue("memory cue"),
		asDynamicCue("knowledge cue"),
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 1))
	cueCount := 0
	for _, m := range keep {
		if isDynamicCueMessage(m) {
			cueCount++
		}
	}
	if cueCount != 2 {
		t.Fatalf("expected 2 trailing user cues kept, got %d", cueCount)
	}
	for _, m := range evicted {
		if isDynamicCueMessage(m) {
			t.Fatalf("evicted must not contain dynamic cues, got %q", m.Content)
		}
	}
}

func TestSplitPromptZones_TrailingUserCues(t *testing.T) {
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("hello"),
		trpcmodel.NewAssistantMessage("hi"),
		asDynamicCue("memory cue"),
	}
	head, conv, tail := splitPromptZones(msgs)
	if len(head) != 1 || len(conv) != 2 || len(tail) != 1 {
		t.Fatalf("head=%d conv=%d tail=%d", len(head), len(conv), len(tail))
	}
	if !isDynamicCueMessage(tail[0]) {
		t.Fatal("tail must be the user-role dynamic cue")
	}
}

// TestPartitionByTokenBudget_EvictsOldestFirst verifies token-driven eviction
// drops the oldest conversation messages first, regardless of message count.
func TestPartitionByTokenBudget_EvictsOldestFirst(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "msg1"},
		{Role: trpcmodel.RoleAssistant, Content: "reply1"},
		{Role: trpcmodel.RoleUser, Content: "msg2"},
		{Role: trpcmodel.RoleAssistant, Content: "reply2"},
		{Role: trpcmodel.RoleUser, Content: "msg3"},
		{Role: trpcmodel.RoleAssistant, Content: "reply3"},
		{Role: trpcmodel.RoleUser, Content: "msg4"},
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, convBudgetFor(msgs, 3))
	if len(evicted) != 4 {
		t.Fatalf("expected 4 evicted, got %d", len(evicted))
	}
	convKeep := nonSystemMessages(keep)
	if len(convKeep) != 3 {
		t.Fatalf("expected 3 conversation in keep, got %d", len(convKeep))
	}
	if convKeep[0].Content != "msg3" || convKeep[1].Content != "reply3" || convKeep[2].Content != "msg4" {
		t.Fatalf("expected keep msg3,reply3,msg4, got %s,%s,%s",
			convKeep[0].Content, convKeep[1].Content, convKeep[2].Content)
	}
}

// TestPartitionByTokenBudget_NoEvictionUnderBudget verifies nothing is
// evicted when the total estimate already fits the budget.
func TestPartitionByTokenBudget_NoEvictionUnderBudget(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, 1<<20)
	if len(evicted) != 0 || len(keep) != len(msgs) {
		t.Fatalf("expected no eviction under budget, got keep=%d evicted=%d", len(keep), len(evicted))
	}
}

// TestPartitionByTokenBudget_EvictAllWhenBudgetTiny verifies a budget below
// the fixed zones evicts the entire conversation (tail cues are handled by
// the degradation chain, not this function).
func TestPartitionByTokenBudget_EvictAllWhenBudgetTiny(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "msg1"},
		{Role: trpcmodel.RoleAssistant, Content: "reply1"},
		{Role: trpcmodel.RoleUser, Content: "msg2"},
	}
	keep, evicted := partitionMessagesByTokenBudget(msgs, 0)
	if len(evicted) != 3 {
		t.Fatalf("expected all 3 conversation messages evicted, got %d", len(evicted))
	}
	if len(keep) != 1 || keep[0].Role != trpcmodel.RoleSystem {
		t.Fatalf("expected only head system kept, got %v", keep)
	}
}

func TestPartitionByTokenBudget_EmptyMessages(t *testing.T) {
	keep, evicted := partitionMessagesByTokenBudget(nil, 100)
	if len(keep) != 0 || len(evicted) != 0 {
		t.Fatalf("expected empty keep and evicted, got %d/%d", len(keep), len(evicted))
	}
}

// --- Hook tests（终审闸门：注入后计数 + token 口径截断 + 降级链） ---

func runCompressionHook(t *testing.T, hook callbacks0, ctx context.Context, args *trpcmodel.BeforeModelArgs) {
	t.Helper()
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// callbacks0 is a local alias to avoid importing the callbacks package in
// every test signature.
type callbacks0 = interface{ Priority() int }

// TestCompressionHook_FinalReviewPriority pins the hook to priority 9
// (LayerDynamic): after every injection hook (5/6) and before the prompt
// snapshot (10), so token accounting covers the fully-injected request.
func TestCompressionHook_FinalReviewPriority(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{ContextWindow: 10000}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	if got := hook.(interface{ Priority() int }).Priority(); got != 9 {
		t.Fatalf("终审闸门必须在 priority 9（注入后、snapshot 前），got %d", got)
	}
}

// TestCompressionHook_SkipsWhenRatioBelowThreshold verifies the hook does not
// modify messages when the post-injection ratio is below the hard threshold.
func TestCompressionHook_SkipsWhenRatioBelowThreshold(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{ContextWindow: 10000}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook（确定性截断无外部依赖）")
	}

	originalMsgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: originalMsgs}}
	runCompressionHook(t, hook, context.Background(), args)
	if len(args.Request.Messages) != len(originalMsgs) {
		t.Fatalf("expected messages unchanged, got %d messages", len(args.Request.Messages))
	}
}

// TestCompressionHook_TailCuesCountedInRatio is the P0-A regression: a
// request whose base messages are below threshold but whose injected tail
// cues push it over MUST truncate. (At the old priority 3 the hook measured
// only the base list; at priority 9 it measures the full injected list.)
//
// Fixture calibrated to the default estimator (2.5 chars/token, floor):
// base = sys(3) + 8×30 = 243 chars → 97 tok → 97/120 ≈ 0.81 (no trigger);
// with the 60-char tail cue: 303 chars → 121 tok → 121/120 ≈ 1.01 (trigger).
func TestCompressionHook_TailCuesCountedInRatio(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	ag := biz.Agent{ContextWindow: 120}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
	}
	for i := 0; i < 4; i++ {
		msgs = append(msgs,
			trpcmodel.Message{Role: trpcmodel.RoleUser, Content: strings.Repeat("u", 30)},
			trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: strings.Repeat("a", 30)},
		)
	}
	msgs = append(msgs, trpcmodel.Message{Role: trpcmodel.RoleSystem, Content: strings.Repeat("k", 60)}) // injected knowledge cue
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runCompressionHook(t, hook, context.Background(), args)

	hasMarker := false
	for _, m := range args.Request.Messages {
		if strings.Contains(m.Content, "<context_truncated>") {
			hasMarker = true
		}
	}
	if !hasMarker {
		t.Fatalf("注入后超阈值必须触发截断（P0-A：注入后计数），messages: %v", args.Request.Messages)
	}
}

// TestCompressionHook_MarkerAtEvictionBoundary verifies the truncation marker
// sits at the true eviction boundary: after the head system block and BEFORE
// the first surviving conversation message — never after the tail cues.
//
// Fixture: 213 chars → 85 tok → 85/82 ≈ 1.04 (trigger); target = 66 tok.
// convBudget = 66-target reserve... keeps exactly the newest conv message.
func TestCompressionHook_MarkerAtEvictionBoundary(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	ag := biz.Agent{ContextWindow: 82}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: strings.Repeat("a", 50)},
		{Role: trpcmodel.RoleAssistant, Content: strings.Repeat("b", 50)},
		{Role: trpcmodel.RoleUser, Content: strings.Repeat("c", 50)},
		{Role: trpcmodel.RoleAssistant, Content: strings.Repeat("d", 50)},
		{Role: trpcmodel.RoleSystem, Content: "memory cue"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runCompressionHook(t, hook, context.Background(), args)

	out := args.Request.Messages
	markerIdx, memCueIdx, firstConvIdx := -1, -1, -1
	for i, m := range out {
		if strings.Contains(m.Content, "<context_truncated>") {
			markerIdx = i
		}
		if m.Content == "memory cue" {
			memCueIdx = i
		}
		if firstConvIdx < 0 && m.Role != trpcmodel.RoleSystem {
			firstConvIdx = i
		}
	}
	if markerIdx < 0 {
		t.Fatalf("expected truncation marker, got %v", out)
	}
	if memCueIdx >= 0 && markerIdx > memCueIdx {
		t.Fatalf("marker 不得落在尾部 cue 之后（marker=%d cue=%d）: %v", markerIdx, memCueIdx, out)
	}
	if firstConvIdx >= 0 && markerIdx > firstConvIdx {
		t.Fatalf("marker 必须落在首个保留会话消息之前（marker=%d conv=%d）: %v", markerIdx, firstConvIdx, out)
	}
}

// TestCompressionHook_DegradationDropsTailCues is the P0-C chain: when
// evicting all history still leaves the ratio above threshold (oversized tail
// cues), the final-review gate drops tail cues (largest first) and records
// DroppedCueCount in CompressionMeta.
func TestCompressionHook_DegradationDropsTailCues(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	// window 40 → threshold 0.90 ⇒ trigger at 36 tokens (90 runes).
	// head sys(3) + 1 conv(8) + big cue(150 runes≈60 tok) + small cue(10 runes≈4 tok)
	// Even after evicting the single conv message the big cue alone busts the
	// target → the big cue must be dropped; the small one may survive.
	ag := biz.Agent{ContextWindow: 40}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "q1"},
		{Role: trpcmodel.RoleSystem, Content: strings.Repeat("k", 150)},
		{Role: trpcmodel.RoleSystem, Content: "tiny cue.."},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	ctx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{})
	runCompressionHook(t, hook, ctx, args)

	for _, m := range args.Request.Messages {
		if strings.Contains(m.Content, strings.Repeat("k", 150)) {
			t.Fatalf("超限尾部 cue 必须被降级链丢弃: %v", args.Request.Messages)
		}
	}
	meta := LoadCompressionMeta(ctx)
	if meta.DroppedCueCount == 0 {
		t.Fatalf("DroppedCueCount 应记录被丢弃的尾部 cue 数量")
	}
}

// TestCompressionHook_ReverifiedUnderTarget verifies the post-truncation
// re-check: after the gate runs, the final estimate must be under the
// truncation target (or every evictable/droppable message was removed).
func TestCompressionHook_ReverifiedUnderTarget(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	ag := biz.Agent{ContextWindow: 200}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
	}
	// 40 conversation messages × 25 runes ≈ 400 tokens total → ratio 2.0.
	for i := 0; i < 40; i++ {
		msgs = append(msgs,
			trpcmodel.Message{Role: trpcmodel.RoleUser, Content: strings.Repeat("u", 25)},
			trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: strings.Repeat("a", 25)},
		)
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runCompressionHook(t, hook, context.Background(), args)

	final := analyzePromptRequest(args.Request.Messages)
	target := int(float64(200) * 0.90 * truncationTargetFactor)
	if final.EstTokens > target {
		convLeft := 0
		for _, m := range args.Request.Messages {
			if m.Role != trpcmodel.RoleSystem {
				convLeft++
			}
		}
		if convLeft > 0 {
			t.Fatalf("复验失败：截断后 est=%d 仍超 target=%d 且仍有可驱逐历史 %d 条", final.EstTokens, target, convLeft)
		}
	}
}

// TestCompressionHook_DefaultThresholdIsHardRatio verifies the default trigger
// stays at 0.90 (hard ratio): a ratio of ~0.85 must NOT truncate.
func TestCompressionHook_DefaultThresholdIsHardRatio(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	// 51 runes total ≈ 20 est tokens / 24 window ≈ 0.83–0.88 → below 0.90.
	ag := biz.Agent{ContextWindow: 24}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "system prompt here"},
		{Role: trpcmodel.RoleUser, Content: "msg1"},
		{Role: trpcmodel.RoleAssistant, Content: "reply1"},
		{Role: trpcmodel.RoleUser, Content: "msg2"},
		{Role: trpcmodel.RoleAssistant, Content: "reply2"},
		{Role: trpcmodel.RoleUser, Content: "msg3"},
		{Role: trpcmodel.RoleAssistant, Content: "reply3"},
		{Role: trpcmodel.RoleUser, Content: "msg4"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runCompressionHook(t, hook, context.Background(), args)
	if len(args.Request.Messages) != len(msgs) {
		t.Fatalf("ratio 0.85 不应触发截断（阈值 0.90）, got %d messages (was %d)", len(args.Request.Messages), len(msgs))
	}
}

// TestCompressionHook_CustomHardTriggerRatio verifies the threshold is read
// from per-agent settings (HardTriggerRatio), mirroring CompressPolicy.
func TestCompressionHook_CustomHardTriggerRatio(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	// ratio ≈ 0.12 with window=100; custom hard ratio 0.10 → must truncate.
	ag := biz.Agent{
		ContextWindow: 100,
		Settings:      &biz.AgentRuntimeSettings{HardTriggerRatio: 0.10},
	}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "msg1"},
		{Role: trpcmodel.RoleAssistant, Content: "reply1"},
		{Role: trpcmodel.RoleUser, Content: "msg2"},
		{Role: trpcmodel.RoleAssistant, Content: "reply2"},
		{Role: trpcmodel.RoleUser, Content: "msg3"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runCompressionHook(t, hook, context.Background(), args)
	if len(args.Request.Messages) >= len(msgs) {
		t.Fatalf("custom HardTriggerRatio=0.10 应触发截断, got %d messages (was %d)", len(args.Request.Messages), len(msgs))
	}
}

// TestCompressionHook_DisabledAgentReturnsNilHook verifies the per-agent
// compression switch is respected: L0SnapshotMode=off without
// ContextCompactionEnabled disables the hook entirely.
func TestCompressionHook_DisabledAgentReturnsNilHook(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{
		ContextWindow: 10,
		Settings:      &biz.AgentRuntimeSettings{L0SnapshotMode: "off"},
	}
	if hook := newContextCompressionBeforeHook(ag, deps); hook != nil {
		t.Fatalf("压缩关闭的 agent 应返回 nil hook, got %v", hook)
	}
}

// TestCompressionHook_NilSettingsEnabledByDefault verifies that nil settings
// mean compression is enabled (default-on) and the hook is created without
// any external compressor dependency.
func TestCompressionHook_NilSettingsEnabledByDefault(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{ContextWindow: 10000}
	if hook := newContextCompressionBeforeHook(ag, deps); hook == nil {
		t.Fatalf("nil settings 默认启用，应返回非 nil hook")
	}
}

// TestCompressionHook_MarkerRecordedInMeta verifies truncation writes
// CompressionMeta (Occurred, EvictedCount) for the L0 snapshot overlay,
// with an empty SummaryText (deterministic truncation, no LLM summary).
func TestCompressionHook_MarkerRecordedInMeta(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	ag := biz.Agent{ContextWindow: 10}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "msg1"},
		{Role: trpcmodel.RoleAssistant, Content: "reply1"},
		{Role: trpcmodel.RoleUser, Content: "msg2"},
		{Role: trpcmodel.RoleAssistant, Content: "reply2"},
		{Role: trpcmodel.RoleUser, Content: "msg3"},
		{Role: trpcmodel.RoleAssistant, Content: "reply3"},
		{Role: trpcmodel.RoleUser, Content: "msg4"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	ctx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{})
	runCompressionHook(t, hook, ctx, args)
	meta := LoadCompressionMeta(ctx)
	if !meta.Occurred {
		t.Fatal("截断后 CompressionMeta.Occurred 应为 true")
	}
	if meta.EvictedCount == 0 {
		t.Fatal("EvictedCount 应记录被截断的消息数")
	}
	if meta.SummaryText != "" {
		t.Fatalf("确定性截断不应有 SummaryText, got %q", meta.SummaryText)
	}
}

// --- Helpers ---

func nonSystemMessages(msgs []trpcmodel.Message) []trpcmodel.Message {
	var out []trpcmodel.Message
	for _, m := range msgs {
		if m.Role != trpcmodel.RoleSystem {
			out = append(out, m)
		}
	}
	return out
}

// assertNoOrphanToolPairs checks that every tool-call in msgs has its
// tool-result also in msgs, and vice versa.
func assertNoOrphanToolPairs(t *testing.T, msgs []trpcmodel.Message, side string) {
	t.Helper()
	toolCallIDs := make(map[string]bool)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			toolCallIDs[tc.ID] = true
		}
	}
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			if !toolCallIDs[m.ToolID] {
				t.Fatalf("%s: orphan tool_result %q (no matching tool_call in %s)", side, m.ToolID, side)
			}
		}
	}
	toolResultIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			toolResultIDs[m.ToolID] = true
		}
	}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if !toolResultIDs[tc.ID] {
				t.Fatalf("%s: orphan tool_call %q (no matching tool_result in %s)", side, tc.ID, side)
			}
		}
	}
}
