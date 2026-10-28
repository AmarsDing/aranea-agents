package skillruntime

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/decide"
	"aranea-agents/pkg/loggateway"
)

type captureCollector struct{ recs []decision.Record }

func (c *captureCollector) Emit(_ context.Context, r decision.Record) { c.recs = append(c.recs, r) }

func TestResolveSkillSlugsDetailed_ShadowEmitsSkillIntent(t *testing.T) {
	cap := &captureCollector{}
	rec := decide.NewRecorder(cap, loggateway.NewNoop())
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("read-xlsx", "Read XLSX", "reads xlsx spreadsheets", nil,
				[]string{"数据获取与集成/内部数据源/文件系统读取（读取表格）"}),
			makeCandidate("other", "Other", "unrelated", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{
		Runtime:   &mockRuntime{json: `{"intent_routing_enabled":true,"intent_max_paths":3,"max_skills_in_toolset":2}`},
		UserQuery: "帮我读一下表格文件",
		Shadow:    rec,
	}
	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Slugs) == 0 {
		t.Fatal("expected at least one routed slug for fixture query")
	}
	if len(cap.recs) != 1 {
		t.Fatalf("want 1 shadow record, got %d", len(cap.recs))
	}
	got := cap.recs[0]
	if got.Category != decision.CategoryJEVShadow {
		t.Fatalf("category=%q", got.Category)
	}
	if got.Outcome != "q=skill_intent;k="+result.Slugs[0] {
		t.Fatalf("outcome=%q want top slug %q", got.Outcome, result.Slugs[0])
	}
}

func TestResolveSkillSlugsDetailed_NilShadowNoEmit(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("a", "A", "a", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{UserQuery: "anything"}
	if _, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil); err != nil {
		t.Fatal(err)
	}
}
