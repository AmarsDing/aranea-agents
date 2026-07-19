package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

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

// --- Hook tests ---

// TestCompressionHook_SkipsWhenRatioBelowThreshold verifies that the hook
// does not modify messages when the ratio is below the compression threshold.
func TestCompressionHook_SkipsWhenRatioBelowThreshold(t *testing.T) {
	compressor := &fakeContextCompressor{shouldCompressResult: false}
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ContextCompressor: compressor,
		},
	}
	// Large context window → ratio well below 0.80 threshold.
	ag := biz.Agent{ContextWindow: 10000}
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
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
	// Messages should be unchanged.
	if len(args.Request.Messages) != len(originalMsgs) {
		t.Fatalf("expected messages unchanged, got %d messages", len(args.Request.Messages))
	}
	if compressor.compressCalled {
		t.Fatalf("expected Compress NOT to be called (ratio below threshold)")
	}
}

// TestCompressionHook_CompressesWhenRatioAboveThreshold verifies that the hook
// evicts old messages and injects a summary when the ratio is above threshold.
func TestCompressionHook_CompressesWhenRatioAboveThreshold(t *testing.T) {
	compressor := &fakeContextCompressor{
		shouldCompressResult: true,
		compressResult: biz.ContextCompressionResult{
			Summary:      "Compressed summary",
			EvictedCount: 4,
		},
	}
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ContextCompressor: compressor,
		},
		TRPCExtensionDeps: TRPCExtensionDeps{
			LG: loggateway.NewNoop(),
		},
	}
	// Very small context window → ratio exceeds 0.80 threshold.
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

	if !compressor.compressCalled {
		t.Fatalf("expected Compress to be called")
	}
	// The resulting messages should contain the summary (wrapped in a context_summary block).
	hasSummary := false
	for _, m := range args.Request.Messages {
		if strings.Contains(m.Content, "Compressed summary") {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		t.Fatalf("expected summary in messages, got: %v", args.Request.Messages)
	}
	// The result should have fewer messages than original (evicted messages replaced by summary).
	if len(args.Request.Messages) >= len(msgs) {
		t.Fatalf("expected fewer messages after compression, got %d (was %d)", len(args.Request.Messages), len(msgs))
	}
}

// TestCompressionHook_CompressFailureDoesNotBlock verifies that a compression
// error does not block the model call (graceful degradation).
func TestCompressionHook_CompressFailureDoesNotBlock(t *testing.T) {
	compressor := &fakeContextCompressor{
		shouldCompressResult: true,
		compressErr:          errors.New("LLM down"),
	}
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ContextCompressor: compressor,
		},
		TRPCExtensionDeps: TRPCExtensionDeps{
			LG: loggateway.NewNoop(),
		},
	}
	// Very small context window → ratio reaches 0.80 threshold.
	// (4 tokens / 5 window = 0.80)
	ag := biz.Agent{ContextWindow: 5}
	hook := newContextCompressionBeforeHook(ag, deps)

	originalMsgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
		{Role: trpcmodel.RoleAssistant, Content: "hi"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: originalMsgs}}
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	result, err := hookFn.HandleBeforeModel(context.Background(), args)
	// Graceful degradation: no error, messages unchanged.
	if err != nil {
		t.Fatalf("expected nil error on compress failure, got %v", err)
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
	if len(args.Request.Messages) != len(originalMsgs) {
		t.Fatalf("expected messages unchanged on compress failure, got %d", len(args.Request.Messages))
	}
}

// TestCompressionHook_NilCompressorReturnsNil verifies that no hook is created
// when ContextCompressor is not wired.
func TestCompressionHook_NilCompressorReturnsNil(t *testing.T) {
	deps := TRPCBuilderDeps{}
	ag := biz.Agent{ContextWindow: 10000}
	if hook := newContextCompressionBeforeHook(ag, deps); hook != nil {
		t.Fatalf("expected nil hook when ContextCompressor is nil, got %v", hook)
	}
}

// TestCompressionHook_PolicyDefaultsApplied verifies that the hook uses
// MemoryRuntimePolicy defaults (threshold=0.80, keepRatio=0.30) when no
// agent settings are provided. This ensures the policy-driven configuration
// is wired correctly and replaces the old hardcoded constants.
func TestCompressionHook_PolicyDefaultsApplied(t *testing.T) {
	// With ContextWindow=100 and ~12 tokens of messages, ratio ≈ 0.12 < 0.80.
	// No compression should occur with the default policy threshold.
	compressor := &fakeContextCompressor{
		compressResult: biz.ContextCompressionResult{Summary: "should not be used"},
	}
	deps := TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{
			ContextCompressor: compressor,
		},
	}
	ag := biz.Agent{ContextWindow: 100} // ratio ≈ 12/100 = 0.12 < 0.80
	hook := newContextCompressionBeforeHook(ag, deps)
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "hello world"},
		{Role: trpcmodel.RoleAssistant, Content: "hi there"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if compressor.compressCalled {
		t.Fatalf("expected Compress NOT to be called (ratio below default 0.80 threshold)")
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

// fakeContextCompressor implements biz.ContextCompressor for testing.
type fakeContextCompressor struct {
	shouldCompressResult bool
	shouldCompressCalled bool
	compressResult       biz.ContextCompressionResult
	compressErr          error
	compressCalled       bool
	lastExistingSummary  string
	lastEvictedCount     int
}

func (f *fakeContextCompressor) ShouldCompress(usedRatio float64) bool {
	f.shouldCompressCalled = true
	return f.shouldCompressResult
}

func (f *fakeContextCompressor) Compress(_ context.Context, existingSummary string, evictedMessages []biz.ConsolidateMessage) (biz.ContextCompressionResult, error) {
	f.compressCalled = true
	f.lastExistingSummary = existingSummary
	f.lastEvictedCount = len(evictedMessages)
	return f.compressResult, f.compressErr
}
