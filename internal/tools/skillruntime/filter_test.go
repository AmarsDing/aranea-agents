package skillruntime

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

type mockFilterResolver struct {
	candidates    []biz.SkillRuntimeCandidate
	candidatesErr error
	embScores     map[string]float64
	embErr        error
}

func (m *mockFilterResolver) ListEnabledPublishedSkillCandidates(_ context.Context) ([]biz.SkillRuntimeCandidate, error) {
	return m.candidates, m.candidatesErr
}

func (m *mockFilterResolver) ListEnabledPublishedSkillKeys(_ context.Context) ([]string, error) {
	out := make([]string, len(m.candidates))
	for i, c := range m.candidates {
		out[i] = c.Slug
	}
	return out, nil
}

func (m *mockFilterResolver) ScoreByEmbedding(_ context.Context, _ string, _ []biz.SkillRuntimeCandidate) (map[string]float64, error) {
	if m.embErr != nil {
		return nil, m.embErr
	}
	return m.embScores, nil
}

func TestFilterCache_StoreAndLoad(t *testing.T) {
	c := &filterCache{}
	val := map[string]bool{"skill-a": true, "skill-b": true}
	c.Store("key1", val)
	loaded, ok := c.Load("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if !loaded["skill-a"] || !loaded["skill-b"] {
		t.Error("loaded value does not match stored value")
	}
}

func TestFilterCache_LoadMiss(t *testing.T) {
	c := &filterCache{}
	_, ok := c.Load("nonexistent")
	if ok {
		t.Error("expected miss for nonexistent key")
	}
}

func TestFilterCache_StoreEvictsOldestWhenFull(t *testing.T) {
	c := &filterCache{}
	for i := 0; i < filterCacheMaxEntries; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		c.Store(key, map[string]bool{})
	}
	c.Store("overflow", map[string]bool{"overflow": true})
	_, _, evicts := c.Stats()
	if evicts < 1 {
		t.Errorf("expected at least 1 eviction, got %d", evicts)
	}
	_, ok := c.Load("overflow")
	if !ok {
		t.Error("overflow key should exist after eviction")
	}
}

func TestFilterCache_StoreOverwriteNoEviction(t *testing.T) {
	c := &filterCache{}
	c.Store("key1", map[string]bool{"a": true})
	c.Store("key1", map[string]bool{"b": true})
	_, _, evicts := c.Stats()
	if evicts != 0 {
		t.Errorf("overwriting existing key should not cause eviction, got %d", evicts)
	}
	loaded, ok := c.Load("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if !loaded["b"] {
		t.Error("expected updated value with b=true")
	}
	if loaded["a"] {
		t.Error("expected old value a to be replaced")
	}
}

func TestFilterCache_Stats(t *testing.T) {
	c := &filterCache{}
	c.Store("k1", map[string]bool{})
	c.Load("k1")
	c.Load("k1")
	c.Load("miss")
	hits, misses, evicts := c.Stats()
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
	if evicts != 0 {
		t.Errorf("evicts = %d, want 0", evicts)
	}
}

func TestAgentVisibilityFilter_Allow_SlugInList(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if !f.allow(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a to be allowed")
	}
}

func TestAgentVisibilityFilter_Allow_SlugNotInList(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if f.allow(context.Background(), trpcskill.Summary{Name: "skill-b"}) {
		t.Error("expected skill-b to be denied")
	}
}

func TestAgentVisibilityFilter_Allow_EmptyAllowedList(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if f.allow(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("expected any skill to be denied with empty allowed list")
	}
}

func TestAgentVisibilityFilter_Allow_ResolveError(t *testing.T) {
	resolver := &mockFilterResolver{
		candidatesErr: context.DeadlineExceeded,
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if f.allow(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("expected any skill to be denied when resolve fails (fail-closed)")
	}
}

func TestAgentVisibilityFilter_Allow_NilFilter(t *testing.T) {
	var f *AgentVisibilityFilter
	if !f.allow(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("nil filter should allow all")
	}
}

func TestAgentVisibilityFilter_Allow_NilSkillUC(t *testing.T) {
	f := &AgentVisibilityFilter{skillUC: nil, runtime: &mockRuntime{json: "{}"}, lg: loggateway.NewNoop()}
	if !f.allow(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("nil skillUC should allow all")
	}
}

func TestAgentVisibilityFilter_Allow_CaseInsensitive(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("Skill-A", "A", "desc a", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if !f.allow(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected case-insensitive match for skill-a")
	}
	if !f.allow(context.Background(), trpcskill.Summary{Name: "SKILL-A"}) {
		t.Error("expected case-insensitive match for SKILL-A")
	}
}

func TestAgentVisibilityFilter_Allow_TrimmedName(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	if !f.allow(context.Background(), trpcskill.Summary{Name: "  skill-a  "}) {
		t.Error("expected trimmed name to match")
	}
}

func TestAgentVisibilityFilter_AllowedSlugs_CacheHit(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	result1 := f.allowedSlugs(context.Background())
	if !result1["skill-a"] {
		t.Error("expected skill-a in first call result")
	}
	result2 := f.allowedSlugs(context.Background())
	if !result2["skill-a"] {
		t.Error("expected skill-a in cached result")
	}
	hits, _, _ := f.cache.Stats()
	if hits < 1 {
		t.Errorf("expected at least 1 cache hit, got %d", hits)
	}
}

func TestAgentVisibilityFilter_AllowedSlugs_CacheMissAndResolve(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("skill-a", "A", "desc a", nil, nil),
			makeCandidate("skill-b", "B", "desc b", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	result := f.allowedSlugs(context.Background())
	if len(result) != 2 {
		t.Errorf("expected 2 slugs, got %d", len(result))
	}
	if !result["skill-a"] || !result["skill-b"] {
		t.Error("expected both skill-a and skill-b in result")
	}
}

func TestAgentVisibilityFilter_AllowedSlugs_ResolveError(t *testing.T) {
	resolver := &mockFilterResolver{
		candidatesErr: context.DeadlineExceeded,
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	result := f.allowedSlugs(context.Background())
	if len(result) != 0 {
		t.Errorf("expected empty map on resolve error, got %d entries", len(result))
	}
}

func TestAgentVisibilityFilter_AllowedSlugs_SlugsLowercased(t *testing.T) {
	resolver := &mockFilterResolver{
		candidates: []biz.SkillRuntimeCandidate{
			makeCandidate("Skill-A", "A", "desc a", nil, nil),
			makeCandidate("SKILL-B", "B", "desc b", nil, nil),
		},
	}
	runtime := &mockRuntime{json: "{}"}
	f := &AgentVisibilityFilter{skillUC: resolver, runtime: runtime, lg: loggateway.NewNoop()}
	result := f.allowedSlugs(context.Background())
	if !result["skill-a"] {
		t.Error("expected skill-a (lowercased) in result")
	}
	if !result["skill-b"] {
		t.Error("expected skill-b (lowercased) in result")
	}
}
