package agent

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz/decision"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TestForcePlanningRouteCtx pins the ctx-key contract: the service layer marks
// the turn ctx when the pre-planning gate forces planning (SP-2a), and the
// AfterModel hook reads it back. Empty TaskPrompt must not mark.
func TestForcePlanningRouteCtx(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := ContextWithForcePlanningRoute(context.Background(), ForcePlanningRoute{
			TaskPrompt: "写一份竞品分析报告",
			Level:      "complex",
			Score:      0.9,
			Reason:     "测试",
		})
		route, ok := ForcePlanningRouteFromCtx(ctx)
		if !ok {
			t.Fatal("expected route marker present")
		}
		if route.TaskPrompt != "写一份竞品分析报告" || route.Level != "complex" || route.Score != 0.9 {
			t.Fatalf("unexpected route: %+v", route)
		}
	})
	t.Run("empty_task_prompt_not_marked", func(t *testing.T) {
		ctx := ContextWithForcePlanningRoute(context.Background(), ForcePlanningRoute{TaskPrompt: "  "})
		if _, ok := ForcePlanningRouteFromCtx(ctx); ok {
			t.Fatal("empty task prompt must not mark ctx")
		}
	})
	t.Run("absent", func(t *testing.T) {
		if _, ok := ForcePlanningRouteFromCtx(context.Background()); ok {
			t.Fatal("unmarked ctx must report absent")
		}
	})
}

type afterModelHookInvoker interface {
	HandleAfterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error)
}

func invokeForcePlanningHook(t *testing.T, cb callbacks.Callback, ctx context.Context, args *trpcmodel.AfterModelArgs) *trpcmodel.AfterModelResult {
	t.Helper()
	h, ok := cb.(afterModelHookInvoker)
	if !ok {
		t.Fatal("hook is not an AfterModelHook")
	}
	res, err := h.HandleAfterModel(ctx, args)
	if err != nil {
		t.Fatalf("hook returned error: %v", err)
	}
	return res
}

func markedCtx() context.Context {
	return ContextWithForcePlanningRoute(context.Background(), ForcePlanningRoute{
		TaskPrompt: "写一份竞品分析报告",
		Level:      "complex",
		Score:      0.9,
		Reason:     "评估完成：复杂任务，强制走规划路径",
	})
}

func finalTextResponse() *trpcmodel.Response {
	return &trpcmodel.Response{
		ID:     "resp-1",
		Object: trpcmodel.ObjectTypeChatCompletion,
		Done:   true,
		Usage:  &trpcmodel.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		Choices: []trpcmodel.Choice{{
			Index:   0,
			Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "这是直接答案"},
		}},
	}
}

func requestWithPlanTool(messages ...trpcmodel.Message) *trpcmodel.Request {
	return &trpcmodel.Request{
		Tools:    map[string]trpctool.Tool{planAndExecuteToolName: nil},
		Messages: messages,
	}
}

// TestForcePlanningRouteHook pins SP-2a 硬路由触发/抑制矩阵。
func TestForcePlanningRouteHook(t *testing.T) {
	newHook := func(cc *captureDecisionCollector) callbacks.Callback {
		// cc 为 nil 时必须传真正的 nil 接口——typed-nil 指针装进接口后
		// EmitDecision 的 c == nil 判空失效，c.Emit 空指针崩溃。
		var dc decision.Collector
		if cc != nil {
			dc = cc
		}
		return newForcePlanningRouteAfterModelHook(TRPCBuilderDeps{
			TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{DecisionCollector: dc},
		})
	}

	t.Run("no_marker_noop", func(t *testing.T) {
		res := invokeForcePlanningHook(t, newHook(nil), context.Background(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("unmarked ctx must not hard-route")
		}
	})

	t.Run("partial_response_noop", func(t *testing.T) {
		resp := finalTextResponse()
		resp.IsPartial = true
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: resp,
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("partial response must not hard-route")
		}
	})

	t.Run("error_response_noop", func(t *testing.T) {
		resp := finalTextResponse()
		resp.Error = &trpcmodel.ResponseError{Message: "boom"}
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: resp,
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("error response must not hard-route")
		}
	})

	t.Run("llm_tool_call_response_noop", func(t *testing.T) {
		resp := finalTextResponse()
		resp.Choices[0].Message.ToolCalls = []trpcmodel.ToolCall{{
			ID:       "call_x",
			Type:     "function",
			Function: trpcmodel.FunctionDefinitionParam{Name: "web_research"},
		}}
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: resp,
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("LLM tool-call response must not hard-route (loop continues)")
		}
	})

	t.Run("tool_not_in_request_noop", func(t *testing.T) {
		// 团队成员 run：ctx 标记泄漏但工具面无 plan_and_execute，不得触发。
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  &trpcmodel.Request{Tools: map[string]trpctool.Tool{}},
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("request without plan_and_execute must not hard-route")
		}
	})

	t.Run("already_called_via_tool_calls_noop", func(t *testing.T) {
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request: requestWithPlanTool(trpcmodel.Message{
				Role: trpcmodel.RoleAssistant,
				ToolCalls: []trpcmodel.ToolCall{{
					ID:       "call_prev",
					Type:     "function",
					Function: trpcmodel.FunctionDefinitionParam{Name: planAndExecuteToolName},
				}},
			}),
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("plan_and_execute already called must not re-route")
		}
	})

	t.Run("already_called_via_tool_result_noop", func(t *testing.T) {
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request: requestWithPlanTool(trpcmodel.Message{
				Role:     trpcmodel.RoleTool,
				ToolID:   "call_prev",
				ToolName: planAndExecuteToolName,
				Content:  "{}",
			}),
			Response: finalTextResponse(),
		})
		if res != nil && res.CustomResponse != nil {
			t.Fatal("plan_and_execute tool result in history must not re-route")
		}
	})

	t.Run("hard_route_fires", func(t *testing.T) {
		cc := &captureDecisionCollector{}
		resp := finalTextResponse()
		res := invokeForcePlanningHook(t, newHook(cc), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "写一份竞品分析报告"}),
			Response: resp,
		})
		if res == nil || res.CustomResponse == nil {
			t.Fatal("expected hard-route CustomResponse")
		}
		custom := res.CustomResponse
		if custom.IsPartial || !custom.Done {
			t.Fatalf("custom response must be final: partial=%v done=%v", custom.IsPartial, custom.Done)
		}
		if custom.ID != resp.ID {
			t.Fatalf("custom response must reuse original ID for dedup: got %q want %q", custom.ID, resp.ID)
		}
		if custom.Usage != resp.Usage {
			t.Fatal("custom response must preserve original Usage for token accounting")
		}
		if len(custom.Choices) != 1 || len(custom.Choices[0].Message.ToolCalls) != 1 {
			t.Fatalf("custom response must carry exactly one tool call: %+v", custom.Choices)
		}
		tc := custom.Choices[0].Message.ToolCalls[0]
		if tc.Function.Name != planAndExecuteToolName {
			t.Fatalf("tool call name = %q, want %q", tc.Function.Name, planAndExecuteToolName)
		}
		if tc.ID == "" {
			t.Fatal("tool call ID must be non-empty (framework validates)")
		}
		var args map[string]any
		if err := json.Unmarshal(tc.Function.Arguments, &args); err != nil {
			t.Fatalf("tool call arguments not valid JSON: %v", err)
		}
		if args["task_prompt"] != "写一份竞品分析报告" {
			t.Fatalf("task_prompt = %v", args["task_prompt"])
		}
		// IsToolCallResponse 决定框架继续循环执行工具。
		if !custom.IsToolCallResponse() {
			t.Fatal("custom response must be a tool-call response so the flow executes it")
		}
		// SP-1b：决策记录双写。
		if len(cc.recs) != 1 {
			t.Fatalf("expected 1 decision record, got %d", len(cc.recs))
		}
		rec := cc.recs[0]
		if rec.Outcome != "hard_route_plan_and_execute" {
			t.Fatalf("decision outcome = %q", rec.Outcome)
		}
		if rec.Category != "planner_orchestration" {
			t.Fatalf("decision category = %q", rec.Category)
		}
	})

	t.Run("hard_route_plan_team_passes_mode", func(t *testing.T) {
		ctx := ContextWithForcePlanningRoute(context.Background(), ForcePlanningRoute{
			TaskPrompt: "让市场部出一版 Q3 推广文案框架",
			Level:      "moderate",
			Score:      0.4,
			Reason:     "强制走规划路径",
			Mode:       "dag",
		})
		res := invokeForcePlanningHook(t, newHook(nil), ctx, &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: finalTextResponse(),
		})
		if res == nil || res.CustomResponse == nil {
			t.Fatal("expected hard-route CustomResponse")
		}
		tc := res.CustomResponse.Choices[0].Message.ToolCalls[0]
		var args map[string]any
		if err := json.Unmarshal(tc.Function.Arguments, &args); err != nil {
			t.Fatalf("args: %v", err)
		}
		if args["mode"] != "dag" {
			t.Fatalf("mode = %v, want dag (FIT-ROUTE-1: committed PlanTeam must not inject empty mode)", args["mode"])
		}
	})

	t.Run("hard_route_nil_collector_still_fires", func(t *testing.T) {
		// collector nil（CLI/未装配）时 flowlog 仍落、直调不阻断（D1）。
		res := invokeForcePlanningHook(t, newHook(nil), markedCtx(), &trpcmodel.AfterModelArgs{
			Request:  requestWithPlanTool(),
			Response: finalTextResponse(),
		})
		if res == nil || res.CustomResponse == nil {
			t.Fatal("nil collector must not block hard-route")
		}
	})
}
