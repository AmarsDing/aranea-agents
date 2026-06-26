package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// TestProcessEvent_streamingReplyFinalChunkEmpty_preservesAccumulatedContent
// verifies that when a streaming reply's final non-partial chunk carries no
// text, the accumulated reply content is preserved instead of being overwritten
// with an empty string.
//
// Symptom: frontend shows an empty reply bubble even though deltas arrived.
func TestProcessEvent_streamingReplyFinalChunkEmpty_preservesAccumulatedContent(t *testing.T) {
	p, bus, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	// Partial chunks accumulate content.
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: "Hello "}}},
		},
	})
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: "world"}}},
		},
	})
	// Final non-partial chunk with empty text (common OpenAI-style terminator).
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: false,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: ""}}},
		},
	})

	// Drain the sequencer.
	p.Close()

	// Find the reply activity in the repo.
	var reply *biz.Activity
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindReply {
			reply = &a
			break
		}
	}
	if reply == nil {
		t.Fatal("no reply activity persisted")
	}
	if reply.Content != "Hello world" {
		t.Errorf("reply content=%q want %q", reply.Content, "Hello world")
	}
	if reply.Status != biz.ActivityStatusCompleted {
		t.Errorf("reply status=%q want %q", reply.Status, biz.ActivityStatusCompleted)
	}

	// The done event must also carry the preserved content.
	evs := bus.published
	var doneEv *biz.ActivityEvent
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Event == biz.ActivityEventCompleted {
			if string(evs[i].Activity.Kind) == string(biz.ActivityKindReply) {
				doneEv = &evs[i]
				break
			}
		}
	}
	if doneEv == nil {
		t.Fatal("no activity_done event for reply")
	}
	if doneEv.Activity.Content != "Hello world" {
		t.Errorf("done event content=%q want %q", doneEv.Activity.Content, "Hello world")
	}
}

// TestProcessEvent_streamingToolCallDeltas_createSingleActivity verifies that
// a tool call streamed across multiple partial deltas creates exactly one
// action Activity and accumulates the arguments, instead of creating orphaned
// running activities for each delta.
func TestProcessEvent_streamingToolCallDeltas_createSingleActivity(t *testing.T) {
	p, bus, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	// OpenAI-style tool call streaming: id/name in first delta, arguments in later deltas.
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						ID:   "call_123",
						Type: "function",
						Function: trpcmodel.FunctionDefinitionParam{
							Name: "read_file",
						},
					}},
				},
			}},
		},
	})
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						ID:   "call_123",
						Type: "function",
						Function: trpcmodel.FunctionDefinitionParam{
							Arguments: []byte(`{"path":"/tmp/x"}`),
						},
					}},
				},
			}},
		},
	})

	p.Close()

	// Count action activities in the repo.
	var actions []biz.Activity
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindAction {
			actions = append(actions, a)
		}
	}
	if len(actions) != 1 {
		t.Fatalf("want 1 action activity, got %d", len(actions))
	}
	if actions[0].ToolName != "read_file" {
		t.Errorf("tool name=%q want %q", actions[0].ToolName, "read_file")
	}
	if actions[0].ToolArguments != `{"path":"/tmp/x"}` {
		t.Errorf("tool arguments=%q want %q", actions[0].ToolArguments, `{"path":"/tmp/x"}`)
	}

	// Only one activity_start for action kind should have been published.
	startCount := 0
	for _, ev := range bus.published {
		if ev.Event == biz.ActivityEventCreated {
			if string(ev.Activity.Kind) == string(biz.ActivityKindAction) {
				startCount++
			}
		}
	}
	if startCount != 1 {
		t.Errorf("action activity_start count=%d want 1", startCount)
	}
}

// TestProcessEvent_toolResult_notDoubleEncoded verifies that tool result
// content is stored as the raw string returned by the runtime, not as a
// JSON-marshaled string (which would double-encode it).
func TestProcessEvent_toolResult_notDoubleEncoded(t *testing.T) {
	p, _, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	// Create the tool call first so OnToolResult can find it.
	p.OnToolCall(ctx, "call_123", "read_file", `{"path":"/tmp/x"}`, "agent-1", timeNow())
	// Runtime returns a JSON string as msg.Content.
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeToolResponse,
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					ToolID:  "call_123",
					Content: `{"path":"/tmp/x","content":"hello"}`,
				},
			}},
		},
	})

	p.Close()

	var action *biz.Activity
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindAction {
			action = &a
			break
		}
	}
	if action == nil {
		t.Fatal("no action activity persisted")
	}

	// The stored result must be the raw JSON object string, not a JSON-encoded string.
	want := `{"path":"/tmp/x","content":"hello"}`
	if action.ToolResult != want {
		t.Errorf("tool result=%q want %q", action.ToolResult, want)
	}

	// It must be valid JSON that decodes into an object.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(action.ToolResult), &parsed); err != nil {
		t.Errorf("tool result is not valid JSON: %v", err)
	}
}

// TestProcessEvent_streamingReasoningFinalizedWhenFinalChunkEmpty verifies that
// a streaming thinking activity is finalized even when the final non-partial
// chunk carries no reasoning content. The accumulated reasoning buffer must be
// used for the final activity state.
func TestProcessEvent_streamingReasoningFinalizedWhenFinalChunkEmpty(t *testing.T) {
	p, bus, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{ReasoningContent: "step 1 "}}},
		},
	})
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{ReasoningContent: "step 2"}}},
		},
	})
	// Final chunk with empty reasoning (common terminator).
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: false,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{ReasoningContent: ""}}},
		},
	})

	p.Close()

	var thinking *biz.Activity
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindThinking {
			thinking = &a
			break
		}
	}
	if thinking == nil {
		t.Fatal("no thinking activity persisted")
	}
	if thinking.Status != biz.ActivityStatusCompleted {
		t.Errorf("thinking status=%q want %q", thinking.Status, biz.ActivityStatusCompleted)
	}
	wantReasoning := "step 1 step 2"
	if thinking.Reasoning != wantReasoning {
		t.Errorf("thinking reasoning=%q want %q", thinking.Reasoning, wantReasoning)
	}

	// Verify a done event was published with the full reasoning.
	doneCount := 0
	for _, ev := range bus.published {
		if ev.Event == biz.ActivityEventCompleted {
			if string(ev.Activity.Kind) == string(biz.ActivityKindThinking) {
				doneCount++
				if ev.Activity.Reasoning != wantReasoning {
					t.Errorf("done event reasoning=%q want %q", ev.Activity.Reasoning, wantReasoning)
				}
			}
		}
	}
	if doneCount != 1 {
		t.Errorf("thinking activity_done count=%d want 1", doneCount)
	}
}

// TestOnTurnEnd_finalizesChildActivities verifies that OnTurnEnd marks any
// still-running child activities (thinking/reply/action) as completed, so the
// frontend does not leave them in streaming state forever.
func TestOnTurnEnd_finalizesChildActivities(t *testing.T) {
	p, bus, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	p.OnTurnStart(ctx, ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"})
	p.OnTextDelta(ctx, "agent-1", "reply content")
	p.OnReasoningDelta(ctx, "agent-1", "reasoning content", true)
	p.OnToolCall(ctx, "call_123", "read_file", `{"path":"/tmp/x"}`, "agent-1", timeNow())

	p.OnTurnEnd(ctx, nil)
	p.Close()

	checkCompleted := func(kind biz.ActivityKind, name string) {
		t.Helper()
		for _, a := range repo.activities {
			if a.Kind == kind && a.Status != biz.ActivityStatusCompleted {
				t.Errorf("%s activity status=%q want %q", name, a.Status, biz.ActivityStatusCompleted)
			}
		}
	}
	checkCompleted(biz.ActivityKindReply, "reply")
	checkCompleted(biz.ActivityKindThinking, "thinking")
	checkCompleted(biz.ActivityKindAction, "action")

	// Each child activity should have a done event.
	doneKinds := make(map[biz.ActivityKind]int)
	for _, ev := range bus.published {
		if ev.Event == biz.ActivityEventCompleted {
			switch string(ev.Activity.Kind) {
			case string(biz.ActivityKindReply):
				doneKinds[biz.ActivityKindReply]++
			case string(biz.ActivityKindThinking):
				doneKinds[biz.ActivityKindThinking]++
			case string(biz.ActivityKindAction):
				doneKinds[biz.ActivityKindAction]++
			}
		}
	}
	if doneKinds[biz.ActivityKindReply] != 1 {
		t.Errorf("reply done events=%d want 1", doneKinds[biz.ActivityKindReply])
	}
	if doneKinds[biz.ActivityKindThinking] != 1 {
		t.Errorf("thinking done events=%d want 1", doneKinds[biz.ActivityKindThinking])
	}
}

// TestProcessEvent_finalChunkTextWithoutPriorDelta_createsReply verifies that
// when the model only emits the reply text in the final non-partial chunk
// (after reasoning/tool calls, for example), a reply Activity is still created
// and the UI has a completed reply to render.
func TestProcessEvent_finalChunkTextWithoutPriorDelta_createsReply(t *testing.T) {
	p, bus, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}, nil)
	ctx := context.Background()

	// Only a final chunk with text, no prior content deltas.
	p.ProcessEvent(ctx, &trpcevent.Event{
		Author: "agent-1",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: false,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: "final answer"}}},
		},
	})
	p.Close()

	var reply *biz.Activity
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindReply {
			reply = &a
			break
		}
	}
	if reply == nil {
		t.Fatal("no reply activity persisted")
	}
	if reply.Content != "final answer" {
		t.Errorf("reply content=%q want %q", reply.Content, "final answer")
	}
	if reply.Status != biz.ActivityStatusCompleted {
		t.Errorf("reply status=%q want %q", reply.Status, biz.ActivityStatusCompleted)
	}

	// Must emit start before done (B-05 ordering invariant).
	startIdx, doneIdx := -1, -1
	for i, ev := range bus.published {
		if ev.Event != biz.ActivityEventCreated && ev.Event != biz.ActivityEventCompleted {
			continue
		}
		if string(ev.Activity.Kind) != string(biz.ActivityKindReply) {
			continue
		}
		if ev.Event == biz.ActivityEventCreated {
			startIdx = i
		}
		if ev.Event == biz.ActivityEventCompleted {
			doneIdx = i
		}
	}
	if startIdx == -1 {
		t.Error("missing reply activity_start event")
	}
	if doneIdx == -1 {
		t.Error("missing reply activity_done event")
	}
	if startIdx > doneIdx {
		t.Errorf("reply start(%d) after done(%d)", startIdx, doneIdx)
	}
}

// --- D3: OnError semantics + OnTurnEnd terminal state protection ---

// findRootTask returns the persisted root task Activity (kind=task) from the
// mock repo, or nil if none exists.
func findRootTask(repo *mockActivityWriter) *biz.Activity {
	for _, a := range repo.activities {
		if a.Kind == biz.ActivityKindTask {
			return &a
		}
	}
	return nil
}

// TestOnError_RootTaskExists_TransitionsToFailed verifies that when OnError
// fires after OnTurnStart, the root task Activity transitions to status=failed
// and the error classification is stored in Meta (D3 Case 1).
func TestOnError_RootTaskExists_TransitionsToFailed(t *testing.T) {
	p, _, repo := newTestProjector(t)
	meta := ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1", AgentDisplayName: "Agent"}
	p.Configure(meta, nil)
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	p.OnError(ctx, "model rate limited", "RateLimitError", "429")
	p.Close()

	root := findRootTask(repo)
	if root == nil {
		t.Fatal("no root task activity persisted")
	}
	if root.Status != biz.ActivityStatusFailed {
		t.Errorf("root status=%q want %q", root.Status, biz.ActivityStatusFailed)
	}
	if root.Meta["error_message"] != "model rate limited" {
		t.Errorf("error_message=%v want %q", root.Meta["error_message"], "model rate limited")
	}
	if root.Meta["error_type"] != "RateLimitError" {
		t.Errorf("error_type=%v want %q", root.Meta["error_type"], "RateLimitError")
	}
	if root.Meta["error_code"] != "429" {
		t.Errorf("error_code=%v want %q", root.Meta["error_code"], "429")
	}
	// Note: DurationMs is computed via the same now.Sub(a.Timestamp).Milliseconds()
	// pattern used at 12 other call sites (OnTurnEnd/OnToolEnd/OnNotice/...). It is
	// not asserted here because unit tests execute in <1ms, causing the integer
	// truncation to 0 — making a >0 assertion flaky without artificial sleeps.
}

// TestOnError_NoRootTask_CreatesMinimalFailedTask verifies that when OnError
// fires without a prior OnTurnStart, a minimal failed task Activity is created
// so the error is still surfaced to the frontend (D3 Case 2).
func TestOnError_NoRootTask_CreatesMinimalFailedTask(t *testing.T) {
	p, _, repo := newTestProjector(t)
	p.Configure(ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1", AgentDisplayName: "Agent"}, nil)
	ctx := context.Background()

	// OnError without OnTurnStart — no root task exists.
	p.OnError(ctx, "init failure", "", "")
	p.Close()

	root := findRootTask(repo)
	if root == nil {
		t.Fatal("no minimal failed task created")
	}
	if root.Status != biz.ActivityStatusFailed {
		t.Errorf("root status=%q want %q", root.Status, biz.ActivityStatusFailed)
	}
	if root.Kind != biz.ActivityKindTask {
		t.Errorf("root kind=%q want %q", root.Kind, biz.ActivityKindTask)
	}
	if root.Content != "init failure" {
		t.Errorf("root content=%q want %q", root.Content, "init failure")
	}
	if root.Meta["error_message"] != "init failure" {
		t.Errorf("error_message=%v want %q", root.Meta["error_message"], "init failure")
	}
	// errType and errCode are empty — Meta should not contain empty keys.
	if _, ok := root.Meta["error_type"]; ok {
		t.Errorf("error_type should be absent when empty")
	}
}

// TestOnTurnEnd_TerminalStateProtection verifies that OnTurnEnd does NOT
// overwrite a Failed root task with Completed (D3 terminal guard). Token
// usage is still attached even for terminal states.
func TestOnTurnEnd_TerminalStateProtection(t *testing.T) {
	p, _, repo := newTestProjector(t)
	meta := ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}
	p.Configure(meta, nil)
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	p.OnError(ctx, "turn failed", "InternalError", "500")

	// OnTurnEnd must not overwrite the Failed status.
	p.OnTurnEnd(ctx, &ActivityUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	p.Close()

	root := findRootTask(repo)
	if root == nil {
		t.Fatal("no root task activity persisted")
	}
	if root.Status != biz.ActivityStatusFailed {
		t.Errorf("root status=%q want %q (terminal state must not be overwritten by OnTurnEnd)",
			root.Status, biz.ActivityStatusFailed)
	}
	// Token usage should still be attached (OnTurnEnd attaches usage even for
	// terminal states, via ActivityEventUpdated).
	if root.PromptTokens != 100 {
		t.Errorf("PromptTokens=%d want 100 (usage should be attached to terminal state)", root.PromptTokens)
	}
	if root.CompletionTokens != 50 {
		t.Errorf("CompletionTokens=%d want 50", root.CompletionTokens)
	}
}

// TestOnTurnEnd_NormalCompletion_TransitionsToCompleted verifies that without
// a prior error, OnTurnEnd transitions the root to Completed (the non-terminal
// happy path, ensuring the terminal guard doesn't block normal completion).
func TestOnTurnEnd_NormalCompletion_TransitionsToCompleted(t *testing.T) {
	p, _, repo := newTestProjector(t)
	meta := ProjectMeta{SessionID: "sess-1", RequestID: "turn-1", AgentID: "agent-1"}
	p.Configure(meta, nil)
	ctx := context.Background()

	p.OnTurnStart(ctx, meta)
	p.OnTurnEnd(ctx, &ActivityUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30})
	p.Close()

	root := findRootTask(repo)
	if root == nil {
		t.Fatal("no root task activity persisted")
	}
	if root.Status != biz.ActivityStatusCompleted {
		t.Errorf("root status=%q want %q (normal completion)", root.Status, biz.ActivityStatusCompleted)
	}
	if root.PromptTokens != 10 {
		t.Errorf("PromptTokens=%d want 10", root.PromptTokens)
	}
}

func timeNow() time.Time {
	return time.Now().UTC()
}
