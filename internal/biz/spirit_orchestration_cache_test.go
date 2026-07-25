package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// OrchestrationCache 领域配方（B.10.21.7 / B.10.21.9 biz cache 层）测试。
// ---------------------------------------------------------------------------

func newTestDomainCache() *OrchestrationCache {
	return NewOrchestrationCache(loggateway.NewNoop(), nil)
}

func TestRecordAndBestRecipeForDomain_ExactHit(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.85, 1, []string{"agent-a"})

	entry, ok := c.BestRecipeForDomain("创作/文学")
	if !ok {
		t.Fatal("expected hit for exact domain")
	}
	if entry.DomainPath != "创作/文学" || entry.DQScore != 0.85 {
		t.Errorf("entry = %+v", entry)
	}
	if len(entry.AgentKeys) != 1 || entry.AgentKeys[0] != "agent-a" {
		t.Errorf("AgentKeys = %v, want [agent-a]", entry.AgentKeys)
	}
}

func TestBestRecipeForDomain_PrefixMatchBothDirections(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作", TopologyDirect, 0.8, 1, []string{"agent-a"})

	// 查询更具体：配方域是查询域的前缀。
	if _, ok := c.BestRecipeForDomain("创作/文学"); !ok {
		t.Fatal("query 创作/文学 should hit recipe 创作（前缀匹配）")
	}
	// 查询更宽泛：查询域是配方域的前缀。
	c2 := newTestDomainCache()
	c2.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.8, 1, []string{"agent-a"})
	if _, ok := c2.BestRecipeForDomain("创作"); !ok {
		t.Fatal("query 创作 should hit recipe 创作/文学（反向前缀匹配）")
	}
	// 路径边界：不同一级域不命中。
	if _, ok := c.BestRecipeForDomain("软件/后端"); ok {
		t.Fatal("query 软件/后端 must not hit recipe 创作")
	}
}

func TestBestRecipeForDomain_PicksHighestDQ(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作", TopologyDirect, 0.75, 1, []string{"agent-low"})
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-high"})

	entry, ok := c.BestRecipeForDomain("创作/文学")
	if !ok {
		t.Fatal("expected hit")
	}
	if entry.AgentKeys[0] != "agent-high" {
		t.Errorf("winner = %q, want agent-high（DQ 0.9 > 0.75）", entry.AgentKeys[0])
	}
}

func TestBestRecipeForDomain_BelowDQThresholdSkips(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, DomainRecipeMinDQ-0.01, 1, []string{"agent-a"})
	if _, ok := c.BestRecipeForDomain("创作/文学"); ok {
		t.Fatalf("DQ < %.2f must not be returned", DomainRecipeMinDQ)
	}
}

func TestBestRecipeForDomain_EmptyAgentKeysSkips(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 0, nil)
	if _, ok := c.BestRecipeForDomain("创作/文学"); ok {
		t.Fatal("recipe with empty AgentKeys must not be returned")
	}
}

func TestRecordDomainRecipe_HigherDQReplacesOnly(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})
	// 更低 DQ 不覆盖。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.8, 1, []string{"agent-b"})
	entry, _ := c.BestRecipeForDomain("创作/文学")
	if entry.AgentKeys[0] != "agent-a" || entry.DQScore != 0.9 {
		t.Errorf("lower DQ must not replace: %+v", entry)
	}
	// 更高 DQ 覆盖。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.95, 1, []string{"agent-c"})
	entry, _ = c.BestRecipeForDomain("创作/文学")
	if entry.AgentKeys[0] != "agent-c" || entry.DQScore != 0.95 {
		t.Errorf("higher DQ must replace: %+v", entry)
	}
}

func TestRecordDomainRecipe_EmptyDomainNoop(t *testing.T) {
	c := newTestDomainCache()
	c.RecordDomainRecipe(context.Background(), "", TopologyDirect, 0.9, 1, []string{"agent-a"})
	if _, ok := c.BestRecipeForDomain("创作/文学"); ok {
		t.Fatal("empty domainPath record must be a no-op")
	}
}

func TestOrchestrationCache_LegacyJSONWithoutDomainPath_Loads(t *testing.T) {
	// 不变量 4：旧 JSON（无 domain_path 字段）加载不报错，且不参与领域查询。
	legacy := `[{"task_pattern":"old-key","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-a"],"updated_at":"2099-01-01T00:00:00Z"}]`
	c := newTestDomainCache()
	if err := c.LoadFromJSON(legacy); err != nil {
		t.Fatalf("LoadFromJSON legacy: %v", err)
	}
	if _, ok := c.BestRecipeForDomain("创作/文学"); ok {
		t.Fatal("legacy entry without domain_path must not match domain query")
	}
}
