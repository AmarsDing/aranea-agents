package eval

import (
	"errors"
	"fmt"
	"strings"
)

// MRRRegressionThreshold 是门禁层每用例 chunk MRR 跌幅阈值。DiffReports 的
// Regression 标志保持 recall-only 的库契约（eval_test.go 已钉死小 MRR 波动不
// 触发），排名质量回归改由金标门禁在 diff 上单独卡：fixture 全确定性，任何
// MRR 跌幅都是真实信号而非噪声。守卫对象恰是本门禁覆盖的 GraphExpander 邻居
// 打分与 MergeSearchResults 排序。
const MRRRegressionThreshold = 0.02

// GateKind 标识一类金标门禁违规。
type GateKind string

const (
	// GateStale：基线快照与运行口径（set name / topK）不一致，快照过期。
	GateStale GateKind = "stale-baseline"
	// GateDegradedBaseline：入库基线本身带错或非满分——无论手改还是错误
	// 时刻写入，降级基线都让后续所有 diff 对着弱化参照量，必须立即红。
	GateDegradedBaseline GateKind = "degraded-baseline"
	// GateSearchError：候选运行有用例检索失败（如语料缺种子锚点）。
	GateSearchError GateKind = "search-error"
	// GateDrift：用例集合与快照不一致（added/removed）。
	GateDrift GateKind = "set-drift"
	// GateRecallRegression：用例 chunk recall 跌幅越过 RegressionThreshold。
	GateRecallRegression GateKind = "recall-regression"
	// GateMRRRegression：用例 chunk MRR 跌幅越过 MRRRegressionThreshold
	//（纯排序/打分回归，recall 不变，recall-only 判据放行的那一类）。
	GateMRRRegression GateKind = "mrr-regression"
)

// GateViolation 是一条门禁阻断条件；Detail 面向人读（测试失败输出 /
// goldeval stdout / CI 汇总）。
type GateViolation struct {
	Kind   GateKind `json:"kind"`
	CaseID string   `json:"case_id,omitempty"`
	Detail string   `json:"detail"`
}

func (v GateViolation) String() string {
	if v.CaseID != "" {
		return fmt.Sprintf("[%s] %s: %s", v.Kind, v.CaseID, v.Detail)
	}
	return fmt.Sprintf("[%s] %s", v.Kind, v.Detail)
}

// CheckGoldGate 是金标快照门禁的单一事实源——gold_snapshot_test.go 与
// cmd/goldeval 共用，防止两份手抄分支各自演化（审查 F5：二者已在 -update
// 错误守卫上分叉过一次）。返回空 = 门禁通过；违规按确定性顺序枚举。
//
// 互补而非替代：update 路径写盘前走 ValidateGoldBaseline，加载评测集后走
// ValidateGoldExpectations。
func CheckGoldGate(base, cand RunReport) []GateViolation {
	if base.SetName != cand.SetName || base.TopK != cand.TopK {
		return []GateViolation{{
			Kind: GateStale,
			Detail: fmt.Sprintf("snapshot set=%q topK=%d vs run set=%q topK=%d; refresh with -gold.update / goldeval -update",
				base.SetName, base.TopK, cand.SetName, cand.TopK),
		}}
	}

	var out []GateViolation
	for _, c := range base.Cases {
		if reason := baselineDefect(c); reason != "" {
			out = append(out, GateViolation{Kind: GateDegradedBaseline, CaseID: c.CaseID, Detail: reason})
		}
	}
	for _, c := range cand.Cases {
		if c.Err != "" {
			out = append(out, GateViolation{Kind: GateSearchError, CaseID: c.CaseID, Detail: c.Err})
		}
	}

	diff := DiffReports(base, cand)
	if len(diff.AddedCaseIDs) > 0 || len(diff.RemovedCaseIDs) > 0 {
		out = append(out, GateViolation{
			Kind:   GateDrift,
			Detail: fmt.Sprintf("added=%v removed=%v; refresh snapshot in the same PR", diff.AddedCaseIDs, diff.RemovedCaseIDs),
		})
	}
	for _, c := range diff.Cases {
		if c.Regression {
			out = append(out, GateViolation{Kind: GateRecallRegression, CaseID: c.CaseID,
				Detail: fmt.Sprintf("chunk recall %+.3f (drop > %.3f)", c.ChunkRecall, RegressionThreshold)})
		}
		if c.ChunkMRR < -(MRRRegressionThreshold + 1e-9) {
			out = append(out, GateViolation{Kind: GateMRRRegression, CaseID: c.CaseID,
				Detail: fmt.Sprintf("chunk mrr %+.3f (drop > %.3f)", c.ChunkMRR, MRRRegressionThreshold)})
		}
	}
	return out
}

// ValidateGoldBaseline 是 -update / -gold.update 刷新快照的写盘守门：基线必须
// 是一次完美运行——无带错用例、每用例 chunk/doc recall 与 chunk/doc MRR 均满分。
//
// 金标集按构造是灵敏度探针（期望 = 种子∪一跳全集、topK=8 容纳得下），正确实现
// 必然满分；把降级快照烘成基线会让其后的分部全部对着弱化参照量，门禁名存实亡。
// 若未来金标构造有意变更到非满分，必须显式改这里并在 PR 中说明。
func ValidateGoldBaseline(report RunReport) error {
	if len(report.Cases) == 0 {
		return errors.New("eval: report has no cases")
	}
	var problems []string
	for _, c := range report.Cases {
		if reason := baselineDefect(c); reason != "" {
			problems = append(problems, c.CaseID+": "+reason)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("eval: refusing imperfect gold baseline (%d problem(s)):\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

// baselineDefect 返回该结果作为金标基线的缺陷描述；空串 = 合格（完美运行）。
func baselineDefect(c CaseResult) string {
	if c.Err != "" {
		return "search error: " + c.Err
	}
	if c.Chunk.RecallAtK < 1.0 || c.Doc.RecallAtK < 1.0 {
		return fmt.Sprintf("degraded recall (chunk %.3f, doc %.3f) — gold baselines must be perfect",
			c.Chunk.RecallAtK, c.Doc.RecallAtK)
	}
	if c.Chunk.MRR < 1.0 || c.Doc.MRR < 1.0 {
		return fmt.Sprintf("degraded mrr (chunk %.3f, doc %.3f) — gold baselines must be perfect",
			c.Chunk.MRR, c.Doc.MRR)
	}
	return ""
}

// ValidateGoldExpectations 要求金标集每用例至少携带一项期望（chunk 或 doc id）。
// 空期望用例按 vacuous-success 惯例恒得 1.0/1.0，等于免疫一切回归；更糟的是
// 弱化既有用例的期望会“提升”指标而非被判回归，可在无关 PR 里偷走门禁灵敏度。
// 仅用于金标门禁路径——库的 vacuous-success 语义对通用评测集仍然成立。
func ValidateGoldExpectations(set EvalSet) error {
	for _, c := range set.Cases {
		if len(c.ExpectedChunkIDs) == 0 && len(c.ExpectedDocIDs) == 0 {
			return fmt.Errorf("eval: gold case %q must expect at least one chunk or doc id", c.ID)
		}
	}
	return nil
}
