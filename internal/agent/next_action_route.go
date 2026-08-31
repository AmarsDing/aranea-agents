package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// N2 硬约束（session-eval-20260829-r2 根修）：plan_and_execute 返回
// next_action=build_orchestration_graph 后，S07 实测模型 tool_load 了该工具
// 却从未调用，转去其他工具后坦白放弃——next_action 此前只是软提示，Spirit
// 对「工具已 load 待调用」状态无感知。本文件提供三层机制：
//
//  1. AfterTool 跟踪：plan_and_execute 结果含「需工具跟进」的 next_action 时，
//     在 invocation state 记 pending 指令；该工具被实际调用时核销。
//  2. BeforeModel 强制 cue：pending 期间每轮请求注入「必须调用」动态 cue；
//     工具已激活未调用时文案升级（禁止再 tool_load / 转向其他工具）。
//  3. AfterModel 终答拦截：pending 期间 LLM 产出无工具调用的终答时，合成一次
//     tool_load 调用替换终答（保持循环存活 + 幂等激活目标工具），每指令仅一次；
//     催办后仍放弃的，写 decision_records/flowlog 留痕放行，不无限循环。
//     BlockFinal（await_orchestration）：替换为状态终答，禁止 DIY 交付物。
//
// 合成 tool_load 而非直调 build_orchestration_graph：后者需 agents 列表等
// 业务参数（build_graph.go: agents 至少一个），系统无法凭空构造有意义的
// 参数，必须由 LLM 结合 roster_miss 上下文自行填参。
//
// 与 force_planning_route.go 的区别：那是门控「该规划没规划」的入口拦截；
// 本文件是「规划已给出下一步指令但未被执行」的出口拦截。

// nextActionRequiredTool 把 next_action 指令映射到必须跟进的工具名。
// 仅收录「需工具调用」的指令；await_user_clarification / decompose_failed /
// authorize_playbook 是面向用户的表达指令。planning_in_progress 在未提交
// 车道上仍允许 Spirit 说话；已提交 PlanTeam 走 await_orchestration。
var nextActionRequiredTool = map[string]string{
	biz.RosterMissNextAction: "build_orchestration_graph",
}

// nextActionBlockFinal marks next_action values that forbid a DIY
// deliverable as the turn's final answer (replace with status speech).
var nextActionBlockFinal = map[string]bool{
	biz.AwaitOrchestrationNextAction: true,
}

// pendingNextAction 是 invocation state 中暂存的待执行指令。
type pendingNextAction struct {
	Tool       string // 必须跟进的工具名（空 = 无工具催办）
	Hint       string // plan_and_execute 随 next_action 返回的中文指引
	Nudged     bool   // AfterModel 是否已合成过一次 tool_load 催办
	BlockFinal bool   // 已提交组队：拦截 DIY 终答，换成状态汇报
}

// pendingNextActionStateKey 是 invocation state 键（per-invocation，不跨 turn）。
const pendingNextActionStateKey = "aranea.pending_next_action"

// nextActionCueMarker 是动态 cue 的去重标记（replaceDynamicCue 语义）。
const nextActionCueMarker = "<!-- aranea:next_action_enforce -->\n"

// newNextActionTrackAfterHook 跟踪 plan_and_execute 的 next_action 指令并
// 在目标工具被调用时核销。priority 55：在工具记录器（50）之后运行。
func newNextActionTrackAfterHook() callbacks.Callback {
	return callbacks.NewAfterToolHook(55, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		toolName := strings.TrimSpace(args.ToolName)
		// 目标工具被实际调用（无论成败）即核销 pending——已遵循指令。
		if v, found := inv.GetState(pendingNextActionStateKey); found {
			if p, ok := v.(pendingNextAction); ok && toolName == p.Tool {
				inv.SetState(pendingNextActionStateKey, pendingNextAction{})
			}
		}
		if toolName != planAndExecuteToolName || args.Error != nil {
			return &trpctool.AfterToolResult{}, nil
		}
		m := toolResultMap(args.Result)
		if m == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		nextAction, _ := m["next_action"].(string)
		nextAction = strings.TrimSpace(nextAction)
		if nextActionBlockFinal[nextAction] {
			hint, _ := m["reuse_reason"].(string)
			inv.SetState(pendingNextActionStateKey, pendingNextAction{BlockFinal: true, Hint: hint})
			return &trpctool.AfterToolResult{}, nil
		}
		required, tracked := nextActionRequiredTool[nextAction]
		if !tracked {
			return &trpctool.AfterToolResult{}, nil
		}
		hint, _ := m["reuse_reason"].(string)
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: required, Hint: hint})
		return &trpctool.AfterToolResult{}, nil
	})
}

// newNextActionCueBeforeHook 在 pending 期间向每轮请求注入强制 cue。
// priority 3 + LayerDynamic：与 reply reminder 同档，经 replaceDynamicCue
// 去重（续轮不堆叠）。deps.DeferredManager 用于区分「未激活/已激活未调用」。
func newNextActionCueBeforeHook(deps TRPCBuilderDeps) callbacks.Callback {
	return callbacks.NewBeforeModelHook(3, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		v, found := inv.GetState(pendingNextActionStateKey)
		if !found {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		p, ok := v.(pendingNextAction)
		if !ok || (p.Tool == "" && !p.BlockFinal) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		var cue string
		if p.BlockFinal {
			cue = `[系统强制] 本轮已提交组队编排。禁止直接撰写用户交付物正文；只向用户说明已派发/进行中，等待团队交付。`
		} else if deps.DeferredManager != nil && deps.DeferredManager.IsActivated(ctx, p.Tool) {
			// 已 load 未调用（S07 形态）：禁止再 tool_load / 转向无关工具。
			cue = fmt.Sprintf(`[系统强制] %s 已在本会话激活，你必须立即调用它完成组队——禁止再次 tool_load、禁止转向其他工具、禁止直接向用户宣告无法执行。参数不足时先用查询类工具收集信息，随后本轮或下一轮必须调用 %s。`,
				p.Tool, p.Tool)
		} else {
			cue = fmt.Sprintf(`[系统强制] plan_and_execute 已指示下一步：调用 %s。%s禁止忽略该指令或直接向用户宣告无法执行；该工具未激活时先 tool_load 再调用。`,
				p.Tool, strings.TrimSpace(p.Hint+" "))
		}
		args.Request.Messages = replaceDynamicCue(args.Request.Messages, nextActionCueMarker, nextActionCueMarker+cue)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// newNextActionRouteAfterModelHook 拦截「pending 期间的无工具调用终答」。
// priority 99：先于 force_planning 硬路由（100）执行——两者互斥（force_planning
// 防重条件为历史无 plan_and_execute 调用，而 pending 只在 plan_and_execute
// 已调用后产生），但观测语义上 next_action 是更后段的拦截。
func newNextActionRouteAfterModelHook(deps TRPCBuilderDeps) callbacks.Callback {
	lg := deps.Logger()
	return callbacks.NewAfterModelHook(99, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return nil, nil
		}
		v, found := inv.GetState(pendingNextActionStateKey)
		if !found {
			return nil, nil
		}
		p, ok := v.(pendingNextAction)
		if !ok || (p.Tool == "" && !p.BlockFinal) {
			return nil, nil
		}
		resp := args.Response
		if resp == nil || resp.IsPartial || !resp.Done || resp.Error != nil {
			return nil, nil
		}
		// LLM 调用了工具（可能正是目标工具，由 AfterTool 核销）——让循环继续。
		if resp.IsToolCallResponse() {
			return nil, nil
		}
		if args.Request == nil {
			return nil, nil
		}
		sessionID := ""
		if inv.Session != nil {
			sessionID = strings.TrimSpace(inv.Session.ID)
		}
		if p.BlockFinal {
			inv.SetState(pendingNextActionStateKey, pendingNextAction{})
			text := strings.TrimSpace(p.Hint)
			if text == "" {
				text = biz.AwaitOrchestrationUserHint
			}
			synthetic := buildStatusTextResponse(resp, text)
			if synthetic == nil {
				return nil, nil
			}
			lg.Warn("next_action 硬约束：拦截组队后 DIY 终答，改为状态汇报",
				loggateway.StepID("chat.next_action.block_final"),
				loggateway.SessionID(sessionID),
			)
			event.EmitDecision(ctx, deps.DecisionCollector, decision.Record{
				DecisionKey: uuid.NewString(),
				Category:    decision.CategoryPlannerOrchestration,
				Scenario:    "next_action 拦截 DIY 终答",
				Reasoning:   "已提交 PlanTeam 编排启动后 LLM 仍撰写交付物正文，替换为状态汇报。",
				Outcome:     "block_final_await_orchestration",
				ActorType:   decision.ActorSystem,
				ActorKey:    "system:next_action_route",
				SourceRef:   decision.SourceRef{SessionID: sessionID},
				Metadata:    map[string]any{"session_id": sessionID},
			}, "chat.next_action.block_final", "拦截组队后 DIY 终答",
				event.P("session_id", sessionID),
			)
			return &trpcmodel.AfterModelResult{Context: ctx, CustomResponse: synthetic}, nil
		}
		if p.Tool == "" {
			return nil, nil
		}
		// 目标工具已在本 turn 出现过（历史消息含调用/结果）——核销放行。
		if toolCalledInRequest(args.Request.Messages, p.Tool) {
			inv.SetState(pendingNextActionStateKey, pendingNextAction{})
			return nil, nil
		}
		if p.Nudged {
			// 已催办一次仍放弃：留痕放行，不无限循环。
			inv.SetState(pendingNextActionStateKey, pendingNextAction{})
			lg.Warn("next_action 硬约束：催办后 LLM 仍未调用目标工具，放行终答",
				loggateway.StepID("chat.next_action.abandoned"),
				loggateway.SessionID(sessionID),
				loggateway.Str("required_tool", p.Tool),
			)
			event.EmitDecision(ctx, deps.DecisionCollector, decision.Record{
				DecisionKey: uuid.NewString(),
				Category:    decision.CategoryPlannerOrchestration,
				Scenario:    "next_action 硬约束放弃",
				Reasoning: fmt.Sprintf("plan_and_execute 指示 next_action=%s，合成 tool_load 催办后 LLM 仍未调用，放行终答（防无限循环）。原指引：%s",
					p.Tool, p.Hint),
				Outcome:   "next_action_abandoned",
				ActorType: decision.ActorSystem,
				ActorKey:  "system:next_action_route",
				SourceRef: decision.SourceRef{SessionID: sessionID},
				Metadata: map[string]any{
					"required_tool": p.Tool,
					"session_id":    sessionID,
				},
			}, "chat.next_action.abandoned", "next_action 硬约束：催办后仍未调用，放行",
				event.P("required_tool", p.Tool),
				event.P("session_id", sessionID),
			)
			return nil, nil
		}
		// 合成 tool_load 催办：替换终答为 tool_load(required) 调用——保持工具
		// 循环存活，且 tool_load 幂等激活目标工具，下一轮请求工具面即含目标
		// 工具 + BeforeModel 强制 cue 升级为「已激活必须调用」文案。
		synthetic := buildToolLoadCallResponse(resp, p.Tool)
		if synthetic == nil {
			return nil, nil
		}
		inv.SetState(pendingNextActionStateKey, pendingNextAction{Tool: p.Tool, Hint: p.Hint, Nudged: true})
		lg.Warn("next_action 硬约束：LLM 终答未遵循指令，合成 tool_load 催办",
			loggateway.StepID("chat.next_action.hard_route"),
			loggateway.SessionID(sessionID),
			loggateway.Str("required_tool", p.Tool),
		)
		event.EmitDecision(ctx, deps.DecisionCollector, decision.Record{
			DecisionKey: uuid.NewString(),
			Category:    decision.CategoryPlannerOrchestration,
			Scenario:    "next_action 硬路由",
			Reasoning: fmt.Sprintf("plan_and_execute 指示 next_action=%s，LLM 终答未调用（软提示未遵从），合成 tool_load 催办并保持循环。原指引：%s",
				p.Tool, p.Hint),
			Outcome:   "hard_route_tool_load",
			ActorType: decision.ActorSystem,
			ActorKey:  "system:next_action_route",
			SourceRef: decision.SourceRef{SessionID: sessionID},
			Metadata: map[string]any{
				"required_tool": p.Tool,
				"session_id":    sessionID,
			},
		}, "chat.next_action.hard_route", "next_action 硬约束：终答未遵循，合成 tool_load 催办",
			event.P("required_tool", p.Tool),
			event.P("session_id", sessionID),
		)
		return &trpcmodel.AfterModelResult{Context: ctx, CustomResponse: synthetic}, nil
	})
}

// toolCalledInRequest 扫描请求消息历史，判定目标工具本 turn 是否已被调用
// （assistant tool_calls）或已有结果（tool 消息）。
func toolCalledInRequest(messages []trpcmodel.Message, toolName string) bool {
	for _, m := range messages {
		if m.ToolName == toolName {
			return true
		}
		for _, tc := range m.ToolCalls {
			if tc.Function.Name == toolName {
				return true
			}
		}
	}
	return false
}

// buildToolLoadCallResponse 构造替换终答的合成 tool_load 工具调用响应。
// 保留原响应的 ID/Object/Created/Model/Usage（计量与事件链路不受影响），
// 语义同 buildForcedPlanToolCallResponse。
func buildToolLoadCallResponse(orig *trpcmodel.Response, toolName string) *trpcmodel.Response {
	argsJSON, err := json.Marshal(map[string]any{"tool_name": toolName})
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
					ID:   "call_next_action_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
					Function: trpcmodel.FunctionDefinitionParam{
						Name:      "tool_load",
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

// buildStatusTextResponse replaces a DIY final with a status-only reply.
func buildStatusTextResponse(orig *trpcmodel.Response, text string) *trpcmodel.Response {
	if orig == nil {
		return nil
	}
	finish := "stop"
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
				Role:    trpcmodel.RoleAssistant,
				Content: text,
			},
			FinishReason: &finish,
		}},
	}
	if out.Object == "" {
		out.Object = trpcmodel.ObjectTypeChatCompletion
	}
	return out
}
