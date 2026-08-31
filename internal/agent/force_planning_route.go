package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// SP-2a（session-eval-20260829 根修 C1）：预规划门控从「提示式」升级为
// 「硬路由」。门控判定 ForcePlanning 后仍注入系统提示（首选路径，LLM 遵从
// 时零额外开销）；LLM 未遵从——终答不含任何工具调用——时，本文件的
// AfterModel 钩子把该终答替换为 plan_and_execute 工具调用响应，框架按
// 普通工具调用继续执行（直调，跳过重试，不新增 LLM 轮次）。
//
// 候选方案对比（2026-08-29 用户裁定「直调，跳过重试」）：
//   - provider tool_choice：框架 openai.go 显式剥离 tool_control 字段
//     （modelrequest.DeleteToolControlFields），需改 vendored 框架，禁令
//     排除；
//   - LLM 重试一轮：多一轮全量上下文调用（S07 实测单轮 input 10K+），
//     且重试仍不保证遵从；
//   - AfterModel CustomResponse 直调：框架原生支持（callbacks.go
//     AfterModelResult.CustomResponse「replace the original response」），
//     零新增 LLM 轮次，确定性 100%。

// 硬路由目标工具名复用 tool_loop_guard.go 的 planAndExecuteToolName 常量
//（Spirit 专属编排入口，cli_admin_tools.go spiritCustomTools 仅 isSpirit 装配）。

// ForcePlanningRoute 携带门控的强制规划决策进入 Spirit run ctx。
// 仅根 turn 设置（续跑 turn 门控跳过，chat_orchestrator_turn.go）。
type ForcePlanningRoute struct {
	TaskPrompt string  // 意图精化目标优先，回退原始用户消息
	Level      string  // 复杂度档位（moderate/complex）
	Score      float64 // 六维评分
	Reason     string  // 门控理由（审计）
	Mode       string  // 已提交的 plan_and_execute mode（PlanTeam 为 dag/parallel；空 = PlanSolo）
}

type forcePlanningRouteKey struct{}

// ContextWithForcePlanningRoute 在 turn ctx 上标记「本 turn 强制规划」。
// TaskPrompt 为空时不标记（与门控跳过等价）。
func ContextWithForcePlanningRoute(ctx context.Context, route ForcePlanningRoute) context.Context {
	if strings.TrimSpace(route.TaskPrompt) == "" {
		return ctx
	}
	return context.WithValue(ctx, forcePlanningRouteKey{}, route)
}

// ForcePlanningRouteFromCtx 读取强制规划标记。
func ForcePlanningRouteFromCtx(ctx context.Context) (ForcePlanningRoute, bool) {
	if ctx == nil {
		return ForcePlanningRoute{}, false
	}
	v, ok := ctx.Value(forcePlanningRouteKey{}).(ForcePlanningRoute)
	return v, ok
}

// newForcePlanningRouteAfterModelHook 返回硬路由 AfterModel 钩子。
//
// 触发条件（全部满足）：
//  1. turn ctx 携带 ForcePlanningRoute 标记（门控判定强制规划）；
//  2. 响应是最终聚合响应（非 partial、Done、无错误）；
//  3. 响应不含工具调用——LLM 正准备直接终答（提示未被遵从）；
//  4. 请求工具面含 plan_and_execute（Spirit 根 turn；团队成员无此工具，
//     兼防 turn ctx 标记泄漏到 member run 时误触发）；
//  5. 请求消息历史中尚无 plan_and_execute 调用/结果（防重：LLM 自主调用
//     或本钩子注入后，后续轮次不再介入）。
//
// LLM 先调其他工具（检索/记忆等上下文收集）时条件 3 不命中，循环正常
// 继续；其后的无工具调用终答仍会被本钩子接管。
//
// priority 100：其余 AfterModel 观测 hook（token 累加 0 / L0 快照 10）先
// 看到原始响应；本钩子返回 CustomResponse 会终止回调链（框架
// continueOnResponse=false），排最后避免观测缺口。
func newForcePlanningRouteAfterModelHook(deps TRPCBuilderDeps) callbacks.Callback {
	lg := deps.Logger()
	return callbacks.NewAfterModelHook(100, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
		route, ok := ForcePlanningRouteFromCtx(ctx)
		if !ok {
			return nil, nil
		}
		resp := args.Response
		if resp == nil || resp.IsPartial || !resp.Done || resp.Error != nil {
			return nil, nil
		}
		// LLM 自主调用了工具（可能正是 plan_and_execute）——让循环继续，
		// 下一轮请求消息会携带调用记录（条件 5 防重命中）。
		if resp.IsToolCallResponse() {
			return nil, nil
		}
		if args.Request == nil {
			return nil, nil
		}
		synthetic, outcome, stepMsg := buildForcedLaneToolCall(args.Request, resp, route)
		if synthetic == nil {
			return nil, nil
		}
		sessionID := ""
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
			sessionID = strings.TrimSpace(inv.Session.ID)
		}
		lg.Warn("强制规划硬路由：LLM 终答未调用车道工具，系统直调",
			loggateway.StepID("chat.force_planning.hard_route"),
			loggateway.SessionID(sessionID),
			loggateway.Str("gate_level", route.Level),
			loggateway.Float64("gate_score", route.Score),
			loggateway.Str("outcome", outcome),
		)
		// SP-1b 统一入口：decision_records + flowlog 双写；collector nil 时
		// flowlog 仍落（D1）。
		event.EmitDecision(ctx, deps.DecisionCollector, decision.Record{
			DecisionKey: uuid.NewString(),
			Category:    decision.CategoryPlannerOrchestration,
			Scenario:    "强制规划硬路由",
			Reasoning: fmt.Sprintf("门控判定强制规划（%s，score %.2f），LLM 终答未调用 plan_and_execute，硬路由直调（跳过重试）。门控理由：%s",
				route.Level, route.Score, route.Reason),
			Outcome:   outcome,
			ActorType: decision.ActorSystem,
			ActorKey:  "system:pre_planning_gate",
			SourceRef: decision.SourceRef{SessionID: sessionID},
			Metadata: map[string]any{
				"gate_level":  route.Level,
				"gate_score":  route.Score,
				"gate_reason": route.Reason,
				"session_id":  sessionID,
			},
		}, "chat.force_planning.hard_route", stepMsg,
			event.P("gate_level", route.Level),
			event.P("gate_score", route.Score),
			event.P("session_id", sessionID),
		)
		return &trpcmodel.AfterModelResult{Context: ctx, CustomResponse: synthetic}, nil
	})
}

const subagentsSpawnToolName = "subagents_spawn"

// buildForcedLaneToolCall picks the tool that honors the committed lane:
// Spirit → plan_and_execute; governance faces without that tool → subagents_spawn.
func buildForcedLaneToolCall(req *trpcmodel.Request, orig *trpcmodel.Response, route ForcePlanningRoute) (*trpcmodel.Response, string, string) {
	if req == nil {
		return nil, "", ""
	}
	if _, has := req.Tools[planAndExecuteToolName]; has && !toolCalledInRequest(req.Messages, planAndExecuteToolName) {
		synthetic := buildForcedPlanToolCallResponse(orig, route)
		if synthetic == nil {
			return nil, "", ""
		}
		return synthetic, "hard_route_plan_and_execute", "强制规划硬路由：LLM 未调 plan_and_execute，系统直调"
	}
	if _, has := req.Tools[subagentsSpawnToolName]; has && !toolCalledInRequest(req.Messages, subagentsSpawnToolName) {
		synthetic := buildForcedSpawnToolCallResponse(orig, route)
		if synthetic == nil {
			return nil, "", ""
		}
		return synthetic, "hard_route_subagents_spawn", "治理岗硬路由：LLM 未派发即终答，系统直调 subagents_spawn"
	}
	return nil, "", ""
}

func buildForcedSpawnToolCallResponse(orig *trpcmodel.Response, route ForcePlanningRoute) *trpcmodel.Response {
	args := map[string]any{"task": strings.TrimSpace(route.TaskPrompt), "kind": "general"}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	finish := "tool_calls"
	out := &trpcmodel.Response{
		ID:        orig.ID,
		Object:    orig.Object,
		Created:   orig.Created,
		Model:     orig.Model,
		Usage:     orig.Usage,
		Timestamp: orig.Timestamp,
		Done:      true,
		Choices: []trpcmodel.Choice{{
			Index: 0,
			Message: trpcmodel.Message{
				Role: trpcmodel.RoleAssistant,
				ToolCalls: []trpcmodel.ToolCall{{
					Type: "function",
					ID:   "call_force_spawn_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
					Function: trpcmodel.FunctionDefinitionParam{
						Name:      subagentsSpawnToolName,
						Arguments: argsJSON,
					},
				}},
			},
			FinishReason: &finish,
		}},
	}
	if out.Object == "" {
		out.Object = trpcmodel.ObjectTypeChatCompletion
	}
	return out
}

// buildForcedPlanToolCallResponse 构造替换原终答的合成工具调用响应。
// 保留原响应的 ID/Object/Created/Model/Usage——token 计量（stream_consumer
// 按 Response.ID 去重、Usage 透传）与事件链路不受影响。原终答文本丢弃：
// 流式 delta 已下发无法回收，落库历史只保留「工具调用」这一干净事实。
// Mode 来自编排器提交的 RouteDecision：PlanTeam 必须带 dag/parallel，禁止
// 再把空 mode 交给 Plan() 自降级 direct（S06 FIT-ROUTE-1）。PlanSolo 仍可
// 只传 task_prompt，由 Plan() 按 committed lane 抬档分解。
func buildForcedPlanToolCallResponse(orig *trpcmodel.Response, route ForcePlanningRoute) *trpcmodel.Response {
	args := map[string]any{"task_prompt": strings.TrimSpace(route.TaskPrompt)}
	if m := strings.TrimSpace(route.Mode); m != "" {
		args["mode"] = m
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	finish := "tool_calls"
	out := &trpcmodel.Response{
		ID:        orig.ID,
		Object:    orig.Object,
		Created:   orig.Created,
		Model:     orig.Model,
		Usage:     orig.Usage,
		Timestamp: orig.Timestamp,
		Done:      true,
		Choices: []trpcmodel.Choice{{
			Index: 0,
			Message: trpcmodel.Message{
				Role: trpcmodel.RoleAssistant,
				ToolCalls: []trpcmodel.ToolCall{{
					Type: "function",
					ID:   "call_force_plan_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
					Function: trpcmodel.FunctionDefinitionParam{
						Name:      planAndExecuteToolName,
						Arguments: argsJSON,
					},
				}},
			},
			FinishReason: &finish,
		}},
	}
	if out.Object == "" {
		out.Object = trpcmodel.ObjectTypeChatCompletion
	}
	return out
}
