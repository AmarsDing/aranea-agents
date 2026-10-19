package skillruntime

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"
)

// TestBuildRankCandidates_UsesSkillIDWithSlugFallback 直接单测 B1 修复点：
// candidate 带 SkillID 时按 ID 查询健康指标；不带（存量调用方）时回退 slug。
func TestBuildRankCandidates_UsesSkillIDWithSlugFallback(t *testing.T) {
	scored := []slugScore{
		{slug: "with-id", skillID: "skill_111"},
		{slug: "legacy-slug"},
	}
	provider := &fakeHealthProvider{rates: map[string]float64{
		"skill_111":   0.9,
		"legacy-slug": 0.5,
	}}
	out := buildRankCandidates(context.Background(), scored, provider, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(out))
	}
	if out[0].HistoricalSuccess != 0.9 {
		t.Errorf("with-id HistoricalSuccess = %v, want 0.9 (queried by skill ID)", out[0].HistoricalSuccess)
	}
	if out[1].HistoricalSuccess != 0.5 {
		t.Errorf("legacy-slug HistoricalSuccess = %v, want 0.5 (slug fallback)", out[1].HistoricalSuccess)
	}
	if len(provider.keys) != 2 || provider.keys[0] != "skill_111" || provider.keys[1] != "legacy-slug" {
		t.Errorf("query keys = %v, want [skill_111 legacy-slug]", provider.keys)
	}
}

// TestResolveSkillSlugsDetailed_NilHealthProviderSkipsFusion pins the
// pre-R1 behavior: without a provider the lexicographic tie-break decides.
func TestResolveSkillSlugsDetailed_NilHealthProviderSkipsFusion(t *testing.T) {
	resolver := &mockSkillResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("aaa-skill", "AAA", "desc", nil, nil),
			makeCandidate("zzz-skill", "ZZZ", "desc", nil, nil),
		},
	}
	opts := &SkillToolsetOptions{Runtime: &mockRuntime{json: `{}`}}

	result, err := ResolveSkillSlugsDetailed(context.Background(), resolver, opts, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Slugs) != 2 || result.Slugs[0] != "aaa-skill" {
		t.Errorf("expected lexicographic order [aaa-skill zzz-skill], got %v", result.Slugs)
	}
}
