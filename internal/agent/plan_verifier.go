package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// plan_verifier.go — P4-G1 计划校验门（Plan Verification Gate）。
//
// 问题：任务分解此前只有 DAG 结构校验（validateSubTaskDAG），不校验计划
// 可行性——子任务声明的 required_capabilities 可能无任何 Agent 具备、子任务
// 定义可能为空壳，这些失败要等到分配/执行期才暴露（表现为随机兜底分配）。
//
// 方案（Self-Refine 有界自修复）：分解后、计划落库前插入规则层校验门；
// 违例时把违例详情写回 prompt 有界重分解 1 次；仍违例则带详情降级 direct。
// 理论对照：Self-Refine（NeurIPS 2023）generate→feedback→refine 有界循环。
//
// 红线约束：
//   - 重分解有界（恰好 1 次），禁无限自循环（多 Agent 墙钟风险 #4）。
//   - 能力清单构建失败 fail-open（跳过校验），基础设施抖动不阻断规划。
//   - capBuilder 为 nil（旧测试构造路径）时整体跳过，行为与旧版一致。

// 计划校验违例规则。
const (
	// PlanViolationEmptyDefinition 子任务 name/description 为空。
	PlanViolationEmptyDefinition = "empty_definition"
	// PlanViolationCapabilityUnsatisfiable 子任务声明的能力标签无任何 Agent 具备。
	PlanViolationCapabilityUnsatisfiable = "capability_unsatisfiable"
	// PlanViolationOversizedPlan 子任务数量超过病态阈值。
	PlanViolationOversizedPlan = "oversized_plan"
)

// maxVerifiedSubTasks 计划规模病态阈值。DECISION.md 约定 1-6 个子任务，
// detectTeamCount 允许用户显式请求至 20——取 12 为病态边界（只拦 LLM
// 失控产出，不误伤合法的多团队请求）。
const maxVerifiedSubTasks = 12

// PlanViolation 是一条计划校验违例。
type PlanViolation struct {
	Rule      string // PlanViolation* 常量
	SubTaskID string // oversized_plan 时为空（计划级违例）
	Detail    string
}

// verifyPlanFeasibility 对分解产物做规则层可行性校验（纯函数，无副作用）。
//
// 规则：
//   - R1 empty_definition：name 或 description 空白。
//   - R2 capability_unsatisfiable：RequiredCapabilities 非空且存在标签不在
//     任何 Agent 的 Roles 并集中（大小写不敏感）。能力清单为空时不适用
//     （系统无业务 Agent 是运维态问题，不是计划违例）。
//   - R3 oversized_plan：子任务数 > maxVerifiedSubTasks。
func verifyPlanFeasibility(subTasks []biz.SubTask, capabilities []biz.AgentCapability) []PlanViolation {
	if len(subTasks) == 0 {
		return nil
	}
	var violations []PlanViolation

	if len(subTasks) > maxVerifiedSubTasks {
		violations = append(violations, PlanViolation{
			Rule:   PlanViolationOversizedPlan,
			Detail: fmt.Sprintf("子任务数量 %d 超过上限 %d", len(subTasks), maxVerifiedSubTasks),
		})
	}

	roleUnion := make(map[string]struct{})
	for _, cap := range capabilities {
		for _, r := range cap.Roles {
			if nr := normalizeCapabilityTag(r); nr != "" {
				roleUnion[nr] = struct{}{}
			}
		}
	}

	for _, st := range subTasks {
		if strings.TrimSpace(st.Name) == "" || strings.TrimSpace(st.Description) == "" {
			violations = append(violations, PlanViolation{
				Rule:      PlanViolationEmptyDefinition,
				SubTaskID: st.ID,
				Detail:    "子任务 name 或 description 为空",
			})
		}
		if len(st.RequiredCapabilities) == 0 || len(roleUnion) == 0 {
			continue
		}
		var missing []string
		for _, req := range st.RequiredCapabilities {
			nr := normalizeCapabilityTag(req)
			if nr == "" {
				continue
			}
			if _, ok := roleUnion[nr]; !ok {
				missing = append(missing, req)
			}
		}
		if len(missing) > 0 {
			violations = append(violations, PlanViolation{
				Rule:      PlanViolationCapabilityUnsatisfiable,
				SubTaskID: st.ID,
				Detail:    fmt.Sprintf("required_capabilities %v 无任何 Agent 具备", missing),
			})
		}
	}
	return violations
}

func normalizeCapabilityTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// formatViolationsForRetry 把违例格式化为写回分解 prompt 的反馈文本
// （Self-Refine：失败上下文必须随修复尝试注入，否则重分解会犯同样错误）。
func formatViolationsForRetry(violations []PlanViolation) string {
	var b strings.Builder
	b.WriteString("[计划校验反馈] 上一次分解未通过可行性校验，请修正后重新分解：\n")
	for _, v := range violations {
		fmt.Fprintf(&b, "- [%s] subtask %s: %s\n", v.Rule, v.SubTaskID, v.Detail)
	}
	b.WriteString("要求：required_capabilities 只能使用现有 Agent 具备的能力标签；每个子任务必须有非空 name 和 description。")
	return b.String()
}

// summarizeViolations 生成降级说明用的违例摘要（含具体标签，便于排障）。
func summarizeViolations(violations []PlanViolation) string {
	parts := make([]string, 0, len(violations))
	for _, v := range violations {
		if v.SubTaskID != "" {
			parts = append(parts, v.SubTaskID+": "+v.Detail)
		} else {
			parts = append(parts, v.Detail)
		}
	}
	return strings.Join(parts, "；")
}

// planVerifyOutcome 是校验门结果。degraded=true 时 subTasks/dag 为 nil，
// 调用方按分解失败路径降级 direct。
type planVerifyOutcome struct {
	subTasks []biz.SubTask
	dag      *biz.PlanTaskDAG
	degraded bool
	note     string // 修复/降级说明，由调用方并入 decomposeReason
}

// applyPlanVerifyGate 对分解产物执行计划校验门。通过则原样返回；违例则
// 有界重分解 1 次（违例反馈写回 prompt）；仍违例返回 degraded。
func (impl *taskPlannerImpl) applyPlanVerifyGate(ctx context.Context, subTasks []biz.SubTask, dag *biz.PlanTaskDAG, input biz.PlanInput, teamCount int, level biz.ComplexityLevel) planVerifyOutcome {
	passthrough := planVerifyOutcome{subTasks: subTasks, dag: dag}
	if impl.capBuilder == nil || len(subTasks) == 0 {
		return passthrough
	}
	caps, err := impl.capBuilder.BuildAll(ctx)
	if err != nil {
		// fail-open：能力清单是校验门的唯一外部依赖，其故障不应阻断规划。
		impl.lg.Warn("计划校验门：能力清单构建失败，跳过校验（fail-open）",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("trace_id", input.TraceID),
			loggateway.Err(err),
		)
		return passthrough
	}
	violations := verifyPlanFeasibility(subTasks, caps)
	if len(violations) == 0 {
		return passthrough
	}

	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogWarn("spirit.planner.verify", "计划校验门",
			fmt.Sprintf("分解产物未通过可行性校验（%d 项违例），尝试有界修复", len(violations)),
			event.P("violation_count", len(violations)),
			event.P("violation_summary", summarizeViolations(violations)))
	}
	impl.lg.Warn("计划校验门：分解产物存在可行性违例，尝试有界重分解",
		loggateway.StepID(biz.SpiritStepPlannerDecompose),
		loggateway.Str("trace_id", input.TraceID),
		loggateway.Int("violation_count", len(violations)),
		loggateway.Str("violation_summary", summarizeViolations(violations)),
	)

	repairFn := impl.repairDecomposeFn
	if repairFn == nil {
		repairFn = impl.decomposeTask
	}
	feedback := formatViolationsForRetry(violations)
	repaired, repairedDAG, repErr := repairFn(ctx, input.UserMessage+"\n\n"+feedback, input.IntentArtifact, teamCount, level)
	if repErr == nil && len(repaired) > 0 {
		if reViolations := verifyPlanFeasibility(repaired, caps); len(reViolations) == 0 {
			if em := event.TraceEmitterFromContext(ctx); em != nil {
				em.LogDone("spirit.planner.verify", "计划校验门修复后通过",
					event.P("violation_count", len(violations)),
					event.P("repaired_subtask_count", len(repaired)))
			}
			impl.lg.Info("计划校验门：有界重分解修复成功",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", input.TraceID),
				loggateway.Int("repaired_subtask_count", len(repaired)),
			)
			return planVerifyOutcome{
				subTasks: repaired,
				dag:      repairedDAG,
				note:     "（校验门修复后通过）",
			}
		}
	}

	note := fmt.Sprintf("计划校验未通过（%s），降级为 direct", summarizeViolations(violations))
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogWarn("spirit.planner.verify", "计划校验门",
			"有界修复后仍未通过校验，降级为 direct",
			event.P("violation_summary", summarizeViolations(violations)))
	}
	return planVerifyOutcome{degraded: true, note: note}
}
