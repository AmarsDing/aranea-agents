package biz

import (
	"context"
	"fmt"
)

// ── P2 F4：触发率黄金集回归（Gate 第七维 trigger_accuracy）──────────────────
// 设计：docs/development/phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md §6
//
// description/triggers 决定 skill 是否被调用，是独立于功能正确性的第二评测轴。
// 运行时触发为确定性机制（skillruntime.matchTrigger：CJK 子串 / ASCII 词边界），
// 因此黄金集评测无需 LLM，完全确定性。

const (
	// TriggerGoldenMinAccuracy 是 draft 触发判定准确率的绝对下限，低于该值
	// Gate trigger_accuracy 维拒绝（无论棘轮是否生效）。
	TriggerGoldenMinAccuracy = 0.8

	// TriggerGoldenMaxCases 是单次黄金集回归最多评测的用例数（skill-creator
	// 式 20 查询迭代规模）。
	TriggerGoldenMaxCases = 20
)

// SkillTriggerGoldenResult 是一次触发率黄金集回归的双端结果。
type SkillTriggerGoldenResult struct {
	DatasetName      string
	Total            int     // 合法用例数（非法 ExpectedOutput 已剔除）
	Accuracy         float64 // draft 判定正确率
	BaselineAccuracy float64 // 当前线上正文判定正确率（棘轮对照端）
	HasBaseline      bool    // 当前正文可解析时 true；false 时 Gate 跳过棘轮仅查绝对下限
	FalseNeg         int     // should-trigger 未命中（draft 端）
	FalsePos         int     // should-not-trigger 误中（draft 端）
}

// SkillTriggerGoldenRunner 用黄金集（should-trigger / should-not-trigger 查询）
// 回归 draft 与当前正文的 frontmatter triggers 判定准确率。
//
// 语义约定（best-effort，与回放一致）：
//   - 无绑定数据集（{skill.Name|Slug}__trigger）→ 返回 ErrNoReplayDataset（Gate 跳过）
//   - 数据集为空 → 返回 ErrNoReplayDataset（Gate 跳过）
//   - 全部用例非法 → 返回 Total=0 结果（Gate 跳过）
//
// Stability:evolving
type SkillTriggerGoldenRunner interface {
	RunTriggerGolden(ctx context.Context, skillID string, draftBody string, maxCases int) (*SkillTriggerGoldenResult, error)
}

// WithTriggerGoldenRunner enables the trigger-accuracy golden-set regression
// (P2 F4): the draft's frontmatter triggers must not regress below the current
// body's trigger accuracy, and must stay above the absolute floor.
func WithTriggerGoldenRunner(r SkillTriggerGoldenRunner) GateOption {
	return func(v *GateVerifier) { v.triggerGoldenRunner = r }
}

// verifyTriggerAccuracy is the seventh Gate dimension (P2 F4). Verdicts:
//   - runner not wired / no dataset / empty dataset / all cases invalid → skip, pass
//   - baseline available and draft accuracy < baseline → ratchet rejection
//   - draft accuracy < TriggerGoldenMinAccuracy → absolute-floor rejection
//   - otherwise → pass
func (v *GateVerifier) verifyTriggerAccuracy(ctx context.Context, skillID string, draftBody string) GateCheckResult {
	if v.triggerGoldenRunner == nil {
		return GateCheckResult{Name: "trigger_accuracy", Passed: true}
	}
	res, err := v.triggerGoldenRunner.RunTriggerGolden(ctx, skillID, draftBody, TriggerGoldenMaxCases)
	if err != nil || res == nil || res.Total == 0 {
		// ErrNoReplayDataset / 空数据集 / 全非法用例均跳过，不阻断。
		return GateCheckResult{Name: "trigger_accuracy", Passed: true, Reason: "trigger golden regression skipped"}
	}
	// 棘轮：draft 不得劣于当前正文（baseline 不可得时跳过）。
	if res.HasBaseline && res.Accuracy < res.BaselineAccuracy {
		return GateCheckResult{
			Name:   "trigger_accuracy",
			Passed: false,
			Reason: fmt.Sprintf("trigger accuracy ratchet: draft %.0f%% < baseline %.0f%% (dataset=%s, %d cases, FalseNeg=%d FalsePos=%d)",
				res.Accuracy*100, res.BaselineAccuracy*100, res.DatasetName, res.Total, res.FalseNeg, res.FalsePos),
		}
	}
	// 绝对下限。
	if res.Accuracy < TriggerGoldenMinAccuracy {
		return GateCheckResult{
			Name:   "trigger_accuracy",
			Passed: false,
			Reason: fmt.Sprintf("trigger accuracy %.0f%% < %.0f%% (dataset=%s, %d cases, FalseNeg=%d FalsePos=%d)",
				res.Accuracy*100, TriggerGoldenMinAccuracy*100, res.DatasetName, res.Total, res.FalseNeg, res.FalsePos),
		}
	}
	return GateCheckResult{
		Name:   "trigger_accuracy",
		Passed: true,
		Reason: fmt.Sprintf("trigger golden passed (dataset=%s, accuracy %.0f%%, %d cases)", res.DatasetName, res.Accuracy*100, res.Total),
	}
}
