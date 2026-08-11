package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── P3 M1：配对无退化（paired_regression）+ 等价改动检测（no_op_change）──────
// 设计依据：EverMind HarnessBank 调研评审——Gate 判定必须 per-case 归因，
// 拒绝"拆东墙补西墙"（baseline 通过的 case 在 draft 下不允许失败），
// 拒绝"换汤不换药"（draft 输出与 baseline 全部逐字节一致 = 无可测量效果）。

// verdict builds one per-case replay outcome.
func verdict(id string, passed bool, hash string) CaseVerdict {
	return CaseVerdict{CaseID: id, Passed: passed, OutputHash: hash}
}

// summarize builds a SkillReplayResult from per-case verdicts, computing the
// aggregate counters so the functional dimension sees consistent data.
func summarize(results []CaseVerdict) *SkillReplayResult {
	r := &SkillReplayResult{DatasetID: "ds1", DatasetName: "ds", Total: len(results), CaseResults: results}
	for _, v := range results {
		if v.Passed {
			r.Passed++
		}
	}
	if r.Total > 0 {
		r.PassRate = float64(r.Passed) / float64(r.Total)
	}
	return r
}

// abPaired builds an AB result with per-case verdicts on both sides.
func abPaired(base, draft []CaseVerdict) *SkillReplayABResult {
	return &SkillReplayABResult{Baseline: summarize(base), Draft: summarize(draft)}
}

// 回归拒绝：c2 在 baseline 下通过、在 draft 下失败 → 拒绝，win 不抵 regression。
func TestGateVerification_PairedRegression_RegressionRejects(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "h2"), verdict("c3", false, "h3")},
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", false, "x2"), verdict("c3", true, "x3")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected paired-regression rejection")
	}
	c := findCheck(t, res, "paired_regression")
	if c.Passed {
		t.Fatalf("expected paired_regression failure, got %+v", c)
	}
	if !strings.Contains(c.Reason, "c2") || !strings.Contains(c.Reason, "regression") {
		t.Fatalf("reason should name the regressed case, got %q", c.Reason)
	}
	// win/loss/tie 归因计数必须进入 reason（供审批审计）。
	if !strings.Contains(c.Reason, "1/1/1") {
		t.Fatalf("reason should carry win/loss/tie counts, got %q", c.Reason)
	}
}

// 无回归：draft 多赢一个（c2 反败为胜）→ 通过，reason 携带归因计数。
func TestGateVerification_PairedRegression_NoRegression_Passes(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", false, "h2")},
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "x2")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "paired_regression")
	if !c.Passed || !strings.Contains(c.Reason, "1/0/1") {
		t.Fatalf("unexpected paired_regression check: %+v", c)
	}
	if !res.Passed {
		t.Fatalf("expected overall pass, got %+v", res.Checks)
	}
}

// 跳过语义：数据不可用时一律通过（不阻断），与既有维度 best-effort 风格一致。
func TestGateVerification_PairedRegression_Skips(t *testing.T) {
	t.Run("ab_error", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{err: ErrNoReplayDataset}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "paired_regression"); !c.Passed {
			t.Fatalf("expected skip-to-pass, got %+v", c)
		}
	})
	t.Run("baseline_nil", func(t *testing.T) {
		ab := abResult(-1, 0.8) // legacy builder: no per-case data, baseline nil
		v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "paired_regression"); !c.Passed {
			t.Fatalf("expected skip-to-pass, got %+v", c)
		}
	})
	t.Run("no_case_results", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: abResult(0.6, 0.8)}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "paired_regression"); !c.Passed {
			t.Fatalf("expected skip-to-pass, got %+v", c)
		}
	})
	t.Run("case_id_mismatch", func(t *testing.T) {
		ab := abPaired(
			[]CaseVerdict{verdict("c1", true, "h1")},
			[]CaseVerdict{verdict("cX", false, "h1")},
		)
		v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "paired_regression"); !c.Passed {
			t.Fatalf("expected skip-to-pass on unpairable sets, got %+v", c)
		}
	})
	t.Run("ab_runner_not_wired", func(t *testing.T) {
		v := NewGateVerifier(nil, nil)
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "paired_regression"); !c.Passed {
			t.Fatalf("expected skip-to-pass, got %+v", c)
		}
	})
}

// 等价改动拒绝：draft 输出与 baseline 全部逐字节一致 → 无可测量效果，拒绝。
func TestGateVerification_NoOpChange_AllIdenticalRejects(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "h2")},
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "h2")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected no-op rejection when all outputs byte-identical")
	}
	c := findCheck(t, res, "no_op_change")
	if c.Passed || !strings.Contains(c.Reason, "no measurable effect") {
		t.Fatalf("unexpected no_op_change check: %+v", c)
	}
	// 配对维度此时应通过（无回归），拒绝仅来自 no_op_change。
	if c := findCheck(t, res, "paired_regression"); !c.Passed {
		t.Fatalf("paired_regression should pass on identical verdicts, got %+v", c)
	}
}

// 有差异即通过：任一 case 输出不同 → 存在可测量效果。
func TestGateVerification_NoOpChange_Differs_Passes(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "h2")},
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", true, "x2")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c := findCheck(t, res, "no_op_change"); !c.Passed {
		t.Fatalf("expected pass when any output differs, got %+v", c)
	}
}

// LLM 调用失败侧（空 hash）不参与可比性判定；仅剩一个可比 case 且一致 → 仍拒绝。
func TestGateVerification_NoOpChange_FailedCasesExcluded(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", false, "")},
		[]CaseVerdict{verdict("c1", true, "h1"), verdict("c2", false, "")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c := findCheck(t, res, "no_op_change"); c.Passed {
		t.Fatalf("expected rejection: sole comparable case is identical, got %+v", c)
	}
}

// 无可比 case（一侧全部 LLM 失败）→ 无法证明无效果 → 跳过通过。
func TestGateVerification_NoOpChange_NoComparable_Skips(t *testing.T) {
	ab := abPaired(
		[]CaseVerdict{verdict("c1", true, "h1")},
		[]CaseVerdict{verdict("c1", false, "")},
	)
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{result: ab}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c := findCheck(t, res, "no_op_change"); !c.Passed {
		t.Fatalf("expected skip-to-pass, got %+v", c)
	}
}

// AB runner 错误 → no_op 跳过不阻断。
func TestGateVerification_NoOpChange_Error_Skips(t *testing.T) {
	v := NewGateVerifier(nil, nil, WithABReplayRunner(&fakeABReplayRunner{err: errors.New("llm unavailable")}))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c := findCheck(t, res, "no_op_change"); !c.Passed {
		t.Fatalf("expected skip-to-pass, got %+v", c)
	}
}

// 成本纪律：draft 结构非法（base functional 失败）时不得触发 AB 回放。
func TestGateVerification_ABReplay_NotTriggeredWhenBaseFails(t *testing.T) {
	runner := &fakeABReplayRunner{result: abPaired(
		[]CaseVerdict{verdict("c1", true, "h1")},
		[]CaseVerdict{verdict("c1", true, "h1")},
	)}
	v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", "", nil) // 空 draft
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected base functional rejection on empty draft")
	}
	if runner.calls != 0 {
		t.Fatalf("AB replay must not run when base functional fails, got %d calls", runner.calls)
	}
}
