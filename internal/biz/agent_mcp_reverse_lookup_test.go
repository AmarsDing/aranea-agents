package biz

import (
	"context"
	"sort"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// P0-3 反查守卫：MCP server DOWN→UP 恢复边沿时，AgentIDsReferencingMCPServer
// 决定哪些 agent 的构建缓存需要失效。引用语义必须与 FilterEffectiveMCPServers
// 严格对称（deny 优先；无 mcp: allow 条目 = 全部可见 server 均被引用）。

func TestAgentReferencesMCPServer(t *testing.T) {
	mcpOn := []EffectiveAgentTool{{ToolKey: "mcp_tool_set", Enabled: true}}
	brokerOn := []EffectiveAgentTool{{ToolKey: "mcp_broker", Enabled: true}}
	mcpOff := []EffectiveAgentTool{{ToolKey: "mcp_tool_set", Enabled: false}}

	cases := []struct {
		name string
		eff  AgentEffectiveTools
		key  string
		want bool
	}{
		{"allow 显式含 server → 引用",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp_tool_set", "mcp:db_server"}}, "db_server", true},
		{"无 mcp: allow 条目 → 全部可见 server 均引用",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp_tool_set"}}, "db_server", true},
		{"mcp_broker 启用同样视为引用通道",
			AgentEffectiveTools{ToolsEnabled: true, Items: brokerOn, Allow: []string{"mcp:db_server"}}, "db_server", true},
		{"allow 含其他 server → 不引用本 server",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp_tool_set", "mcp:other"}}, "db_server", false},
		{"deny 优先于空 allow",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp_tool_set"}, Deny: []string{"mcp:db_server"}}, "db_server", false},
		{"deny 优先于显式 allow",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp:db_server"}, Deny: []string{"mcp:db_server"}}, "db_server", false},
		{"mcp 工具未启用 → 不引用",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOff, Allow: []string{"mcp:db_server"}}, "db_server", false},
		{"ToolsEnabled 全局关 → 不引用",
			AgentEffectiveTools{ToolsEnabled: false, Items: mcpOn, Allow: []string{"mcp:db_server"}}, "db_server", false},
		{"大小写不敏感（与 FilterEffectiveMCPServers 一致）",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp:DB_Server"}}, "db_server", true},
		{"空 serverKey → false",
			AgentEffectiveTools{ToolsEnabled: true, Items: mcpOn, Allow: []string{"mcp_tool_set"}}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentReferencesMCPServer(tc.eff, tc.key); got != tc.want {
				t.Fatalf("agentReferencesMCPServer(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// mcpLookupAgentReader 分页返回全部 agent。
type mcpLookupAgentReader struct {
	AgentReader
	agents []Agent
}

func (r *mcpLookupAgentReader) GetAgentByID(_ context.Context, id string) (Agent, error) {
	for _, ag := range r.agents {
		if ag.ID == id {
			return ag, nil
		}
	}
	return Agent{}, ErrNotFound
}

func (r *mcpLookupAgentReader) SearchAgents(_ context.Context, q AgentListQuery) (AgentListResult, error) {
	if q.Offset >= len(r.agents) {
		return AgentListResult{Total: len(r.agents)}, nil
	}
	end := q.Offset + q.Limit
	if end > len(r.agents) || q.Limit <= 0 {
		end = len(r.agents)
	}
	return AgentListResult{Items: r.agents[q.Offset:end], Total: len(r.agents)}, nil
}

// mcpLookupSettingsRepo 按 agent 返回预设 settings。
type mcpLookupSettingsRepo struct {
	byAgent map[string]AgentRuntimeSettings
}

func (r *mcpLookupSettingsRepo) GetAgentRuntimeSettings(_ context.Context, id string) (AgentRuntimeSettings, error) {
	if s, ok := r.byAgent[id]; ok {
		return s, nil
	}
	return AgentRuntimeSettings{}, ErrNotFound
}

func (r *mcpLookupSettingsRepo) ListAgentRuntimeSettings(context.Context) (map[string]AgentRuntimeSettings, error) {
	return r.byAgent, nil
}

func (r *mcpLookupSettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return v, nil
}

// mcpLookupToolReader 提供含 opt-in mcp_tool_set 的目录。
type mcpLookupToolReader struct {
	ToolRegistryReader
}

func (r *mcpLookupToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	return ToolListResult{Items: []Tool{
		{Key: "mcp_tool_set", Enabled: false, Category: "integration", Source: "builtin"},
		{Key: "read_file", Enabled: true, Category: "filesystem", Source: "builtin"},
	}, Total: 2}, nil
}

func (r *mcpLookupToolReader) ListToolAgentOverridesByAgent(context.Context, string) ([]ToolAgentOverride, error) {
	return nil, nil
}

func TestAgentIDsReferencingMCPServer(t *testing.T) {
	mkSettings := func(id string, toolsEnabled bool, allow, deny string) AgentRuntimeSettings {
		return AgentRuntimeSettings{
			AgentID:        id,
			ToolsEnabled:   toolsEnabled,
			ToolsProfile:   "coding",
			ToolsAllowJSON: allow,
			ToolsDenyJSON:  deny,
		}
	}
	agents := []Agent{
		{ID: "a-explicit", AgentKey: "k1", Status: "active"},   // allow 显式含 mcp:db_server
		{ID: "a-open", AgentKey: "k2", Status: "active"},       // 无 mcp: allow → 全部 server
		{ID: "a-deny", AgentKey: "k3", Status: "active"},       // deny 优先
		{ID: "a-other", AgentKey: "k4", Status: "active"},      // allow 只含其他 server
		{ID: "a-nomcp", AgentKey: "k5", Status: "active"},      // mcp_tool_set 未启用
		{ID: "a-disabled", AgentKey: "k6", Status: "active"},   // ToolsEnabled=false
	}
	settings := &mcpLookupSettingsRepo{byAgent: map[string]AgentRuntimeSettings{
		"a-explicit": mkSettings("a-explicit", true, `["mcp_tool_set","mcp:db_server"]`, `[]`),
		"a-open":     mkSettings("a-open", true, `["mcp_tool_set"]`, `[]`),
		"a-deny":     mkSettings("a-deny", true, `["mcp_tool_set"]`, `["mcp:db_server"]`),
		"a-other":    mkSettings("a-other", true, `["mcp_tool_set","mcp:other"]`, `[]`),
		"a-nomcp":    mkSettings("a-nomcp", true, `["read_file"]`, `[]`),
		"a-disabled": mkSettings("a-disabled", false, `["mcp_tool_set","mcp:db_server"]`, `[]`),
	}}
	uc := NewAgentUsecase(AgentUsecaseDeps{
		Reader:   &mcpLookupAgentReader{agents: agents},
		Settings: settings,
		Tools:    &mcpLookupToolReader{},
		Lg:       loggateway.NewNoop(),
	})

	got, err := uc.AgentIDsReferencingMCPServer(context.Background(), "db_server")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	sort.Strings(got)
	want := []string{"a-explicit", "a-open"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}

	// 空 key / 无引用 server 快路径。
	if ids, err := uc.AgentIDsReferencingMCPServer(context.Background(), "  "); err != nil || len(ids) != 0 {
		t.Fatalf("blank key must yield empty, got %v, err=%v", ids, err)
	}
	if ids, err := uc.AgentIDsReferencingMCPServer(context.Background(), "unreferenced"); err != nil || len(ids) != 2 {
		// a-open / a-deny 均无 mcp: allow 条目 = 引用全部可见 server；
		// a-deny 的 deny 只点名 db_server，不挡 unreferenced。
		t.Fatalf("unreferenced server must match a-deny+a-open, got %v, err=%v", ids, err)
	}
}
