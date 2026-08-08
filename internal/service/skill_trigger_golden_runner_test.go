package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"
)

// ── P2 F4：触发率黄金集回归 runner ──────────────────────────────────────────
// 设计：docs/development/phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md §6
// 数据集寻址约定：{skill.Name|Slug}__trigger；Case.ExpectedOutput ∈
// {"trigger","no_trigger"}（大小写不敏感），其它值非法跳过。

func newTestGoldenRunner(eval evalDatasetReader, skills biz.SkillLookupReader) *SkillTriggerGoldenRunner {
	return NewSkillTriggerGoldenRunner(eval, skills, loggateway.NewNoop())
}

// goldenSkillBody renders a SKILL.md body with frontmatter triggers.
func goldenSkillBody(triggers ...string) string {
	body := "---\nname: test-skill\ntriggers: ["
	for i, tr := range triggers {
		if i > 0 {
			body += ", "
		}
		body += tr
	}
	body += "]\n---\n# Test Skill\n\n## 用法\n\n做测试。\n"
	return body
}

// 数据集寻址：{Name|Slug}__trigger 未命中 → ErrNoReplayDataset（Gate 跳过）。
func TestSkillTriggerGoldenRunner_NoBoundDataset(t *testing.T) {
	eval := &fakeEvalDatasetReader{datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research", Slug: "web-research"}}
	r := newTestGoldenRunner(eval, skills)

	_, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if !errors.Is(err, biz.ErrNoReplayDataset) {
		t.Fatalf("expected ErrNoReplayDataset, got %v", err)
	}
}

// Slug 命中 + 大小写不敏感。
func TestSkillTriggerGoldenRunner_SlugDatasetHits(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "Web-Research__TRIGGER"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "convert this pdf file", ExpectedOutput: "trigger"},
		},
	}
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "Web Research", Slug: "web-research"},
		markdown: goldenSkillBody("pdf"),
	}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.DatasetName != "Web-Research__TRIGGER" || res.Total != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// 空用例集 → ErrNoReplayDataset。
func TestSkillTriggerGoldenRunner_EmptyCases(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
	}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestGoldenRunner(eval, skills)

	_, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if !errors.Is(err, biz.ErrNoReplayDataset) {
		t.Fatalf("expected ErrNoReplayDataset, got %v", err)
	}
}

// trigger/no_trigger 判定正确性：CJK 子串 + ASCII 词边界语义与 matchTrigger
// 一致；draft 与 baseline 双端各自计分。
func TestSkillTriggerGoldenRunner_Scoring(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "convert this PDF file", ExpectedOutput: "trigger"}, // ASCII 词边界命中
			{ID: "c2", Input: "pdftk usage guide", ExpectedOutput: "no_trigger"},  // pdf 不词中 pdftk
			{ID: "c3", Input: "帮我开发票", ExpectedOutput: "TRIGGER"},                 // CJK 子串命中（大小写不敏感期望值）
			{ID: "c4", Input: "今天天气怎么样", ExpectedOutput: "no_trigger"},            // 不命中
			{ID: "c5", Input: "bogus case", ExpectedOutput: "maybe"},              // 非法期望值跳过
		},
	}
	// draft: triggers [pdf, 发票] → 4/4 全对；baseline: triggers [pdf] → c3 漏触发 3/4。
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "web-research"},
		markdown: goldenSkillBody("pdf"),
	}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf", "发票"), 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.Total != 4 { // c5 非法跳过
		t.Fatalf("expected 4 valid cases, got %d", res.Total)
	}
	if res.Accuracy != 1.0 {
		t.Fatalf("expected draft accuracy 1.0, got %v (FalseNeg=%d FalsePos=%d)", res.Accuracy, res.FalseNeg, res.FalsePos)
	}
	if !res.HasBaseline {
		t.Fatal("expected baseline available")
	}
	if res.BaselineAccuracy != 0.75 {
		t.Fatalf("expected baseline accuracy 0.75, got %v", res.BaselineAccuracy)
	}
}

// FalseNeg/FalsePos 计数：should-trigger 未命中计 FalseNeg，should-not-trigger
// 误中计 FalsePos（draft 端）。
func TestSkillTriggerGoldenRunner_FalseCounts(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "我想报销发票", ExpectedOutput: "trigger"},           // draft 无发票 trigger → FalseNeg
			{ID: "c2", Input: "this pdf file", ExpectedOutput: "no_trigger"}, // draft 有 pdf → FalsePos
		},
	}
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "web-research"},
		markdown: goldenSkillBody("发票"),
	}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.FalseNeg != 1 || res.FalsePos != 1 {
		t.Fatalf("expected FalseNeg=1 FalsePos=1, got %+v", res)
	}
	if res.Accuracy != 0 {
		t.Fatalf("expected accuracy 0, got %v", res.Accuracy)
	}
}

// draft 无可解析 triggers：should-trigger 用例全部 FalseNeg（如实反映）。
func TestSkillTriggerGoldenRunner_DraftNoTriggers_AllFalseNeg(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "convert this pdf", ExpectedOutput: "trigger"},
			{ID: "c2", Input: "another pdf", ExpectedOutput: "trigger"},
		},
	}
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "web-research"},
		markdown: goldenSkillBody("pdf"),
	}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", "# no frontmatter at all", 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.FalseNeg != 2 || res.Accuracy != 0 {
		t.Fatalf("expected all FalseNeg, got %+v", res)
	}
}

// 全部用例非法 → Total=0 结果（Gate 跳过），不报错。
func TestSkillTriggerGoldenRunner_AllInvalid_ZeroTotal(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "yes"},
			{ID: "c2", Input: "q2", ExpectedOutput: ""},
		},
	}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res == nil || res.Total != 0 {
		t.Fatalf("expected Total=0 result, got %+v", res)
	}
}

// 当前正文不可得 → HasBaseline=false，draft 端正常（Gate 仅绝对下限）。
func TestSkillTriggerGoldenRunner_BaselineUnavailable(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases:    []evaluation.Case{{ID: "c1", Input: "this pdf", ExpectedOutput: "trigger"}},
	}
	skills := &fakeReplaySkillLookup{
		skill: biz.Skill{ID: "s1", Name: "web-research"},
		mdErr: errors.New("no current version"),
	}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 0)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.HasBaseline {
		t.Fatalf("expected HasBaseline=false, got %+v", res)
	}
	if res.Accuracy != 1.0 {
		t.Fatalf("expected draft accuracy 1.0, got %v", res.Accuracy)
	}
}

// maxCases 上限截断（在非法用例过滤前生效，与回放语义一致）。
func TestSkillTriggerGoldenRunner_MaxCasesCaps(t *testing.T) {
	cases := make([]evaluation.Case, 4)
	for i := range cases {
		cases[i] = evaluation.Case{ID: "c", Input: "this pdf", ExpectedOutput: "trigger"}
	}
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research__trigger"}},
		cases:    cases,
	}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestGoldenRunner(eval, skills)

	res, err := r.RunTriggerGolden(context.Background(), "s1", goldenSkillBody("pdf"), 2)
	if err != nil {
		t.Fatalf("RunTriggerGolden: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2 cases after cap, got %d", res.Total)
	}
}
