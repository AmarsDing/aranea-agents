package graph

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// deliverable_inspection_wiring.go —— P3-2（2026-09-03 语义面防线）：合成前
// 产出语义校验（Inspector 轻量版）。
//
// 成员「执行成功却交付垃圾」（拒绝语/错误转储/空内容）会无异常流入
// synthesizer，聚合抛光后静默放大（MAS 语义面故障）。本回调在 synthesizer
// 节点执行前对 graph state 的 deliverable map 做规则体检
// （biz.InspectDeliverables），把发现作为 [产出校验] 通告原地注入节点输入
// stateCopy 的 messages——synthesizer LLM 的上下文由 messages 构建，通告使
// 可疑产出对其可见，与 P1 名册（prompt 层）/失败通告（mechanism 层）构成
// 三层语义防线。
//
// fail-open 哲学：只标注不阻断。回调必须原地 mutate state 并返回 (nil, nil)
// ——框架语义下 BeforeNodeCallback 返回非 nil customResult 会跳过节点执行
// （executor.runBeforeCallbacks），返回错误会终止图，二者都违背 advisory
// 定位。判决权留给 synthesizer 与团队级质量门（runner_quality_gate.go）。

// deliverableInspectionOptions 为 synthesizer 角色节点挂合成前校验回调。
// 其他角色节点（worker/coordinator）不挂——校验只在聚合点做一次。
func deliverableInspectionOptions(n NodeDef) []trpcgraph.Option {
	if !strings.EqualFold(strings.TrimSpace(n.RequiredRole), biz.RoleSynthesizer) {
		return nil
	}
	return []trpcgraph.Option{
		trpcgraph.WithPreNodeCallback(inspectDeliverableBeforeSynthesis),
	}
}

// inspectDeliverableBeforeSynthesis 是 synthesizer 节点的 PreNodeCallback：
// 体检 deliverable map，有发现则向 messages 追加一条聚合通告。
func inspectDeliverableBeforeSynthesis(_ context.Context, _ *trpcgraph.NodeCallbackContext, state trpcgraph.State) (any, error) {
	deliv, ok := toDeliverableStateMap(state[biz.DeliverableStateKey])
	if !ok || len(deliv) == 0 {
		return nil, nil
	}
	// 契约在图层不可得（Definition 级概念，不下发 NodeDef），v1 只做内容
	// 体检；Required topic 缺失已由 P1 名册通告与完成期 advisory 双覆盖。
	findings := biz.InspectDeliverables(deliv, nil)
	if len(findings) == 0 {
		return nil, nil
	}
	appendInspectionNoticeMessage(state, findings)
	return nil, nil
}

// toDeliverableStateMap 容忍 state 中 deliverable 的两种形态（直接 map 或
// JSON 反序列化后的 map）。
func toDeliverableStateMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	default:
		return nil, false
	}
}

// inspectionNoticeMaxFindings 是单条通告携带的 findings 上限——通告进 LLM
// 上下文，过长稀释注意力；超出部分聚合为计数。
const inspectionNoticeMaxFindings = 8

// appendInspectionNoticeMessage 把聚合校验通告追加到 state messages
// （assistant 角色，对 synthesizer 上下文可见）。与 P1 skip 通告
// （failure_recovery.go appendSkipNoticeMessage）同构。
func appendInspectionNoticeMessage(state trpcgraph.State, findings []biz.DeliverableFinding) {
	var sb strings.Builder
	sb.WriteString("[产出校验] 合成前语义检查发现 ")
	sb.WriteString(fmt.Sprintf("%d", len(findings)))
	sb.WriteString(" 项可疑产出：")
	for i, f := range findings {
		if i >= inspectionNoticeMaxFindings {
			fmt.Fprintf(&sb, "；…及其余 %d 项", len(findings)-inspectionNoticeMaxFindings)
			break
		}
		if i > 0 {
			sb.WriteString("；")
		}
		fmt.Fprintf(&sb, "topic %q（%s）", f.Topic, f.Detail)
	}
	sb.WriteString("。聚合时请如实标注可疑部分并酌情降级采信，不得将其抛光为可信结论。")
	notice := trpcmodel.Message{
		Role:    trpcmodel.RoleAssistant,
		Content: sb.String(),
	}
	switch existing := state[trpcgraph.StateKeyMessages].(type) {
	case []trpcmodel.Message:
		state[trpcgraph.StateKeyMessages] = append(existing, notice)
	default:
		state[trpcgraph.StateKeyMessages] = []trpcmodel.Message{notice}
	}
}
