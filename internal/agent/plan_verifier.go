package agent

import (
	"context"
	"fmt"
	"sort"
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
	// PlanViolationTeamCountMismatch 子任务数量与用户显式请求的团队数不一致
	// （每个子任务对应一个独立团队）。数量不符时若在分解后静默截取，会整个
	// 丢掉团队的职责（2026-08-21 诗歌会话丢 Team B 根因），必须经校验门
	// 有界重分解；该规则单独不触发降级 direct（数量问题有兜底路径）。
	PlanViolationTeamCountMismatch = "team_count_mismatch"
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
//   - R4 team_count_mismatch：teamCount > 0（用户显式请求 N 个团队）且
//     子任务数 != teamCount。
func verifyPlanFeasibility(subTasks []biz.SubTask, capabilities []biz.AgentCapability, teamCount int) []PlanViolation {
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

	if teamCount > 0 && len(subTasks) != teamCount {
		violations = append(violations, PlanViolation{
			Rule:   PlanViolationTeamCountMismatch,
			Detail: fmt.Sprintf("子任务数量 %d 与用户显式请求的团队数量 %d 不一致", len(subTasks), teamCount),
		})
	}

	roleUnion := make(map[string]struct{})
	for _, tag := range capabilityRoleUnion(capabilities) {
		roleUnion[tag] = struct{}{}
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

// capabilityRoleUnion 返回能力清单中全部 Agent Roles 的归一化并集（排序后
// 确定性输出）——校验门 R2 与分解 prompt 能力标签注入共用同一数据源，
// 保证「prompt 允许用的标签」与「校验门认可的标签」永不发散。
func capabilityRoleUnion(caps []biz.AgentCapability) []string {
	set := make(map[string]struct{})
	for _, cap := range caps {
		for _, r := range cap.Roles {
			if nr := normalizeCapabilityTag(r); nr != "" {
				set[nr] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
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
	for _, v := range violations {
		if v.Rule == PlanViolationTeamCountMismatch {
			b.WriteString(" 用户明确请求了固定数量的团队，本次必须输出与该数量完全一致的子任务数——每个子任务对应一个独立团队，禁止多出或缺少；如某个子任务只是准备/了解现状性质，请把它并入对应创作子任务的 description，而不是单独占一个团队名额。")
			break
		}
	}
	return b.String()
}

// onlyTeamCountViolations 报告违例集合是否仅含团队数量不符——该规则单独
// 不触发降级 direct（数量问题由调用方的截取/告警兜底，整体废弃可执行计划
// 代价更大）。
func onlyTeamCountViolations(violations []PlanViolation) bool {
	if len(violations) == 0 {
		return false
	}
	for _, v := range violations {
		if v.Rule != PlanViolationTeamCountMismatch {
			return false
		}
	}
	return true
}

// hasBlockingViolations 报告违例集合是否含阻断级违例（R1 空定义 / R3 病态
// 规模——需要重分解，修复失败则降级）。R4 数量不符触发重分解但永不降级
// （数量兜底路径）；R2 能力违例见 filterWarnOnlyViolations。
func hasBlockingViolations(violations []PlanViolation) bool {
	for _, v := range violations {
		if v.Rule != PlanViolationCapabilityUnsatisfiable && v.Rule != PlanViolationTeamCountMismatch {
			return true
		}
	}
	return false
}

// filterWarnOnlyViolations 把 R2 能力违例从可行动违例集中剥离（返回剩余
// 可行动违例与被剥离的 R2 违例）。2026-09-04（项 3d）名册治理期止血：
// 名册 roles 覆盖不足（337 个 Agent 中仅 ops_* 运维系列有有效标签，业务
// Agent roles_json 全为 null）时，业务任务的 R2 误杀率 100%（2026-09-04
// 营销任务实证：4/4 子任务违例、有界重分解空耗 42s+ 后仍降级），且名册
// 缺口是确定性的——重分解不可能修复，纯浪费一次 LLM 往返。R2 的设计前提
// 是名册完整，属于运维态数据缺陷而非计划违例；剥离后仅记 warn 保留观测，
// 待项 3c 名册补齐后恢复可行动（重分解+降级）。R4/R1/R3 语义不变。
func filterWarnOnlyViolations(violations []PlanViolation) (actionable, capability []PlanViolation) {
	for _, v := range violations {
		if v.Rule == PlanViolationCapabilityUnsatisfiable {
			capability = append(capability, v)
			continue
		}
		actionable = append(actionable, v)
	}
	return actionable, capability
}

// logCapabilityWarnOnly 记录 R2 能力违例 warn-only 放行（首检与复检共用）。
func (impl *taskPlannerImpl) logCapabilityWarnOnly(ctx context.Context, input biz.PlanInput, capViolations []PlanViolation) {
	if len(capViolations) == 0 {
		return
	}
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogWarn("spirit.planner.verify", "计划校验门",
			fmt.Sprintf("分解产物存在 %d 项能力违例，warn-only 放行（名册治理期）", len(capViolations)),
			event.P("violation_count", len(capViolations)),
			event.P("violation_summary", summarizeViolations(capViolations)))
	}
	impl.lg.Warn("计划校验门：能力违例 warn-only 放行（名册治理期）",
		loggateway.StepID(biz.SpiritStepPlannerDecompose),
		loggateway.Str("trace_id", input.TraceID),
		loggateway.Int("violation_count", len(capViolations)),
		loggateway.Str("violation_summary", summarizeViolations(capViolations)),
	)
}

// summarizeViolations 生成降级说明用的违例摘要（含规则名与具体标签，便于排障
// grep——规则名是稳定标识，Detail 为易变自然语言）。
func summarizeViolations(violations []PlanViolation) string {
	parts := make([]string, 0, len(violations))
	for _, v := range violations {
		if v.SubTaskID != "" {
			parts = append(parts, fmt.Sprintf("%s[%s]: %s", v.SubTaskID, v.Rule, v.Detail))
		} else {
			parts = append(parts, fmt.Sprintf("[%s] %s", v.Rule, v.Detail))
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
	violations := verifyPlanFeasibility(subTasks, caps, teamCount)
	if len(violations) == 0 {
		return passthrough
	}

	// 2026-09-04（项 3d）：剥离 R2 能力违例（warn-only，名册治理期），
	// 剩余可行动违例（R1/R3 阻断级、R4 数量级）维持原有重分解/兜底语义。
	actionable, capViolations := filterWarnOnlyViolations(violations)
	impl.logCapabilityWarnOnly(ctx, input, capViolations)
	if len(actionable) == 0 {
		return planVerifyOutcome{
			subTasks: subTasks,
			dag:      dag,
			note:     "（能力违例 warn-only 放行，名册治理期）",
		}
	}
	violations = actionable

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
		reViolations := verifyPlanFeasibility(repaired, caps, teamCount)
		// 2026-09-04（项 3d）：复检与首检同契约——R2 能力违例 warn-only 剥离。
		// 漏剥离时混合违例（R2+R4）的修复产物会被误判「修复失败」而回退原计划，
		// 修复结果整体丢失（TestPlanVerifyGate_MixedViolations 实证）。
		reActionable, reCapViolations := filterWarnOnlyViolations(reViolations)
		impl.logCapabilityWarnOnly(ctx, input, reCapViolations)
		reViolations = reActionable
		if len(reViolations) == 0 {
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
		if onlyTeamCountViolations(reViolations) {
			// 修复后仅余数量不符：不降级 direct（会把可执行计划整体废弃），
			// 返回修复后产物，由调用方的数量兜底（截取/告警 + 前端通知）处理。
			impl.lg.Warn("计划校验门：修复后团队数量仍不符，交由数量兜底处理",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", input.TraceID),
				loggateway.Str("violation_summary", summarizeViolations(reViolations)),
			)
			return planVerifyOutcome{
				subTasks: repaired,
				dag:      repairedDAG,
				note:     "（校验门修复后团队数量仍不符，已按数量兜底处理）",
			}
		}
	}

	// 修复失败或仍含非数量违例：若原始违例仅数量不符，同样不降级——原样
	// 返回交调用方数量兜底；否则按分解失败路径降级 direct。
	if onlyTeamCountViolations(violations) {
		impl.lg.Warn("计划校验门：团队数量不符且有界修复失败，交由数量兜底处理",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("trace_id", input.TraceID),
			loggateway.Str("violation_summary", summarizeViolations(violations)),
		)
		return planVerifyOutcome{
			subTasks: subTasks,
			dag:      dag,
			note:     "（团队数量不符，校验门修复未果，已按数量兜底处理）",
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
