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

// --- partitionMessagesForCompression tests ---

// TestPartitionMessagesForCompression_ToolPairSafety verifies that the
// partition never splits a tool-call / tool-result pair across the
// keep/evicted boundary. When an assistant message with ToolCalls is
// evicted, its corresponding tool-result messages must also be evicted
// (and vice versa). Splitting them causes the LLM API to reject the
// request with a 400 error (orphan tool result).
func TestPartitionMessagesForCompression_ToolPairSafety(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{{ID: "tc1", Function: trpcmodel.FunctionDefinitionParam{Name: "read"}}}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolID: "tc1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
		{Role: trpcmodel.RoleAssistant, Content: "a2"},
	}
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	// With 5 non-system messages, keepCount = ceil(5*0.30) = 2, evictCount = 3.
	// Without tool-pair awareness, evicted would contain: u1, a1(tc1), r1(tc1)
	// and keep would contain: u2, a2 — which is actually fine here.
	// Let's force a boundary split: use keepRatio=0.60 → keepCount=3, evictCount=2.
	// Without tool-pair awareness, evicted would be: u1, a1(tc1)
	// and keep would be: r1(tc1), u2, a2 — which splits the pair!
	keep2, evicted2 := partitionMessagesForCompression(msgs, 0.60)
	assertNoOrphanToolPairs(t, keep2, "keep")
	assertNoOrphanToolPairs(t, evicted2, "evicted")
	_ = keep
	_ = evicted
}

// TestPartitionMessagesForCompression_ToolPairBoundaryAtKeepSide verifies
// that when the boundary falls between a tool-call and its tool-result,
// the pair is kept together (the safer choice is to keep them in the
// recent/keep side since they are the most recent messages).
func TestPartitionMessagesForCompression_ToolPairBoundaryAtKeepSide(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{{ID: "tc1", Function: trpcmodel.FunctionDefinitionParam{Name: "read"}}}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolID: "tc1"},
		{Role: trpcmodel.RoleUser, Content: "u3"},
	}
	// Non-system: 5 messages. keepRatio=0.40 → keepCount=2, evictCount=3.
	// Without tool-pair awareness: evicted = u1, u2, a1(tc1); keep = r1(tc1), u3
	// This splits the pair! The tool result r1 is orphaned in keep.
	keep, evicted := partitionMessagesForCompression(msgs, 0.40)
	assertNoOrphanToolPairs(t, keep, "keep")
	assertNoOrphanToolPairs(t, evicted, "evicted")
}

// TestPartitionMessagesForCompression_MultipleToolCalls verifies that
// multiple tool calls from a single assistant message are all kept
// together with their results.
func TestPartitionMessagesForCompression_MultipleToolCalls(t *testing.T) {
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
	// Non-system: 5. keepRatio=0.40 → keepCount=2, evictCount=3.
	// Without tool-pair awareness: evicted = u1, a1(tc1,tc2), r1(tc1)
	// and keep = r2(tc2), u2 — splits tc2!
	keep, evicted := partitionMessagesForCompression(msgs, 0.40)
	assertNoOrphanToolPairs(t, keep, "keep")
	assertNoOrphanToolPairs(t, evicted, "evicted")
}

// TestPartitionMessagesForCompression_NoToolMessages verifies that
// the tool-pair safety logic does not affect messages without tool calls.
func TestPartitionMessagesForCompression_NoToolMessages(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleAssistant, Content: "a1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
		{Role: trpcmodel.RoleAssistant, Content: "a2"},
		{Role: trpcmodel.RoleUser, Content: "u3"},
	}
	// Non-system: 5. keepRatio=0.40 → keepCount=2, evictCount=3.
	// No tool calls → same behavior as before.
	keep, evicted := partitionMessagesForCompression(msgs, 0.40)
	if len(evicted) != 3 {
		t.Fatalf("expected 3 evicted, got %d", len(evicted))
	}
	if nonSystemCount(keep) != 2 {
		t.Fatalf("expected 2 non-system in keep, got %d", nonSystemCount(keep))
	}
}

// assertNoOrphanToolPairs checks that every tool-call in msgs has its
// tool-result also in msgs, and vice versa.
func assertNoOrphanToolPairs(t *testing.T, msgs []trpcmodel.Message, side string) {
	t.Helper()
	// Build set of all tool_call IDs in msgs.
	toolCallIDs := make(map[string]bool)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			toolCallIDs[tc.ID] = true
		}
	}
	// Check every tool result has its tool call.
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			if !toolCallIDs[m.ToolID] {
				t.Fatalf("%s: orphan tool_result %q (no matching tool_call in %s)", side, m.ToolID, side)
			}
		}
	}
	// Check every tool call has its tool result.
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

func TestPartitionMessagesForCompression_KeepsAllSystemMessages(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "identity"},
		{Role: trpcmodel.RoleSystem, Content: "instructions"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
		{Role: trpcmodel.RoleAssistant, Content: "hi"},
		{Role: trpcmodel.RoleUser, Content: "how are you"},
	}
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	// System messages must all appear in keep.
	sysCount := 0
	for _, m := range keep {
		if m.Role == trpcmodel.RoleSystem {
			sysCount++
		}
	}
	if sysCount != 2 {
		t.Fatalf("expected 2 system messages in keep, got %d", sysCount)
	}
	// Evicted should not contain system messages.
	for _, m := range evicted {
		if m.Role == trpcmodel.RoleSystem {
			t.Fatalf("evicted should not contain system messages, got %q", m.Content)
		}
	}
}

func TestPartitionMessagesForCompression_KeepsLast30Percent(t *testing.T) {
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
	// Non-system: 7 messages. Keep 30% = ceil(7 * 0.30) = 3.
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	if len(evicted) != 4 {
		t.Fatalf("expected 4 evicted, got %d", len(evicted))
	}
	// Keep should have: 1 system + 3 conversation = 4
	if got := nonSystemCount(keep); got != 3 {
		t.Fatalf("expected 3 non-system in keep, got %d", got)
	}
	// The kept conversation messages should be the LAST three: "msg3", "reply3", "msg4"
	convKeep := nonSystemMessages(keep)
	if len(convKeep) != 3 {
		t.Fatalf("expected 3 conversation in keep, got %d", len(convKeep))
	}
	if convKeep[0].Content != "msg3" || convKeep[1].Content != "reply3" || convKeep[2].Content != "msg4" {
		t.Fatalf("expected keep to be msg3,reply3,msg4, got %s,%s,%s",
			convKeep[0].Content, convKeep[1].Content, convKeep[2].Content)
	}
}

func TestPartitionMessagesForCompression_EmptyMessages(t *testing.T) {
	keep, evicted := partitionMessagesForCompression(nil, 0.30)
	if len(keep) != 0 || len(evicted) != 0 {
		t.Fatalf("expected empty keep and evicted, got %d/%d", len(keep), len(evicted))
	}
}

func TestPartitionMessagesForCompression_OnlySystemMessages(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys1"},
		{Role: trpcmodel.RoleSystem, Content: "sys2"},
	}
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	if len(keep) != 2 {
		t.Fatalf("expected 2 keep, got %d", len(keep))
	}
	if len(evicted) != 0 {
		t.Fatalf("expected 0 evicted, got %d", len(evicted))
	}
}

func TestPartitionMessagesForCompression_FewConversationMessages(t *testing.T) {
	// Only 1 conversation message — keep it, don't evict.
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "only msg"},
	}
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	if len(evicted) != 0 {
		t.Fatalf("expected 0 evicted for single conversation message, got %d", len(evicted))
	}
	if len(keep) != 2 {
		t.Fatalf("expected 2 keep, got %d", len(keep))
	}
}

// --- Hook tests（方案 2：确定性紧急截断，无 LLM） ---

// TestCompressionHook_SkipsWhenRatioBelowThreshold verifies that the hook
// does not modify messages when the ratio is below the hard threshold.
func TestCompressionHook_SkipsWhenRatioBelowThreshold(t *testing.T) {
	deps := TRPCBuilderDeps{}
	// Large context window → ratio well below 0.90 hard threshold.
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
	result, err := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	}).HandleBeforeModel(context.Background(), args)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
	if len(args.Request.Messages) != len(originalMsgs) {
		t.Fatalf("expected messages unchanged, got %d messages", len(args.Request.Messages))
	}
}

// TestCompressionHook_TruncatesWhenRatioAboveHardThreshold verifies that the
// hook deterministically drops old messages and inserts a truncation marker
// when the ratio crosses the hard threshold — no LLM call, no summary.
func TestCompressionHook_TruncatesWhenRatioAboveHardThreshold(t *testing.T) {
	deps := TRPCBuilderDeps{
		TRPCExtensionDeps: TRPCExtensionDeps{
			LG: loggateway.NewNoop(),
		},
	}
	// Very small context window → ratio exceeds 0.90 hard threshold.
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
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Fewer messages than original (old ones dropped).
	if len(args.Request.Messages) >= len(msgs) {
		t.Fatalf("expected fewer messages after truncation, got %d (was %d)", len(args.Request.Messages), len(msgs))
	}
	// Truncation marker present; LLM summary block absent.
	hasMarker := false
	for _, m := range args.Request.Messages {
		if strings.Contains(m.Content, "<context_truncated>") {
			hasMarker = true
		}
		if strings.Contains(m.Content, "<context_summary>") {
			t.Fatalf("确定性截断不应产生 LLM 摘要块: %q", m.Content)
		}
	}
	if !hasMarker {
		t.Fatalf("expected <context_truncated> marker, got: %v", args.Request.Messages)
	}
	// Tool-pair integrity on the surviving messages.
	assertNoOrphanToolPairs(t, args.Request.Messages, "truncated")
}

// TestCompressionHook_DefaultThresholdIsHardRatio verifies the default trigger
// moved from 0.80 (old MemoryRuntimePolicy) to 0.90 (hard trigger): a ratio of
// ~0.85 must NOT truncate anymore.
func TestCompressionHook_DefaultThresholdIsHardRatio(t *testing.T) {
	deps := TRPCBuilderDeps{TRPCExtensionDeps: TRPCExtensionDeps{LG: loggateway.NewNoop()}}
	// 51 runes total ≈ 20 est tokens / 24 window ≈ 0.83–0.88 → above old 0.80, below new 0.90.
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
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(args.Request.Messages) != len(msgs) {
		t.Fatalf("ratio 0.85 不应触发截断（新阈值 0.90）, got %d messages (was %d)", len(args.Request.Messages), len(msgs))
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
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
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
// mean compression is enabled (default-on) and the hook is created without any
// external compressor dependency.
func TestCompressionHook_NilSettingsEnabledByDefault(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{ContextWindow: 10000}
	if hook := newContextCompressionBeforeHook(ag, deps); hook == nil {
		t.Fatalf("nil settings 默认启用，应返回非 nil hook")
	}
}

// TestCompressionHook_MarkerRecordedInMeta verifies truncation writes
// CompressionMeta (Occurred, EvictedCount) for the L0 snapshot overlay,
// with an empty SummaryText (no LLM summary anymore).
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
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	ctx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{})
	if _, err := hookFn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
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

func nonSystemCount(msgs []trpcmodel.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role != trpcmodel.RoleSystem {
			n++
		}
	}
	return n
}

func nonSystemMessages(msgs []trpcmodel.Message) []trpcmodel.Message {
	var out []trpcmodel.Message
	for _, m := range msgs {
		if m.Role != trpcmodel.RoleSystem {
			out = append(out, m)
		}
	}
	return out
}
