package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// ── P2 F3：SuccessTrigger 成功沉淀触发器 ────────────────────────────────────
// 设计：docs/development/phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md §5

func successMetrics(count int, rate float64) *SkillHealthMetrics {
	return &SkillHealthMetrics{
		SkillID:         "sk-1",
		InvocationCount: count,
		SuccessRate:     rate,
	}
}

func successLookupWithRules() *fakeSkillLookupReader {
	return &fakeSkillLookupReader{body: "# S\n\n<!-- aranea:rule id=\"r1\" helpful=4 -->\n有效规则。\n<!-- /aranea:rule -->\n"}
}

// 全部条件满足 → 发 1 条 success 建议，字段与 metadata 完整。
func TestSuccessTrigger_Fires(t *testing.T) {
	agg := &fakeAggregator{metrics: successMetrics(50, 0.962)}
	tr := NewSuccessTrigger(agg, successLookupWithRules(), nil)

	sugs, err := tr.Check(context.Background(), "sk-1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(sugs) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(sugs))
	}
	s := sugs[0]
	if s.TargetType != EvolutionTargetSkill || s.TargetID != "sk-1" {
		t.Fatalf("unexpected target: %+v", s)
	}
	if s.ActionType != EvolutionActionImprove {
		t.Fatalf("unexpected action: %v", s.ActionType)
	}
	if s.TriggerSource != "success" {
		t.Fatalf("unexpected source: %v", s.TriggerSource)
	}
	if !strings.Contains(s.TriggerReason, "96.2%") || !strings.Contains(s.TriggerReason, "50") {
		t.Fatalf("reason should carry success stats, got %q", s.TriggerReason)
	}
	if got := s.MetaString("trigger_source"); got != "success" {
		t.Fatalf("metadata trigger_source = %q", got)
	}
	if got := s.MetaString(EvoMetaLegacyType); got != string(EvoSuggestionSuccessPattern) {
		t.Fatalf("metadata legacy_type = %q", got)
	}
	var rate float64
	if raw := s.MetaRaw("success_rate"); len(raw) == 0 {
		t.Fatal("metadata success_rate missing")
	} else if err := json.Unmarshal(raw, &rate); err != nil || rate != 0.962 {
		t.Fatalf("metadata success_rate = %s (err=%v)", raw, err)
	}
}

// 成功率低于阈值 → 无建议。
func TestSuccessTrigger_BelowSuccessRate(t *testing.T) {
	agg := &fakeAggregator{metrics: successMetrics(50, 0.80)}
	tr := NewSuccessTrigger(agg, successLookupWithRules(), nil)
	sugs, err := tr.Check(context.Background(), "sk-1")
	if err != nil || len(sugs) != 0 {
		t.Fatalf("expected no suggestion, got sugs=%v err=%v", sugs, err)
	}
}

// 调用量不足（< EvoTriggerMinInvocations）→ 无建议。
func TestSuccessTrigger_InsufficientInvocations(t *testing.T) {
	agg := &fakeAggregator{metrics: successMetrics(EvoTriggerMinInvocations-1, 0.99)}
	tr := NewSuccessTrigger(agg, successLookupWithRules(), nil)
	sugs, err := tr.Check(context.Background(), "sk-1")
	if err != nil || len(sugs) != 0 {
		t.Fatalf("expected no suggestion, got sugs=%v err=%v", sugs, err)
	}
}

// 当前正文无规则块 → 跳过（对健康 skill 做全量重写风险大于收益）。
func TestSuccessTrigger_NoRuleBlocks_Skips(t *testing.T) {
	agg := &fakeAggregator{metrics: successMetrics(50, 0.95)}
	tr := NewSuccessTrigger(agg, &fakeSkillLookupReader{body: "# S\n\n无规则块。\n"}, nil)
	sugs, err := tr.Check(context.Background(), "sk-1")
	if err != nil || len(sugs) != 0 {
		t.Fatalf("expected no suggestion, got sugs=%v err=%v", sugs, err)
	}
}

// 依赖降级：nil aggregator / 指标查询失败 / 正文查询失败 → 不触发。
func TestSuccessTrigger_Degrades(t *testing.T) {
	t.Run("nil_aggregator", func(t *testing.T) {
		tr := NewSuccessTrigger(nil, successLookupWithRules(), nil)
		sugs, err := tr.Check(context.Background(), "sk-1")
		if err != nil || len(sugs) != 0 {
			t.Fatalf("expected nil, got sugs=%v err=%v", sugs, err)
		}
	})
	t.Run("metrics_error", func(t *testing.T) {
		agg := &fakeAggregator{err: errors.New("db down")}
		tr := NewSuccessTrigger(agg, successLookupWithRules(), nil)
		sugs, err := tr.Check(context.Background(), "sk-1")
		if err == nil || len(sugs) != 0 {
			t.Fatalf("expected error propagation, got sugs=%v err=%v", sugs, err)
		}
	})
	t.Run("lookup_error", func(t *testing.T) {
		agg := &fakeAggregator{metrics: successMetrics(50, 0.95)}
		tr := NewSuccessTrigger(agg, &fakeSkillLookupReader{bodyErr: errors.New("db down")}, nil)
		sugs, err := tr.Check(context.Background(), "sk-1")
		if err != nil || len(sugs) != 0 {
			t.Fatalf("expected skip on lookup error, got sugs=%v err=%v", sugs, err)
		}
	})
}

// ── F3：trigger_source → SuggestType 映射 + Curator prompt 成功分支 ─────────

// success 来源映射为 success_pattern，其余仍按 fix_failure 模板。
func TestSuggestTypeForUnified_SuccessMapping(t *testing.T) {
	if got := suggestTypeForUnified(&UnifiedEvolutionSuggestion{TriggerSource: "success"}); got != EvoSuggestionSuccessPattern {
		t.Fatalf("got %v", got)
	}
	if got := suggestTypeForUnified(&UnifiedEvolutionSuggestion{TriggerSource: "health"}); got != EvoSuggestionFixFailure {
		t.Fatalf("got %v", got)
	}
}

// 成功分支：指令为「固化与强化」而非「修复」，禁止删除 helpful>0 规则。
func TestBuildEvolverUserPrompt_SuccessBranch(t *testing.T) {
	p := buildEvolverUserPrompt(SkillDraftInput{
		SkillID:       "sk-1",
		SuggestType:   EvoSuggestionSuccessPattern,
		TriggerReason: "30d 成功率 96.2%（50 次调用），沉淀正向模式",
	}, "# S\n")
	if !strings.Contains(p, "固化") || !strings.Contains(p, "helpful") {
		t.Fatalf("success branch prompt missing consolidation instructions:\n%s", p)
	}
	if strings.Contains(p, "近期失败轨迹") {
		t.Fatal("success branch should not include failure-trace framing")
	}
}

// delta system prompt：success 来源时追加 remove 仅允许 harmful>0 约束；
// 其余来源保持原 prompt。
func TestEvolverDeltaSystemPrompt_SuccessConstraint(t *testing.T) {
	base := evolverDeltaSystemPromptFor(EvoSuggestionFixFailure)
	if base != evolverDeltaSystemPrompt {
		t.Fatal("non-success source must keep the base delta prompt")
	}
	succ := evolverDeltaSystemPromptFor(EvoSuggestionSuccessPattern)
	if !strings.Contains(succ, evolverDeltaSystemPrompt) {
		t.Fatal("success prompt must extend the base delta prompt")
	}
	if !strings.Contains(succ, "harmful") || !strings.Contains(succ, "remove") {
		t.Fatalf("success prompt missing remove constraint:\n%s", succ)
	}
}
