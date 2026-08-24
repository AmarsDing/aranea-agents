package team

// run 级累计 input-token 预算闸（2026-08-24）。
//
// 背景：单成员 ReAct 每轮回灌全量历史，token 记账为 N 轮×历史的二次方
// 累计。agent 级 max_tool_iterations（A 方案）限制单成员单 turn 轮数，
// 但多成员/多 step 的 run 级合计此前无任何上限——实测 ops 团队单 run
// 累计 513 万 input tok（成员 eval_memory_probe 490 万 + 其余 23 万）。
// 本闸在成员行记账时累加真实消耗（attribution=="" 的行，镜像行不重复
// 累计），超阈值即通过 RunRegistry.Cancel 取消 run ctx，中止后续成员
// 调度。压缩机制（单轮窗口保护）天然拦不住此类"轮数型"累计，预算闸是
// run 级唯一兜底。
//
// 预算值解析顺序：team definition_json.token_budget_input_tokens > 0
// 用之；= 0 用 DefaultTeamRunInputTokenBudget；< 0 本团队禁用该闸。

// DefaultTeamRunInputTokenBudget is the default run-level cumulative input
// token budget. 1.5M ≈ 30 轮 × 平均 50K 的 ReAct 回灌累计，覆盖正常多
// 成员团队任务，同时远在 513 万事故量级之下。
const DefaultTeamRunInputTokenBudget int64 = 1_500_000

// resolveRunTokenBudget maps the team definition override to an effective
// limit: >0 override, 0 default, <0 disabled (returns -1).
func resolveRunTokenBudget(def Definition) int64 {
	if def.TokenBudgetInputTokens < 0 {
		return -1
	}
	if def.TokenBudgetInputTokens > 0 {
		return def.TokenBudgetInputTokens
	}
	return DefaultTeamRunInputTokenBudget
}

// registerRunTokenBudget arms the gate for a run. limit <= 0 leaves the run
// ungated (disabled override or nil maps in tests).
func (r *Runner) registerRunTokenBudget(runID string, limit int64) {
	if r == nil || runID == "" || limit <= 0 {
		return
	}
	r.budgetMu.Lock()
	defer r.budgetMu.Unlock()
	if r.budgetUsed == nil {
		r.budgetUsed = make(map[string]int64)
		r.budgetLimit = make(map[string]int64)
		r.budgetTripped = make(map[string]bool)
	}
	r.budgetUsed[runID] = 0
	r.budgetLimit[runID] = limit
	r.budgetTripped[runID] = false
}

// releaseRunTokenBudget drops the run's gate state at run end.
func (r *Runner) releaseRunTokenBudget(runID string) {
	if r == nil || runID == "" {
		return
	}
	r.budgetMu.Lock()
	defer r.budgetMu.Unlock()
	delete(r.budgetUsed, runID)
	delete(r.budgetLimit, runID)
	delete(r.budgetTripped, runID)
}

// accumulateRunTokenBudget adds a member row's input tokens to the run total
// and reports whether the budget is now exceeded. The first call that crosses
// the limit returns tripped=true; subsequent calls keep returning true but
// the caller must only fire the cancel once (guarded here by budgetTripped,
// so tripped=true is returned exactly once). Ungated runs (nil maps or
// unknown runID) always return false.
func (r *Runner) accumulateRunTokenBudget(runID string, inputTokens int) (tripped bool, used, limit int64) {
	if r == nil || runID == "" || inputTokens <= 0 {
		return false, 0, 0
	}
	r.budgetMu.Lock()
	defer r.budgetMu.Unlock()
	if r.budgetUsed == nil {
		return false, 0, 0
	}
	l, ok := r.budgetLimit[runID]
	if !ok || l <= 0 {
		return false, 0, 0
	}
	r.budgetUsed[runID] += int64(inputTokens)
	used = r.budgetUsed[runID]
	if used <= l {
		return false, used, l
	}
	if r.budgetTripped[runID] {
		// Already fired — caller must not cancel again.
		return false, used, l
	}
	r.budgetTripped[runID] = true
	return true, used, l
}
