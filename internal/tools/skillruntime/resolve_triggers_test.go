package skillruntime

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func triggeredCandidate(slug string, triggers []string) biz.SkillRuntimeCandidate {
	c := makeCandidate(slug, slug, "desc for "+slug, nil, nil)
	c.Triggers = triggers
	return c
}

// P1-3：CJK trigger 使用子串语义（中文无词边界）。
func TestMatchTrigger_CJKSubstring(t *testing.T) {
	if got := matchTrigger("帮我处理报销流程", []string{"报销"}); got != "报销" {
		t.Fatalf("expected CJK trigger match, got %q", got)
	}
	if got := matchTrigger("帮我订机票", []string{"报销"}); got != "" {
		t.Fatalf("expected no match, got %q", got)
	}
}

// P1-3：ASCII trigger 必须词边界匹配，避免 "pdf" 误中 "pdftk"。
func TestMatchTrigger_ASCIIWordBoundary(t *testing.T) {
	if got := matchTrigger("generate pdf report", []string{"pdf"}); got != "pdf" {
		t.Fatalf("expected ascii trigger match, got %q", got)
	}
	if got := matchTrigger("run pdftk extract", []string{"pdf"}); got != "" {
		t.Fatalf("expected no match inside larger token, got %q", got)
	}
	if got := matchTrigger("Generate PDF report", []string{"pdf"}); got != "pdf" {
		t.Fatalf("expected case-insensitive match, got %q", got)
	}
}

// P1-3：trigger 命中必须绕过 intent 路径收窄（确定性 preload）。
func TestResolveSkillSlugsDetailed_TriggerBypassesIntentNarrowing(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			triggeredCandidate("expense-helper", []string{"报销"}),
			makeCandidate("read-xlsx", "Read XLSX", "reads xlsx spreadsheets", nil,
				[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":true,"intent_max_paths":3}`},
		UserQuery: "读取表格文件并处理报销",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range result.Slugs {
		if s == "expense-helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected triggered skill force-included past intent narrowing, got %v", result.Slugs)
	}
	if !strings.HasPrefix(result.Reasons["expense-helper"], "trigger match:") {
		t.Fatalf("expected trigger reason, got %q", result.Reasons["expense-helper"])
	}
}

// P1-3：trigger 命中必须绕过 tag 过滤。
func TestResolveSkillSlugsDetailed_TriggerBypassesTagFilter(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			triggeredCandidate("expense-helper", []string{"报销"}),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"allowed_tags":["file_type:xlsx"],"intent_routing_enabled":false}`},
		UserQuery: "处理报销",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 || result.Slugs[0] != "expense-helper" {
		t.Fatalf("expected triggered skill to bypass tag filter, got %v", result.Slugs)
	}
}

// P1-3：trigger 命中排序分必须高于 taxonomy 精确匹配（确定性优先）。
func TestResolveSkillSlugsDetailed_TriggerRanksAboveTaxonomy(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("read-xlsx", "Read XLSX", "reads xlsx spreadsheets", nil,
				[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
			triggeredCandidate("expense-helper", []string{"报销"}),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":false}`},
		UserQuery: "处理报销",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) == 0 || result.Slugs[0] != "expense-helper" {
		t.Fatalf("expected triggered skill ranked first, got %v", result.Slugs)
	}
}

// P1-3：trigger 命中占用 max_skills_in_toolset 配额。
func TestResolveSkillSlugsDetailed_TriggerConsumesCap(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			triggeredCandidate("expense-helper", []string{"报销"}),
			makeCandidate("skill-b", "B", "desc b", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"max_skills_in_toolset":1,"intent_routing_enabled":false}`},
		UserQuery: "处理报销",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 1 || result.Slugs[0] != "expense-helper" {
		t.Fatalf("expected only triggered skill within cap, got %v", result.Slugs)
	}
}

// P1-3：Layer A deny 优先于 trigger（策略红线不可被 trigger 绕过）。
func TestResolveSkillSlugsDetailed_LayerADenyBeatsTrigger(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			triggeredCandidate("expense-helper", []string{"报销"}),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"denied_slugs":["expense-helper"],"intent_routing_enabled":false}`},
		UserQuery: "处理报销",
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 0 {
		t.Fatalf("expected denied skill excluded despite trigger, got %v", result.Slugs)
	}
	if result.Reasons["expense-helper"] != "denied by policy" {
		t.Fatalf("expected deny reason, got %q", result.Reasons["expense-helper"])
	}
}
