package configgraph

import (
	"errors"

	"aranea-agents/internal/biz"
)

// fullFixture 造一套覆盖 27 类边 + 关键 broken 场景的源数据（design 0.4/0.5
// 验收：ORPHAN position_key broken、linked_graph JSON 兜底、routing 双解、
// cron team 目标）。
func fullFixture() (*fakeSource, fakeProvider) {
	src := &fakeSource{
		agents: []AgentRow{
			{ID: "uuid-a1", AgentKey: "ops_master", DisplayName: "Ops", Status: "active",
				Kind: "normal", PositionID: "uuid-org-dept", PositionKey: "ORPHAN-KEY", WorkspaceID: "ws1"},
			{ID: "uuid-a2", AgentKey: "backup", DisplayName: "Backup", Status: "active", WorkspaceID: "ws1"},
			{ID: "uuid-a3", AgentKey: "critic", DisplayName: "Critic", Status: "active", WorkspaceID: "ws1"},
			// provider 失败 → broken granted_tool 标记边（不被其他边引用）。
			{ID: "uuid-a4", AgentKey: "broken_eff", DisplayName: "Broken", Status: "active", WorkspaceID: "ws1"},
			{ID: "uuid-aDel", AgentKey: "dead_agent", Status: "active", DeletedAt: "2026-01-01T00:00:00Z"},
		},
		orgs: []OrganizationRow{
			{ID: "uuid-org-root", OrgKey: "root", Name: "Root"},
			{ID: "uuid-org-dept", OrgKey: "dept", Name: "Dept", ParentID: "uuid-org-root", DeptLeadAgentID: "uuid-a1"},
		},
		tools: []ToolRow{
			{ID: "uuid-t1", ToolKey: "shell_exec", DisplayName: "Shell", Category: "runtime", RiskLevel: "high"},
			{ID: "uuid-t2", ToolKey: "web_fetch", DisplayName: "WebFetch", Category: "web"},
			{ID: "uuid-t3", ToolKey: "mcp_tool_set", DisplayName: "MCP", Category: "integration"},
		},
		skills: []SkillRow{
			{ID: "uuid-s1", SkillKey: "skill-a", Name: "A", ParentID: "uuid-s2", AgentID: "uuid-a1"},
			{ID: "uuid-s2", SkillKey: "skill-b", Name: "B"},
		},
		mcps: []MCPServerRow{
			{ID: "uuid-m1", ServerKey: "github", Name: "GitHub"},
		},
		kcs: []KnowledgeCollectionRow{
			{ID: "uuid-kc1", Name: "kb-main", Workspace: "ws1"},
		},
		graphs: []GraphRow{
			{ID: "uuid-g1", Name: "g-main", TeamID: "uuid-team1", WorkspaceID: "ws1",
				NodesJSON: `[{"id":"n1","agent_name":"ops_master","fallback_agent":"backup","reviewer_agent":"critic","tool_names":["shell_exec"]}]`},
			{ID: "uuid-g2", Name: "g-tmpl", IsTemplate: true},
		},
		teams: []TeamRow{
			{ID: "uuid-team1", TeamKey: "team-main", DisplayName: "Main", DepartmentID: "uuid-org-dept",
				DeptLeadAgentID: "uuid-a1", CrossDeptMemberIDs: `["uuid-a2"]`, LinkedGraphID: "uuid-g1",
				WorkspaceID: "ws1",
				DefinitionJSON: `{
					"version":1,"mode":"sequential",
					"synthesizer_agent_id":"uuid-a1",
					"intent_anchor_agent_id":"uuid-a2",
					"members":[
						{"agent_id":"uuid-a1","role":"leader","sort_order":1},
						{"agent_id":"uuid-a2","role":"worker","sort_order":2,"enabled":false}
					],
					"failure_policy":{"node_overrides":{"member-1":{"policy":"fallback","fallback_agent":"uuid-a3"}}},
					"graph_template_id":"uuid-g2",
					"collection_ids":["uuid-kc1"]
				}`},
			{ID: "uuid-team2", TeamKey: "team-legacy", DisplayName: "Legacy",
				DefinitionJSON: `{"version":1,"mode":"sequential","linked_graph_id":"uuid-g2","members":[]}`},
		},
		crons: []CronTaskRow{
			{ID: "uuid-c1", TaskKey: "agent-job", Name: "AgentJob", AgentID: "uuid-a1", ConfigJSON: `{"target_type":"agent"}`},
			{ID: "uuid-c2", TaskKey: "team-job", Name: "TeamJob", ConfigJSON: `{"target_type":"team","team_id":"uuid-team1"}`},
			{ID: "uuid-c3", TaskKey: "sync-job", Name: "Sync", ConfigJSON: `{"target_type":"model_registry_sync"}`},
		},
		channels: []ChannelRow{
			{ID: "uuid-ch1", ChannelKey: "feishu", Name: "Feishu", WorkspaceID: "ws1",
				ConfigJSON: `{"routing":{"default_agent_id":"ops_master","default_team_id":"uuid-team2","rules":[` +
					`{"peer_pattern":"u_*","agent_id":"uuid-a2"},` +
					`{"peer_pattern":"g_*","team_id":"uuid-team1"}]}}`},
		},
		hooks: []HookRow{
			{ID: "uuid-h1", HookKey: "guard", Name: "Guard",
				ConfigJSON: `{"callback_point":"before_tool","condition":{"agent_id":"ops_master","tool_name":"shell_exec"}}`},
		},
		prompts: []PromptFileRow{
			{ID: "uuid-p1", AgentID: "uuid-a1", AgentKey: "ops_master", FileName: "SOUL.md", Body: "hello", SortOrder: 1},
		},
		policies: []AgentToolPolicyRow{
			{AgentID: "uuid-a1", SkillRuntimeJSON: `{"allowed_slugs":["skill-b"]}`},
		},
		overrides: []ToolOverrideRow{
			{ID: "ov1", AgentID: "uuid-a1", ToolID: "uuid-t2", ToolKey: "web_fetch", Mode: "allow"},
		},
	}
	prov := fakeProvider{
		eff: map[string]biz.AgentEffectiveTools{
			"uuid-a1": {ToolsEnabled: true,
				Allow: []string{"mcp:github"},
				Items: []biz.EffectiveAgentTool{
					{ToolKey: "shell_exec", Enabled: true, EffectiveState: "allowed", Origin: "profile"},
					{ToolKey: "web_fetch", Enabled: true, EffectiveState: "allowed", Origin: "override"},
					{ToolKey: "mcp_tool_set", Enabled: true, EffectiveState: "allowed", Origin: "allow"},
				}},
			"uuid-a2": {ToolsEnabled: true, Items: []biz.EffectiveAgentTool{
				{ToolKey: "shell_exec", Enabled: true, EffectiveState: "allowed", Origin: "allow"},
			}},
			"uuid-a3": {ToolsEnabled: true},
		},
		err: map[string]error{
			"uuid-a4": errors.New("boom"),
		},
	}
	return src, prov
}
