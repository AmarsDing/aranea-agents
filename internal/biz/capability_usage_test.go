package biz

import (
	"context"
	"testing"
)

// usageToolReader 按 key 返回工具与覆盖（bindingsToolReader 不区分 key，对照不了多个工具）。
type usageToolReader struct {
	ToolRegistryReader
	catalog   []Tool
	overrides []ToolAgentOverride
}

func (r *usageToolReader) GetTool(_ context.Context, key string) (Tool, error) {
	for _, t := range r.catalog {
		if t.Key == key {
			return t, nil
		}
	}
	return Tool{}, ErrNotFound
}

func (r *usageToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	return ToolListResult{Items: r.catalog, Total: len(r.catalog)}, nil
}

func (r *usageToolReader) ListToolAgentOverrides(_ context.Context, key string) ([]ToolAgentOverride, error) {
	var out []ToolAgentOverride
	for _, o := range r.overrides {
		if o.ToolKey == key {
			out = append(out, o)
		}
	}
	return out, nil
}

// 批量计数必须与逐个工具的 GetToolAgentBindings 得出同样的「可用 Agent 数」。
func TestCountAgentsAllowedByTool_matchesBindings(t *testing.T) {
	catalog := []Tool{
		{Key: "read_file", Enabled: true, Category: "filesystem", Source: "builtin"},
		{Key: "shell_exec", Enabled: true, Category: "system", Source: "builtin", AgentOverrideCount: 1},
		{Key: "web_fetch", Enabled: false, Category: "web", Source: "builtin"},
	}
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{
		"a-coding": {AgentID: "a-coding", ToolsEnabled: true, ToolsProfile: "coding"},
		"a-off":    {AgentID: "a-off", ToolsEnabled: false, ToolsProfile: "coding"},
		"a-deny":   {AgentID: "a-deny", ToolsEnabled: true, ToolsProfile: "coding", ToolsDenyJSON: `["read_file"]`},
		// a-missing 没有设置行，走默认
	}}
	agents := []Agent{{ID: "a-coding"}, {ID: "a-off"}, {ID: "a-deny"}, {ID: "a-missing"}}
	tools := &usageToolReader{
		catalog:   catalog,
		overrides: []ToolAgentOverride{{ToolKey: "shell_exec", AgentID: "a-coding", Mode: "deny"}},
	}
	uc := newBindingsUsecase(&bindingsAgentReader{agents: agents}, settings, tools)

	counts, err := uc.CountAgentsAllowedByTool(context.Background(), []string{"read_file", "shell_exec", "web_fetch", "missing"}, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	for _, tool := range catalog {
		bindings, err := uc.GetToolAgentBindings(context.Background(), tool.Key, "")
		if err != nil {
			t.Fatalf("bindings %s: %v", tool.Key, err)
		}
		want := 0
		for _, b := range bindings {
			if b.State == "allowed" {
				want++
			}
		}
		if counts[tool.Key] != want {
			t.Errorf("%s: batch %d, bindings %d", tool.Key, counts[tool.Key], want)
		}
	}
	if counts["missing"] != 0 {
		t.Errorf("unknown key must count 0, got %d", counts["missing"])
	}
	if counts["read_file"] == 0 {
		t.Errorf("read_file should be allowed for at least the coding agent: %v", counts)
	}
}

// 批量计数必须与逐个服务器的 AgentIDsReferencingMCPServer 一致（沿用它的测试数据）。
func TestCountAgentsByMCPServer_matchesReverseLookup(t *testing.T) {
	mk := func(id string, toolsEnabled bool, allow, deny string) AgentRuntimeSettings {
		return AgentRuntimeSettings{AgentID: id, ToolsEnabled: toolsEnabled, ToolsProfile: "coding", ToolsAllowJSON: allow, ToolsDenyJSON: deny}
	}
	agents := []Agent{{ID: "a-explicit"}, {ID: "a-open"}, {ID: "a-deny"}, {ID: "a-other"}, {ID: "a-nomcp"}, {ID: "a-disabled"}}
	uc := NewAgentUsecase(AgentUsecaseDeps{
		Reader: &mcpLookupAgentReader{agents: agents},
		Settings: &mcpLookupSettingsRepo{byAgent: map[string]AgentRuntimeSettings{
			"a-explicit": mk("a-explicit", true, `["mcp_tool_set","mcp:db_server"]`, `[]`),
			"a-open":     mk("a-open", true, `["mcp_tool_set"]`, `[]`),
			"a-deny":     mk("a-deny", true, `["mcp_tool_set"]`, `["mcp:db_server"]`),
			"a-other":    mk("a-other", true, `["mcp_tool_set","mcp:other"]`, `[]`),
			"a-nomcp":    mk("a-nomcp", true, `["read_file"]`, `[]`),
			"a-disabled": mk("a-disabled", false, `["mcp_tool_set","mcp:db_server"]`, `[]`),
		}},
		Tools: &mcpLookupToolReader{},
	})
	keys := []string{"db_server", "unreferenced", "other"}
	counts, err := uc.CountAgentsByMCPServer(context.Background(), keys, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	for _, k := range keys {
		ids, err := uc.AgentIDsReferencingMCPServer(context.Background(), k)
		if err != nil {
			t.Fatalf("lookup %s: %v", k, err)
		}
		if counts[k] != len(ids) {
			t.Errorf("%s: batch %d, reverse lookup %d (%v)", k, counts[k], len(ids), ids)
		}
	}
}

// 技能可见性与运行时 skillruntime.NewAgentVisibilityFilter 同一规则：拒绝优先，有白名单只看白名单。
func TestCountAgentsSeeingSkill(t *testing.T) {
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{
		"a-allow": {AgentID: "a-allow", SkillRuntimeJSON: `{"allowed_slugs":["alpha"]}`},
		"a-deny":  {AgentID: "a-deny", SkillRuntimeJSON: `{"denied_slugs":["alpha"]}`},
		"a-open":  {AgentID: "a-open"},
	}}
	uc := newBindingsUsecase(&bindingsAgentReader{agents: []Agent{{ID: "a-allow"}, {ID: "a-deny"}, {ID: "a-open"}}}, settings, &usageToolReader{})
	counts, err := uc.CountAgentsSeeingSkill(context.Background(), []string{"alpha", "beta"}, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if counts["alpha"] != 2 || counts["beta"] != 2 {
		t.Fatalf("want alpha=2 (allow+open) beta=2 (deny+open), got %v", counts)
	}
}
