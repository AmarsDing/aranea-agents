package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// fakeReplayRunner implements SkillReplayRunner with a programmed result.
type fakeReplayRunner struct {
	result *SkillReplayResult
	err    error
}

func (f *fakeReplayRunner) Replay(context.Context, string, string, int) (*SkillReplayResult, error) {
	return f.result, f.err
}

// ── helpers ──────────────────────────────────────────────────────────────────

const p1TestDraft = "# Skill\n\n## 规则\n\n可操作内容。\n"

func findCheck(t *testing.T, res *GateVerificationResult, name string) GateCheckResult {
	t.Helper()
	for _, c := range res.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in %+v", name, res.Checks)
	return GateCheckResult{}
}

// ── verifyReplay（Solve 接线）────────────────────────────────────────────────

func TestGateVerification_ReplayBelowThreshold_Rejects(t *testing.T) {
	runner := &fakeReplayRunner{result: &SkillReplayResult{
		DatasetID: "ds1", DatasetName: "ds", Total: 5, Passed: 1, PassRate: 0.2,
	}}
	v := NewGateVerifier(nil, nil, WithReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected rejection when replay pass rate < threshold")
	}
	c := findCheck(t, res, "functional")
	if c.Passed || !strings.Contains(c.Reason, "dataset replay pass rate") {
		t.Fatalf("unexpected functional check: %+v", c)
	}
}

func TestGateVerification_ReplayAboveThreshold_Passes(t *testing.T) {
	runner := &fakeReplayRunner{result: &SkillReplayResult{
		DatasetID: "ds1", DatasetName: "ds", Total: 5, Passed: 4, PassRate: 0.8,
	}}
	v := NewGateVerifier(nil, nil, WithReplayRunner(runner))
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res.Checks)
	}
	c := findCheck(t, res, "functional")
	if !c.Passed || !strings.Contains(c.Reason, "dataset replay passed") {
		t.Fatalf("unexpected functional check: %+v", c)
	}
}

// 跳过语义：无绑定数据集 / LLM 未配置 / runner 内部错误均不阻断（best-effort）。
func TestGateVerification_ReplayError_Skips(t *testing.T) {
	for name, err := range map[string]error{
		"no_dataset":      ErrNoReplayDataset,
		"llm_unavailable": errors.New("no DefaultRefineLLM configured"),
	} {
		t.Run(name, func(t *testing.T) {
			runner := &fakeReplayRunner{err: err}
			v := NewGateVerifier(nil, nil, WithReplayRunner(runner))
			res, vErr := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
			if vErr != nil {
				t.Fatalf("Verify: %v", vErr)
			}
			if !res.Passed {
				t.Fatalf("expected skip-to-pass, got %+v", res.Checks)
			}
			c := findCheck(t, res, "functional")
			if !c.Passed || !strings.Contains(c.Reason, "skipped") {
				t.Fatalf("unexpected functional check: %+v", c)
			}
		})
	}
}

func TestGateVerification_NoReplayRunner_FunctionalBaseOnly(t *testing.T) {
	v := NewGateVerifier(nil, nil)
	res, err := v.Verify(context.Background(), "skill1", p1TestDraft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res.Checks)
	}
}

// ── verifyEffectiveness（计数归因）───────────────────────────────────────────

const p1HarmfulCurrentBody = `# Skill

## 规则

<!-- aranea:rule id="timeout-retry" helpful=1 harmful=3 -->
超时后先重试一次。
<!-- /aranea:rule -->

<!-- aranea:rule id="keep-me" helpful=2 -->
有效规则。
<!-- /aranea:rule -->
`

// harmful=3 的规则在 draft 中原样保留 → 拒绝。
func TestGateVerification_Effectiveness_HarmfulRuleKeptUnchanged_Rejects(t *testing.T) {
	skills := &fakeSkillLookupReader{body: p1HarmfulCurrentBody}
	draft := `# Skill

## 规则

<!-- aranea:rule id="timeout-retry" harmful=3 -->
超时后先重试一次。
<!-- /aranea:rule -->

<!-- aranea:rule id="keep-me" helpful=2 -->
有效规则。
<!-- /aranea:rule -->
`
	v := NewGateVerifier(nil, nil, WithSkillLookup(skills))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Passed {
		t.Fatal("expected rejection when harmful rule kept unchanged")
	}
	c := findCheck(t, res, "effectiveness")
	if c.Passed || !strings.Contains(c.Reason, "timeout-retry") {
		t.Fatalf("unexpected effectiveness check: %+v", c)
	}
}

// harmful=3 的规则被重写或移除 → 通过。
func TestGateVerification_Effectiveness_HarmfulRuleRewrittenOrRemoved_Passes(t *testing.T) {
	skills := &fakeSkillLookupReader{body: p1HarmfulCurrentBody}
	drafts := map[string]string{
		"rewritten": `# Skill

## 规则

<!-- aranea:rule id="timeout-retry" harmful=3 -->
超时后指数退避重试一次，再降级到备选工具。
<!-- /aranea:rule -->

<!-- aranea:rule id="keep-me" helpful=2 -->
有效规则。
<!-- /aranea:rule -->
`,
		"removed": `# Skill

## 规则

<!-- aranea:rule id="keep-me" helpful=2 -->
有效规则。
<!-- /aranea:rule -->
`,
	}
	for name, draft := range drafts {
		t.Run(name, func(t *testing.T) {
			v := NewGateVerifier(nil, nil, WithSkillLookup(skills))
			res, err := v.Verify(context.Background(), "skill1", draft, nil)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !res.Passed {
				t.Fatalf("expected pass, got %+v", res.Checks)
			}
		})
	}
}

// harmful 未达阈值的规则原样保留不触发拒绝。
func TestGateVerification_Effectiveness_BelowThresholdRuleKept_Passes(t *testing.T) {
	skills := &fakeSkillLookupReader{body: p1HarmfulCurrentBody}
	// 与当前正文完全一致：keep-me（harmful=0）原样保留，timeout-retry 被移除。
	draft := `# Skill

## 规则

<!-- aranea:rule id="keep-me" helpful=2 -->
有效规则。
<!-- /aranea:rule -->
`
	v := NewGateVerifier(nil, nil, WithSkillLookup(skills))
	res, err := v.Verify(context.Background(), "skill1", draft, nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got %+v", res.Checks)
	}
}

// nil-safe 跳过：无 skillLookup / 正文无规则块 / 查询失败均通过。
func TestGateVerification_Effectiveness_Skips(t *testing.T) {
	draft := p1TestDraft
	t.Run("no_lookup", func(t *testing.T) {
		v := NewGateVerifier(nil, nil)
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil || !res.Passed {
			t.Fatalf("expected pass, got res=%+v err=%v", res, err)
		}
	})
	t.Run("no_rule_blocks", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{body: "# Old\n\n无规则块。\n"}))
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil || !res.Passed {
			t.Fatalf("expected pass, got res=%+v err=%v", res, err)
		}
	})
	t.Run("lookup_error", func(t *testing.T) {
		v := NewGateVerifier(nil, nil, WithSkillLookup(&fakeSkillLookupReader{bodyErr: errors.New("db down")}))
		res, err := v.Verify(context.Background(), "skill1", draft, nil)
		if err != nil || !res.Passed {
			t.Fatalf("expected pass, got res=%+v err=%v", res, err)
		}
	})
}
