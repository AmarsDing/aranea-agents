package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── P2 Gate：AB 对照回放棘轮（F1）+ 漂移检测（F2）+ 触发率黄金集（F4）──────
// 设计：docs/development/phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md

// fakeABReplayRunner implements SkillReplayABRunner with a programmed result.
type fakeABReplayRunner struct {
	result *SkillReplayABResult
	err    error
}

func (f *fakeABReplayRunner) ReplayAB(context.Context, string, string, int) (*SkillReplayABResult, error) {
	return f.result, f.err
}

// abResult builds a programmed AB result; baseRate < 0 means baseline unavailable.
func abResult(baseRate, draftRate float64) *SkillReplayABResult {
	ab := &SkillReplayABResult{
		Draft: &SkillReplayResult{DatasetID: "ds1", DatasetName: "ds", Total: 5, PassRate: draftRate},
	}
	if baseRate >= 0 {
		ab.Baseline = &SkillReplayResult{DatasetID: "ds1", DatasetName: "ds", Total: 5, PassRate: baseRate}
	}
	return ab
}

// 棘轮：draft 过绝对阈值但劣于基线 → 拒绝。
func TestGateVerification_ABReplay_RatchetRejects(t *testing.T) {
	runner := &fakeABReplayRunner{result: abResult(0.8, 0.6)}
	v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected ratchet rejection when draft < baseline")
	}
	c := findCheck(t, res, "functional")
	if c.Passed || !strings.Contains(c.Reason, "baseline") {
		t.Fatalf("unexpected functional check: %+v", c)
	}
}

// 棘轮：draft 不劣于基线 → 通过。
func TestGateVerification_ABReplay_RatchetPasses(t *testing.T) {
	runner := &fakeABReplayRunner{result: abResult(0.8, 0.9)}
	v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res.Checks)
	}
}

// 绝对阈值仍生效：draft 优于基线但低于 0.6 → 拒绝。
func TestGateVerification_ABReplay_AbsoluteThresholdStillApplies(t *testing.T) {
	runner := &fakeABReplayRunner{result: abResult(0.4, 0.5)}
	v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected absolute-threshold rejection")
	}
	c := findCheck(t, res, "functional")
	if c.Passed || !strings.Contains(c.Reason, "dataset replay pass rate") {
		t.Fatalf("unexpected functional check: %+v", c)
	}
}

// 基线不可得（Baseline=nil）：只做绝对阈值，棘轮跳过。
func TestGateVerification_ABReplay_NoBaseline_AbsoluteOnly(t *testing.T) {
	runner := &fakeABReplayRunner{result: abResult(-1, 0.7)}
	v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass with absolute-only check, got %+v", res.Checks)
	}
}

// AB runner 错误 → 跳过不阻断（best-effort 语义与单跑一致）。
func TestGateVerification_ABReplay_Error_Skips(t *testing.T) {
	for name, err := range map[string]error{
		"no_dataset":      ErrNoReplayDataset,
		"llm_unavailable": errors.New("no DefaultRefineLLM configured"),
	} {
		t.Run(name, func(t *testing.T) {
			runner := &fakeABReplayRunner{err: err}
			v := NewGateVerifier(nil, nil, WithABReplayRunner(runner))
			res, vErr := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
			if vErr != nil {
				t.Fatalf("Verify: %v", vErr)
			}
			if !res.Passed {
				t.Fatalf("expected skip-to-pass, got %+v", res.Checks)
			}
		})
	}
}
