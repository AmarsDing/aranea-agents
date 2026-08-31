package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// N2 硬约束（session-eval-20260829-r2）：next_action 指令的跟踪/催办/核销矩阵。

type afterToolHookInvoker interface {
	HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error)
}

type beforeModelHookInvoker interface {
	HandleBeforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
}

func invokeAfterTool(t *testing.T, cb callbacks.Callback, ctx context.Context, args *trpctool.AfterToolArgs) {
	t.Helper()
	h, ok := cb.(afterToolHookInvoker)
	if !ok {
		t.Fatal("hook is not an AfterToolHook")
	}
	if _, err := h.HandleAfterTool(ctx, args); err != nil {
		t.Fatalf("after tool hook: %v", err)
	}
}

func invokeBeforeModel(t *testing.T, cb callbacks.Callback, ctx context.Context, args *trpcmodel.BeforeModelArgs) {
	t.Helper()
	h, ok := cb.(beforeModelHookInvoker)
	if !ok {
		t.Fatal("hook is not a BeforeModelHook")
	}
	if _, err := h.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("before model hook: %v", err)
	}
}

func nextActionTestCtx() (context.Context, *trpcagent.Invocation) {
	inv := &trpcagent.Invocation{}
	return trpcagent.NewInvocationContext(context.Background(), inv), inv
}

func pendingFromState(t *testing.T, inv *trpcagent.Invocation) pendingNextAction {
	t.Helper()
	v, found := inv.GetState(pendingNextActionStateKey)
	if !found {
		return pendingNextAction{}
	}
	p, _ := v.(pendingNextAction)
	return p
}

func TestNextActionTrackAfterHook(t *testing.T) {
	hook := newNextActionTrackAfterHook()

	t.Run("roster_miss_sets_pending", func(t *testing.T) {
		ctx, inv := nextActionTestCtx()
		invokeAfterTool(t, hook, ctx, &trpctool.AfterToolArgs{
			ToolName: planAndExecuteToolName,
			Result:   map[string]any{"next_action": "build_orchestration_graph", "reuse_reason": "花名册无匹配"},
		})
		p := pendingFromState(t, inv)
		if p.Tool != "build_orchestration_graph" || p.Hint != "花名册无匹配" || p.Nudged {
			t.Fatalf("unexpected pending: %+v", p)
		}
	})

	t.Run("user_facing_next_action_not_tracked", func(t *testing.T) {
		ctx, inv := nextActionTestCtx()
		invokeAfterTool(t, hook, ctx, &trpctool.AfterToolArgs{
			ToolName: planAndExecuteToolName,
			Result:   map[string]any{"next_action": "await_user_clarification"},
		})
		if p := pendingFromState(t, inv); p.Tool != "" {
			t.Fatalf("user-facing next_action must not be tracked, got %+v", p)
		}
	})

	t.Run("required_tool_call_clears_pending", func(t *testing.T) {
		ctx, inv := nextActionTestCtx()
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: "build_orchestration_graph", Hint: "h"})
		invokeAfterTool(t, hook, ctx, &trpctool.AfterToolArgs{ToolName: "build_orchestration_graph"})
		if p := pendingFromState(t, inv); p.Tool != "" {
			t.Fatalf("pending must be cleared after required tool call, got %+v", p)
		}
	})

	t.Run("unrelated_tool_keeps_pending", func(t *testing.T) {
		ctx, inv := nextActionTestCtx()
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: "build_orchestration_graph"})
		invokeAfterTool(t, hook, ctx, &trpctool.AfterToolArgs{ToolName: "list_agent_sessions"})
		if p := pendingFromState(t, inv); p.Tool != "build_orchestration_graph" {
			t.Fatalf("pending must survive unrelated tool, got %+v", p)
		}
	})
}

func TestNextActionCueBeforeHook(t *testing.T) {
	hook := newNextActionCueBeforeHook(TRPCBuilderDeps{})

	t.Run("no_pending_no_cue", func(t *testing.T) {
		ctx, _ := nextActionTestCtx()
		args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{trpcmodel.NewUserMessage("u")}}}
		invokeBeforeModel(t, hook, ctx, args)
		for _, m := range args.Request.Messages {
			if strings.Contains(m.Content, nextActionCueMarker) {
				t.Fatal("cue must not be injected without pending directive")
			}
		}
	})

	t.Run("pending_injects_mandatory_cue", func(t *testing.T) {
		ctx, inv := nextActionTestCtx()
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: "build_orchestration_graph", Hint: "改道显式组队"})
		args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{trpcmodel.NewUserMessage("u")}}}
		invokeBeforeModel(t, hook, ctx, args)
		found := false
		for _, m := range args.Request.Messages {
			if strings.HasPrefix(m.Content, nextActionCueMarker) && strings.Contains(m.Content, "build_orchestration_graph") {
				found = true
			}
		}
		if !found {
			t.Fatal("mandatory cue must be injected while pending")
		}
	})
}

func TestNextActionRouteAfterModelHook(t *testing.T) {
	newHook := func() callbacks.Callback {
		return newNextActionRouteAfterModelHook(TRPCBuilderDeps{})
	}
	pendingCtx := func() (context.Context, *trpcagent.Invocation) {
		ctx, inv := nextActionTestCtx()
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: "build_orchestration_graph", Hint: "h"})
		return ctx, inv
	}

	t.Run("no_pending_noop", func(t *testing.T) {
		ctx, _ := nextActionTestCtx()
		res := invokeForcePlanningHook(t, newHook(), ctx, &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Messages: []trpcmodel.Message{}},
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("no pending directive must not hard-route")
		}
	})

	t.Run("final_answer_synthesizes_tool_load", func(t *testing.T) {
		ctx, inv := pendingCtx()
		res := invokeForcePlanningHook(t, newHook(), ctx, &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Messages: []trpcmodel.Message{}},
			Response: finalTextResponse(),
		})
		if res == nil || res.CustomResponse == nil {
			t.Fatal("final answer with pending directive must be replaced by tool_load call")
		}
		tc := res.CustomResponse.Choices[0].Message.ToolCalls
		if len(tc) != 1 || tc[0].Function.Name != "tool_load" {
			t.Fatalf("expected synthetic tool_load call, got %+v", tc)
		}
		if !strings.Contains(string(tc[0].Function.Arguments), "build_orchestration_graph") {
			t.Fatalf("tool_load args must target required tool, got %s", tc[0].Function.Arguments)
		}
		if p := pendingFromState(t, inv); !p.Nudged {
			t.Fatal("pending must be marked nudged after synthetic nudge")
		}
	})

	t.Run("second_abandon_passes_through", func(t *testing.T) {
		ctx, inv := pendingCtx()
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: "build_orchestration_graph", Nudged: true})
		res := invokeForcePlanningHook(t, newHook(), ctx, &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Messages: []trpcmodel.Message{}},
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("already-nudged pending must pass through (no infinite loop)")
		}
		if p := pendingFromState(t, inv); p.Tool != "" {
			t.Fatal("pending must be cleared after abandon")
		}
	})

	t.Run("required_tool_in_history_clears_and_passes", func(t *testing.T) {
		ctx, inv := pendingCtx()
		msgs := []trpcmodel.Message{{
			Role:      trpcmodel.RoleAssistant,
			ToolCalls: []trpcmodel.ToolCall{{Type: "function", Function: trpcmodel.FunctionDefinitionParam{Name: "build_orchestration_graph"}}},
		}}
		res := invokeForcePlanningHook(t, newHook(), ctx, &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Messages: msgs},
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("required tool already called must pass through")
		}
		if p := pendingFromState(t, inv); p.Tool != "" {
			t.Fatal("pending must be cleared when required tool found in history")
		}
	})

	t.Run("tool_call_response_noop", func(t *testing.T) {
		ctx, _ := pendingCtx()
		resp := finalTextResponse()
		resp.Choices[0].Message.ToolCalls = []trpcmodel.ToolCall{{Type: "function", Function: trpcmodel.FunctionDefinitionParam{Name: "memory_search"}}}
		res := invokeForcePlanningHook(t, newHook(), ctx, &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Messages: []trpcmodel.Message{}},
			Response: resp,
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("tool-call response must not be intercepted")
		}
	})
}
