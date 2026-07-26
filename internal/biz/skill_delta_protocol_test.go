package biz

import (
	"strings"
	"testing"
)

// ── ParseRuleBlocks / Render ─────────────────────────────────────────────────

func TestParseRuleBlocks_Basic(t *testing.T) {
	body := "# Skill\n\n说明文字。\n\n<!-- aranea:rule id=\"timeout-retry\" -->\n超时后先重试一次。\n<!-- /aranea:rule -->\n\n尾部说明。\n"
	doc := ParseRuleBlocks(body)

	rules := doc.Rules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].ID != "timeout-retry" {
		t.Fatalf("rule id = %q", rules[0].ID)
	}
	if rules[0].Content != "超时后先重试一次。" {
		t.Fatalf("rule content = %q", rules[0].Content)
	}
	if rules[0].Helpful != 0 || rules[0].Harmful != 0 {
		t.Fatalf("default counters must be 0, got helpful=%d harmful=%d", rules[0].Helpful, rules[0].Harmful)
	}
	// 非规则段保留
	rendered := doc.Render()
	if !strings.Contains(rendered, "# Skill") || !strings.Contains(rendered, "尾部说明。") {
		t.Fatalf("non-rule segments lost: %q", rendered)
	}
}

func TestParseRuleBlocks_Counters(t *testing.T) {
	body := `<!-- aranea:rule id="r1" helpful=3 harmful=1 -->
内容
<!-- /aranea:rule -->`
	doc := ParseRuleBlocks(body)
	r := doc.RuleByID("r1")
	if r == nil {
		t.Fatal("rule r1 not parsed")
	}
	if r.Helpful != 3 || r.Harmful != 1 {
		t.Fatalf("counters = helpful:%d harmful:%d", r.Helpful, r.Harmful)
	}
	// 计数器回写进标记
	rendered := doc.Render()
	if !strings.Contains(rendered, `helpful=3`) || !strings.Contains(rendered, `harmful=1`) {
		t.Fatalf("counters not rendered back: %q", rendered)
	}
}

func TestParseRuleBlocks_UnterminatedMarkerTreatedAsText(t *testing.T) {
	body := "前文\n<!-- aranea:rule id=\"orphan\" -->\n没有闭合标记的内容\n"
	doc := ParseRuleBlocks(body)
	if len(doc.Rules()) != 0 {
		t.Fatalf("unterminated marker must not produce a rule, got %d", len(doc.Rules()))
	}
	if !strings.Contains(doc.Render(), "orphan") {
		t.Fatal("unterminated marker line must be preserved as ordinary text")
	}
}

func TestRuleDocument_RenderRoundTrip(t *testing.T) {
	body := "# S\n\n<!-- aranea:rule id=\"a\" helpful=2 -->\n规则 A\n<!-- /aranea:rule -->\n\n中段。\n\n<!-- aranea:rule id=\"b\" harmful=1 -->\n规则 B\n<!-- /aranea:rule -->\n"
	doc1 := ParseRuleBlocks(body)
	rendered := doc1.Render()
	doc2 := ParseRuleBlocks(rendered)

	r1, r2 := doc1.Rules(), doc2.Rules()
	if len(r1) != len(r2) {
		t.Fatalf("round trip lost rules: %d → %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i].ID != r2[i].ID || r1[i].Content != r2[i].Content ||
			r1[i].Helpful != r2[i].Helpful || r1[i].Harmful != r2[i].Harmful {
			t.Fatalf("round trip mismatch at rule %d: %+v → %+v", i, r1[i], r2[i])
		}
	}
}

func TestHasRuleBlocks(t *testing.T) {
	if HasRuleBlocks("# 普通正文\n没有规则块。") {
		t.Fatal("false positive")
	}
	if !HasRuleBlocks("<!-- aranea:rule id=\"x\" -->\n内容\n<!-- /aranea:rule -->") {
		t.Fatal("false negative")
	}
}

// ── ParseDeltaOpsJSON ────────────────────────────────────────────────────────

func TestParseDeltaOpsJSON_Valid(t *testing.T) {
	ops, err := ParseDeltaOpsJSON(`[
		{"op":"modify","rule_id":"a","content":"新内容"},
		{"op":"add","rule_id":"b","content":"新增"},
		{"op":"remove","rule_id":"c"}
	]`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(ops) != 3 || ops[0].Op != DeltaOpModify || ops[2].Op != DeltaOpRemove {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestParseDeltaOpsJSON_ToleratesFence(t *testing.T) {
	ops, err := ParseDeltaOpsJSON("```json\n[{\"op\":\"remove\",\"rule_id\":\"a\"}]\n```")
	if err != nil || len(ops) != 1 {
		t.Fatalf("fenced parse failed: ops=%+v err=%v", ops, err)
	}
}

func TestParseDeltaOpsJSON_RejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"bad_json":        "not json",
		"empty_array":     "[]",
		"unknown_op":      `[{"op":"replace","rule_id":"a","content":"x"}]`,
		"missing_rule_id": `[{"op":"remove"}]`,
		"missing_content": `[{"op":"modify","rule_id":"a"}]`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDeltaOpsJSON(input); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

// ── ApplyDeltaOps ────────────────────────────────────────────────────────────

func TestApplyDeltaOps_HappyPath(t *testing.T) {
	body := "<!-- aranea:rule id=\"keep\" -->\n保留\n<!-- /aranea:rule -->\n" +
		"<!-- aranea:rule id=\"mod\" helpful=2 -->\n旧内容\n<!-- /aranea:rule -->\n" +
		"<!-- aranea:rule id=\"del\" -->\n待删\n<!-- /aranea:rule -->\n" +
		"<!-- aranea:rule id=\"mrg\" -->\n前半\n<!-- /aranea:rule -->"
	doc := ParseRuleBlocks(body)

	changed, err := ApplyDeltaOps(doc, []DeltaOp{
		{Op: DeltaOpModify, RuleID: "mod", Content: "新内容"},
		{Op: DeltaOpAdd, RuleID: "new-rule", Content: "新增规则"},
		{Op: DeltaOpMerge, RuleID: "mrg", Content: "后半"},
		{Op: DeltaOpRemove, RuleID: "del"},
	})
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if len(changed) != 4 {
		t.Fatalf("changed = %v", changed)
	}

	if got := doc.RuleByID("mod"); got.Content != "新内容" || got.Helpful != 2 {
		t.Fatalf("modify lost counter or content wrong: %+v", got)
	}
	if got := doc.RuleByID("new-rule"); got == nil || got.Content != "新增规则" {
		t.Fatalf("add failed: %+v", got)
	}
	if got := doc.RuleByID("mrg"); got.Content != "前半\n后半" {
		t.Fatalf("merge failed: %q", got.Content)
	}
	if doc.RuleByID("del") != nil {
		t.Fatal("remove failed")
	}
	if doc.RuleByID("keep").Content != "保留" {
		t.Fatal("untouched rule changed")
	}
}

func TestApplyDeltaOps_StrictRejection(t *testing.T) {
	mkDoc := func() *RuleDocument {
		return ParseRuleBlocks("<!-- aranea:rule id=\"a\" -->\n内容\n<!-- /aranea:rule -->")
	}
	cases := map[string]DeltaOp{
		"add_duplicate":    {Op: DeltaOpAdd, RuleID: "a", Content: "x"},
		"modify_unknown":   {Op: DeltaOpModify, RuleID: "ghost", Content: "x"},
		"merge_unknown":    {Op: DeltaOpMerge, RuleID: "ghost", Content: "x"},
		"remove_unknown":   {Op: DeltaOpRemove, RuleID: "ghost"},
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplyDeltaOps(mkDoc(), []DeltaOp{op}); err == nil {
				t.Fatalf("expected strict rejection for %s", name)
			}
		})
	}
}

// ── BumpRuleCounters ─────────────────────────────────────────────────────────

func TestBumpRuleCounters(t *testing.T) {
	body := "<!-- aranea:rule id=\"h\" helpful=1 -->\n甲\n<!-- /aranea:rule -->\n" +
		"<!-- aranea:rule id=\"b\" -->\n乙\n<!-- /aranea:rule -->"

	doc := ParseRuleBlocks(body)
	BumpRuleCounters(doc, []string{"h", "ghost"}, EvoEffectivenessHelpful)
	if got := doc.RuleByID("h"); got.Helpful != 2 {
		t.Fatalf("helpful not bumped: %+v", got)
	}

	doc2 := ParseRuleBlocks(body)
	BumpRuleCounters(doc2, []string{"b"}, EvoEffectivenessHarmful)
	if got := doc2.RuleByID("b"); got.Harmful != 1 {
		t.Fatalf("harmful not bumped: %+v", got)
	}

	doc3 := ParseRuleBlocks(body)
	BumpRuleCounters(doc3, []string{"h", "b"}, EvoEffectivenessNeutral)
	if got := doc3.RuleByID("h"); got.Helpful != 1 {
		t.Fatal("neutral verdict must not bump counters")
	}
}
