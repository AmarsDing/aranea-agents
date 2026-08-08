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

// ── F2：漂移检测（Gate 第六维 drift）────────────────────────────────────────

// driftRuleBlock renders one rule block marker pair.
func driftRuleBlock(id string, helpful int) string {
	return "<!-- aranea:rule id=\"" + id + "\" helpful=" + itoa(helpful) + " -->\n规则 " + id + " 内容。\n<!-- /aranea:rule -->\n"
}

// driftBody builds a SKILL.md body with the given rule blocks (id, helpful).
func driftBody(rules ...[2]any) string {
	body := "# Skill\n\n## 规则\n\n"
	for _, r := range rules {
		body += driftRuleBlock(r[0].(string), r[1].(int)) + "\n"
	}
	return body
}

func rule(id string, helpful int) [2]any { return [2]any{id, helpful} }

// 破坏性更新：draft 删除 helpful≥3 的规则 → 拒绝。
func TestGateVerification_Drift_RemovedHighHelpfulRule_Rejects(t *testing.T) {
	current := driftBody(rule("good-rule", 5), rule("ok-rule", 1))
	draft := driftBody(rule("ok-rule", 1))
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: current}))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected drift rejection when high-helpful rule removed")
	}
	c := findCheck(t, res, "drift")
	if c.Passed || !strings.Contains(c.Reason, "good-rule") {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// 破坏性更新：当前 ≥4 规则且删除比例 >50% → 拒绝。
func TestGateVerification_Drift_RemoveRatioOver50_Rejects(t *testing.T) {
	current := driftBody(rule("r1", 0), rule("r2", 0), rule("r3", 0), rule("r4", 0))
	draft := driftBody(rule("r1", 0)) // 删 3/4 = 75%
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: current}))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "drift")
	if c.Passed || !strings.Contains(c.Reason, "removed") {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// 删除比例 ≤50%（4 删 2）且不含高 helpful → 通过。
func TestGateVerification_Drift_RemoveRatioAtHalf_Passes(t *testing.T) {
	current := driftBody(rule("r1", 0), rule("r2", 0), rule("r3", 0), rule("r4", 0))
	draft := driftBody(rule("r1", 0), rule("r2", 0))
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: current}))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "drift")
	if !c.Passed {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// 臃肿：draft 规则数 > 当前×1.5 且 > 当前+5 → 拒绝（当前 10，draft 17）。
func TestGateVerification_Drift_BloatRejects(t *testing.T) {
	var curRules [][2]any
	for i := 0; i < 10; i++ {
		curRules = append(curRules, rule("r"+itoa(i), 0))
	}
	draftRules := append(append([][2]any{}, curRules...),
		rule("n1", 0), rule("n2", 0), rule("n3", 0), rule("n4", 0), rule("n5", 0), rule("n6", 0), rule("n7", 0))
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: driftBody(curRules...)}))
	res, err := v.Verify(context.Background(), "skill1", driftBody(draftRules...), nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "drift")
	if c.Passed || !strings.Contains(c.Reason, "bloat") {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// 臃肿双条件边界：当前 4，draft 7（>1.5×=6 但不 >4+5=9）→ 通过（防小基数误杀）。
func TestGateVerification_Drift_BloatSmallBase_Passes(t *testing.T) {
	current := driftBody(rule("r1", 0), rule("r2", 0), rule("r3", 0), rule("r4", 0))
	draft := driftBody(rule("r1", 0), rule("r2", 0), rule("r3", 0), rule("r4", 0),
		rule("n1", 0), rule("n2", 0), rule("n3", 0))
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: current}))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "drift")
	if !c.Passed {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// modify/merge 不算删除：全部 id 保留（内容改写）→ 通过。
func TestGateVerification_Drift_ModifyKeepsIDs_Passes(t *testing.T) {
	current := driftBody(rule("r1", 2), rule("r2", 1))
	// 同 id，不同内容（modify）。
	draft := "# Skill\n\n## 规则\n\n<!-- aranea:rule id=\"r1\" helpful=2 -->\n改写后的内容一。\n<!-- /aranea:rule -->\n\n<!-- aranea:rule id=\"r2\" helpful=1 -->\n改写后的内容二。\n<!-- /aranea:rule -->\n"
	v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: current}))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "drift")
	if !c.Passed {
		t.Fatalf("unexpected drift check: %+v", c)
	}
}

// 跳过语义：无 lookup / 当前无规则块 / lookup 失败 → drift 通过。
func TestGateVerification_Drift_Skips(t *testing.T) {
	draft := driftBody(rule("r1", 0))
	t.Run("no_lookup", func(t *testing.T) {
		v := NewGateVerifier(nil, nil)
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "drift"); !c.Passed {
			t.Fatalf("unexpected drift check: %+v", c)
		}
	})
	t.Run("no_rule_blocks", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: "# Old\n\n无规则块。\n"}))
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "drift"); !c.Passed {
			t.Fatalf("unexpected drift check: %+v", c)
		}
	})
	t.Run("lookup_error", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{bodyErr: errors.New("db down")}))
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "drift"); !c.Passed {
			t.Fatalf("unexpected drift check: %+v", c)
		}
	})
}

// ── F4：触发率黄金集回归（Gate 第七维 trigger_accuracy）─────────────────────

// fakeTriggerGoldenRunner implements SkillTriggerGoldenRunner with a
// programmed result.
type fakeTriggerGoldenRunner struct {
	result *SkillTriggerGoldenResult
	err    error
}

func (f *fakeTriggerGoldenRunner) RunTriggerGolden(context.Context, string, string, int) (*SkillTriggerGoldenResult, error) {
	return f.result, f.err
}

// goldenResult builds a programmed golden-set result; baseRate < 0 means
// baseline unavailable (HasBaseline=false).
func goldenResult(total int, draftRate, baseRate float64) *SkillTriggerGoldenResult {
	r := &SkillTriggerGoldenResult{
		DatasetName: "sk__trigger",
		Total:       total,
		Accuracy:    draftRate,
	}
	if baseRate >= 0 {
		r.BaselineAccuracy = baseRate
		r.HasBaseline = true
	}
	return r
}

// 棘轮：draft 准确率低于当前正文 → 拒绝。
func TestGateVerification_TriggerGolden_RatchetRejects(t *testing.T) {
	runner := &fakeTriggerGoldenRunner{result: goldenResult(10, 0.9, 1.0)}
	v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected trigger_accuracy ratchet rejection")
	}
	c := findCheck(t, res, "trigger_accuracy")
	if c.Passed || !strings.Contains(c.Reason, "baseline") {
		t.Fatalf("unexpected trigger_accuracy check: %+v", c)
	}
}

// 绝对下限：无基线时 draft 准确率 < 0.8 → 拒绝。
func TestGateVerification_TriggerGolden_AbsoluteFloorRejects(t *testing.T) {
	runner := &fakeTriggerGoldenRunner{result: goldenResult(10, 0.7, -1)}
	v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected absolute-floor rejection")
	}
	c := findCheck(t, res, "trigger_accuracy")
	if c.Passed || !strings.Contains(c.Reason, "accuracy") {
		t.Fatalf("unexpected trigger_accuracy check: %+v", c)
	}
}

// 双阈值同时生效：draft 不劣于基线但低于绝对下限 → 拒绝。
func TestGateVerification_TriggerGolden_AboveBaselineBelowFloor_Rejects(t *testing.T) {
	runner := &fakeTriggerGoldenRunner{result: goldenResult(10, 0.75, 0.7)}
	v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := findCheck(t, res, "trigger_accuracy")
	if c.Passed {
		t.Fatalf("expected absolute-floor rejection, got %+v", c)
	}
}

// 双端达标 → 通过。
func TestGateVerification_TriggerGolden_Passes(t *testing.T) {
	runner := &fakeTriggerGoldenRunner{result: goldenResult(10, 0.95, 0.9)}
	v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res.Checks)
	}
	c := findCheck(t, res, "trigger_accuracy")
	if !c.Passed || !strings.Contains(c.Reason, "trigger") {
		t.Fatalf("unexpected trigger_accuracy check: %+v", c)
	}
}

// 跳过语义：runner 未接线 / runner 错误 / nil 结果 / 全非法用例 → 通过。
func TestGateVerification_TriggerGolden_Skips(t *testing.T) {
	t.Run("not_wired", func(t *testing.T) {
		v := NewGateVerifier(nil, nil)
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "trigger_accuracy"); !c.Passed {
			t.Fatalf("unexpected trigger_accuracy check: %+v", c)
		}
	})
	t.Run("runner_error", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(&fakeTriggerGoldenRunner{err: ErrNoReplayDataset}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "trigger_accuracy"); !c.Passed {
			t.Fatalf("unexpected trigger_accuracy check: %+v", c)
		}
	})
	t.Run("nil_result", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(&fakeTriggerGoldenRunner{}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "trigger_accuracy"); !c.Passed {
			t.Fatalf("unexpected trigger_accuracy check: %+v", c)
		}
	})
	t.Run("all_cases_invalid", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithTriggerGoldenRunner(&fakeTriggerGoldenRunner{result: goldenResult(0, 0, -1)}))
		res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if c := findCheck(t, res, "trigger_accuracy"); !c.Passed {
			t.Fatalf("unexpected trigger_accuracy check: %+v", c)
		}
	})
}
