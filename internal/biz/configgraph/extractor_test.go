package configgraph

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeSource struct {
	agents    []AgentRow
	teams     []TeamRow
	skills    []SkillRow
	tools     []ToolRow
	prompts   []PromptFileRow
	crons     []CronTaskRow
	channels  []ChannelRow
	orgs      []OrganizationRow
	graphs    []GraphRow
	kcs       []KnowledgeCollectionRow
	mcps      []MCPServerRow
	hooks     []HookRow
	overrides []ToolOverrideRow
	policies  []AgentToolPolicyRow
}

func (f *fakeSource) ListAgents(context.Context) ([]AgentRow, error) { return f.agents, nil }
func (f *fakeSource) ListTeams(context.Context) ([]TeamRow, error)   { return f.teams, nil }
func (f *fakeSource) ListSkills(context.Context) ([]SkillRow, error) { return f.skills, nil }
func (f *fakeSource) ListTools(context.Context) ([]ToolRow, error)   { return f.tools, nil }
func (f *fakeSource) ListPromptFiles(context.Context) ([]PromptFileRow, error) {
	return f.prompts, nil
}
func (f *fakeSource) ListCronTasks(context.Context) ([]CronTaskRow, error) { return f.crons, nil }
func (f *fakeSource) ListChannels(context.Context) ([]ChannelRow, error)   { return f.channels, nil }
func (f *fakeSource) ListOrganizations(context.Context) ([]OrganizationRow, error) {
	return f.orgs, nil
}
func (f *fakeSource) ListGraphs(context.Context) ([]GraphRow, error) { return f.graphs, nil }
func (f *fakeSource) ListKnowledgeCollections(context.Context) ([]KnowledgeCollectionRow, error) {
	return f.kcs, nil
}
func (f *fakeSource) ListMCPServers(context.Context) ([]MCPServerRow, error) { return f.mcps, nil }
func (f *fakeSource) ListHooks(context.Context) ([]HookRow, error)           { return f.hooks, nil }
func (f *fakeSource) ListToolOverrides(context.Context) ([]ToolOverrideRow, error) {
	return f.overrides, nil
}
func (f *fakeSource) ListAgentToolPolicies(context.Context) ([]AgentToolPolicyRow, error) {
	return f.policies, nil
}

type fakeProvider struct {
	eff map[string]biz.AgentEffectiveTools
	err map[string]error
}

func (p fakeProvider) GetEffectiveTools(_ context.Context, agentID string) (biz.AgentEffectiveTools, error) {
	if e := p.err[agentID]; e != nil {
		return biz.AgentEffectiveTools{}, e
	}
	return p.eff[agentID], nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func extractEdges(t *testing.T, x Extractor, src SourceRepo) []Edge {
	t.Helper()
	edges, err := x.ExtractEdges(context.Background(), src)
	if err != nil {
		t.Fatalf("%T.ExtractEdges: %v", x, err)
	}
	return edges
}

func extractNodes(t *testing.T, x Extractor, src SourceRepo) []Node {
	t.Helper()
	nodes, err := x.ExtractNodes(context.Background(), src)
	if err != nil {
		t.Fatalf("%T.ExtractNodes: %v", x, err)
	}
	return nodes
}

// findEdge locates one extracted edge by (type, srcRef, dstType, dstRef|dstKey).
func findEdge(edges []Edge, typ, srcRef, dstType, dstRefOrKey string) *Edge {
	for i := range edges {
		e := &edges[i]
		if e.Type != typ || e.SrcRef != srcRef || e.DstType != dstType {
			continue
		}
		if e.DstRef == dstRefOrKey || e.DstKey == dstRefOrKey {
			return e
		}
	}
	return nil
}

// countType counts structural edges of typ, excluding extract_error markers
// (error edges are failure markers, not graph edges — asserted separately via
// hasExtractError).
func countType(edges []Edge, typ string) int {
	n := 0
	for _, e := range edges {
		if e.Type != typ {
			continue
		}
		if msg, _ := e.Evidence["extract_error"].(string); msg != "" {
			continue
		}
		n++
	}
	return n
}

func hasExtractError(edges []Edge, typ, srcRef string) bool {
	for _, e := range edges {
		if e.Type == typ && e.SrcRef == srcRef {
			if msg, _ := e.Evidence["extract_error"].(string); msg != "" {
				return true
			}
		}
	}
	return false
}

// ── registry ─────────────────────────────────────────────────────────────────

func TestExtractors_RegistryOrder(t *testing.T) {
	got := Extractors(nil)
	if len(got) != 12 {
		t.Fatalf("want 12 extractors, got %d", len(got))
	}
	want := []string{
		NodeTypeTool, NodeTypeSkill, NodeTypeOrganization, NodeTypeKnowledgeCollection,
		NodeTypeMCPServer, NodeTypeGraph, NodeTypeHook, NodeTypePromptFile,
		NodeTypeAgent, NodeTypeTeam, NodeTypeCronTask, NodeTypeChannel,
	}
	for i, x := range got {
		if x.NodeType() != want[i] {
			t.Fatalf("extractor[%d] = %q, want %q (target-first order)", i, x.NodeType(), want[i])
		}
	}
}

// ── 0.4 extractors ───────────────────────────────────────────────────────────

func TestExtractor_Tool(t *testing.T) {
	src := &fakeSource{tools: []ToolRow{
		{ID: "uuid-t1", ToolKey: "shell_exec", DisplayName: "Shell", Category: "runtime", RiskLevel: "high", RequiresConfirmation: true},
		{ID: "uuid-t2", ToolKey: "old_tool", DeletedAt: "2026-01-01T00:00:00Z"},
		{ID: "", ToolKey: "skipped"},
	}}
	nodes := extractNodes(t, toolExtractor{}, src)
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	n := nodes[0]
	if n.ID != NodeID(NodeTypeTool, "uuid-t1") || n.NodeKey != "shell_exec" || n.Status != NodeStatusActive {
		t.Fatalf("unexpected node: %+v", n)
	}
	if n.Attrs["risk_level"] != "high" || n.Attrs["category"] != "runtime" || n.Attrs["requires_confirmation"] != true {
		t.Fatalf("attrs mismatch: %+v", n.Attrs)
	}
	if nodes[1].Status != NodeStatusDeleted {
		t.Fatalf("deleted_at must map to status=deleted, got %q", nodes[1].Status)
	}
	if edges := extractEdges(t, toolExtractor{}, src); len(edges) != 0 {
		t.Fatalf("tool extractor emits no edges, got %d", len(edges))
	}
}

func TestExtractor_Skill(t *testing.T) {
	src := &fakeSource{skills: []SkillRow{
		{ID: "uuid-s1", SkillKey: "skill-a", Name: "A", ParentID: " uuid-s2 ", AgentID: "uuid-a1", WorkspaceID: "ws1"},
		{ID: "uuid-s2", SkillKey: "skill-b", Name: "B"},
		{ID: "uuid-s3", SkillKey: "skill-c", ParentID: "uuid-s1", AgentID: "uuid-a1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	nodes := extractNodes(t, skillExtractor{}, src)
	if len(nodes) != 3 || nodes[2].Status != NodeStatusDeleted {
		t.Fatalf("nodes: %+v", nodes)
	}
	edges := extractEdges(t, skillExtractor{}, src)
	if findEdge(edges, EdgeTypeSkillParent, "uuid-s1", NodeTypeSkill, "uuid-s2") == nil {
		t.Fatal("skill_parent edge missing")
	}
	e := findEdge(edges, EdgeTypeOwnsSkill, "uuid-a1", NodeTypeSkill, "uuid-s1")
	if e == nil || e.SrcType != NodeTypeAgent || e.DstKey != "skill-a" {
		t.Fatalf("owns_skill edge wrong: %+v", e)
	}
	// deleted skill: no out-edges from uuid-s3 (neither parent nor owner re-emitted).
	for _, e := range edges {
		if e.SrcRef == "uuid-s3" || (e.Type == EdgeTypeOwnsSkill && e.DstRef == "uuid-s3") {
			t.Fatalf("deleted skill must not produce edges: %+v", e)
		}
	}
}

func TestExtractor_Organization(t *testing.T) {
	src := &fakeSource{orgs: []OrganizationRow{
		{ID: "uuid-org-root", OrgKey: "root", Name: "Root"},
		{ID: "uuid-org-dept", OrgKey: "dept", Name: "Dept", ParentID: "uuid-org-root", DeptLeadAgentID: "uuid-a1"},
	}}
	edges := extractEdges(t, organizationExtractor{}, src)
	if findEdge(edges, EdgeTypeOrgParent, "uuid-org-dept", NodeTypeOrganization, "uuid-org-root") == nil {
		t.Fatal("org_parent missing")
	}
	e := findEdge(edges, EdgeTypeOrgDeptLead, "uuid-org-dept", NodeTypeAgent, "uuid-a1")
	if e == nil || e.SrcType != NodeTypeOrganization {
		t.Fatalf("org_dept_lead wrong: %+v", e)
	}
}

func TestExtractor_KnowledgeCollection(t *testing.T) {
	src := &fakeSource{kcs: []KnowledgeCollectionRow{
		{ID: "uuid-kc1", Name: "kb", Workspace: "ws1"},
		{ID: "uuid-kc2", Name: "old", Status: "Deleted"},
	}}
	nodes := extractNodes(t, knowledgeCollectionExtractor{}, src)
	if len(nodes) != 2 || nodes[0].WorkspaceID != "ws1" || nodes[1].Status != NodeStatusDeleted {
		t.Fatalf("nodes: %+v", nodes)
	}
}

func TestExtractor_MCPServer(t *testing.T) {
	src := &fakeSource{mcps: []MCPServerRow{
		{ID: "uuid-m1", ServerKey: "github", Name: "GitHub", WorkspaceID: "ws1"},
		{ID: "uuid-m2", ServerKey: "dead", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	nodes := extractNodes(t, mcpServerExtractor{}, src)
	if len(nodes) != 2 || nodes[0].NodeKey != "github" || nodes[1].Status != NodeStatusDeleted {
		t.Fatalf("nodes: %+v", nodes)
	}
}

func TestExtractor_Graph(t *testing.T) {
	nodesJSON := `[{"id":"n1","agent_name":"ops_master","fallback_agent":"backup","reviewer_agent":"critic","tool_names":["shell_exec"," web_fetch "]}]`
	src := &fakeSource{graphs: []GraphRow{
		{ID: "uuid-g1", Name: "g-main", TeamID: "uuid-team1", NodesJSON: nodesJSON, WorkspaceID: "ws1"},
		{ID: "uuid-g2", Name: "g-bad", NodesJSON: `{not json`},
		{ID: "uuid-g3", Name: "g-empty"},
	}}
	nodes := extractNodes(t, graphExtractor{}, src)
	if len(nodes) != 3 || nodes[0].NodeKey != "g-main" {
		t.Fatalf("nodes: %+v", nodes)
	}
	edges := extractEdges(t, graphExtractor{}, src)
	// graph_agent ×3 roles（双解：DstRef/DstKey 都带原值）。
	for _, want := range []struct{ ref, role string }{
		{"ops_master", "agent"}, {"backup", "fallback"}, {"critic", "reviewer"},
	} {
		e := findEdge(edges, EdgeTypeGraphAgent, "uuid-g1", NodeTypeAgent, want.ref)
		if e == nil {
			t.Fatalf("graph_agent %s missing", want.ref)
		}
		if e.Evidence["role"] != want.role || e.Evidence["node_id"] != "n1" {
			t.Fatalf("graph_agent evidence wrong: %+v", e.Evidence)
		}
	}
	if countType(edges, EdgeTypeGraphTool) != 2 {
		t.Fatalf("want 2 graph_tool edges, got %d", countType(edges, EdgeTypeGraphTool))
	}
	if findEdge(edges, EdgeTypeGraphOwnedBy, "uuid-g1", NodeTypeTeam, "uuid-team1") == nil {
		t.Fatal("graph_owned_by missing")
	}
	if !hasExtractError(edges, EdgeTypeGraphAgent, "uuid-g2") {
		t.Fatal("bad nodes json must yield extract_error marker")
	}
	if countType(edges, EdgeTypeGraphAgent) != 3 {
		t.Fatalf("bad/empty graphs must not add agent edges, got %d", countType(edges, EdgeTypeGraphAgent))
	}
}

func TestExtractor_Hook(t *testing.T) {
	src := &fakeSource{hooks: []HookRow{
		{ID: "uuid-h1", HookKey: "guard", ConfigJSON: `{"callback_point":"before_tool","condition":{"agent_id":"ops_master","tool_name":"shell_exec"}}`},
		{ID: "uuid-h2", HookKey: "global", ConfigJSON: `{"callback_point":"after_agent"}`},
		{ID: "uuid-h3", HookKey: "bad", ConfigJSON: `{oops`},
		{ID: "uuid-h4", HookKey: "dead", ConfigJSON: `{"condition":{"agent_id":"a1"}}`, DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	nodes := extractNodes(t, hookExtractor{}, src)
	if len(nodes) != 4 || nodes[3].Status != NodeStatusDeleted {
		t.Fatalf("nodes: %+v", nodes)
	}
	edges := extractEdges(t, hookExtractor{}, src)
	if findEdge(edges, EdgeTypeHookRef, "uuid-h1", NodeTypeAgent, "ops_master") == nil {
		t.Fatal("hook_ref agent missing")
	}
	if findEdge(edges, EdgeTypeHookRef, "uuid-h1", NodeTypeTool, "shell_exec") == nil {
		t.Fatal("hook_ref tool missing")
	}
	if countType(edges, EdgeTypeHookRef) != 2 {
		t.Fatalf("global hook must not emit edges, got %d hook_ref", countType(edges, EdgeTypeHookRef))
	}
	if !hasExtractError(edges, EdgeTypeHookRef, "uuid-h3") {
		t.Fatal("bad config must yield extract_error marker")
	}
	for _, e := range edges {
		if e.SrcRef == "uuid-h4" {
			t.Fatal("deleted hook must not emit edges")
		}
	}
}

func TestExtractor_PromptFile(t *testing.T) {
	src := &fakeSource{prompts: []PromptFileRow{
		{ID: "uuid-p1", AgentID: "uuid-a1", AgentKey: "ops_master", FileName: "SOUL.md", Body: "hello", SortOrder: 2},
		{ID: "uuid-p2", AgentID: "uuid-gone", AgentKey: "", FileName: "EXTRA.md", Body: ""},
	}}
	nodes := extractNodes(t, promptFileExtractor{}, src)
	if len(nodes) != 2 {
		t.Fatalf("nodes: %+v", nodes)
	}
	if nodes[0].NodeKey != "ops_master/SOUL.md" {
		t.Fatalf("node key = %q", nodes[0].NodeKey)
	}
	if nodes[0].Attrs["body_hash"] != bodyHash("hello") || nodes[0].Attrs["sort_order"] != 2 {
		t.Fatalf("attrs: %+v", nodes[0].Attrs)
	}
	if nodes[1].NodeKey != "EXTRA.md" {
		t.Fatalf("orphan prompt key degrades to file name, got %q", nodes[1].NodeKey)
	}
	edges := extractEdges(t, promptFileExtractor{}, src)
	e := findEdge(edges, EdgeTypeHasPromptFile, "uuid-a1", NodeTypePromptFile, "uuid-p1")
	if e == nil || e.SrcType != NodeTypeAgent || e.DstKey != "ops_master/SOUL.md" {
		t.Fatalf("has_prompt_file wrong: %+v", e)
	}
}

// ── 0.5 extractors ───────────────────────────────────────────────────────────

func TestExtractor_AgentBoundPosition(t *testing.T) {
	src := &fakeSource{agents: []AgentRow{
		{ID: "uuid-a1", AgentKey: "ops", PositionID: "uuid-org-1", PositionKey: " dept "},
		{ID: "uuid-a2", AgentKey: "noop"},
		{ID: "uuid-a3", AgentKey: "dead", PositionID: "uuid-org-1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	edges := extractEdges(t, agentExtractor{}, src)
	if findEdge(edges, EdgeTypeBoundPosition, "uuid-a1", NodeTypeOrganization, "uuid-org-1") == nil {
		t.Fatal("bound_position missing")
	}
	e := findEdge(edges, EdgeTypeBoundPositionKey, "uuid-a1", NodeTypeOrganization, "dept")
	if e == nil || e.DstRef != "" || e.DstKey != "dept" {
		t.Fatalf("bound_position_key must be key-only: %+v", e)
	}
	if countType(edges, EdgeTypeBoundPosition) != 1 {
		t.Fatal("deleted agent must not emit bound_position")
	}
}

func TestExtractor_AgentGrantedTool(t *testing.T) {
	src := &fakeSource{agents: []AgentRow{{ID: "uuid-a1"}, {ID: "uuid-a2"}, {ID: "uuid-a3"}}}
	prov := fakeProvider{
		eff: map[string]biz.AgentEffectiveTools{
			"uuid-a1": {ToolsEnabled: true, Items: []biz.EffectiveAgentTool{
				{ToolKey: "shell_exec", Enabled: true, EffectiveState: "allowed", Origin: "profile"},
				{ToolKey: "web_fetch", Enabled: true, EffectiveState: "allowed", Origin: "override"},
				{ToolKey: "denied_tool", Enabled: false, EffectiveState: "denied"},
			}},
			"uuid-a2": {ToolsEnabled: true},
		},
		err: map[string]error{"uuid-a3": errors.New("boom")},
	}
	edges := extractEdges(t, agentExtractor{provider: prov}, src)
	e1 := findEdge(edges, EdgeTypeGrantedTool, "uuid-a1", NodeTypeTool, "shell_exec")
	if e1 == nil || e1.Evidence[EvidenceKeyGrantOrigin] != GrantOriginProfile {
		t.Fatalf("profile grant wrong: %+v", e1)
	}
	e2 := findEdge(edges, EdgeTypeGrantedTool, "uuid-a1", NodeTypeTool, "web_fetch")
	if e2 == nil || e2.Evidence[EvidenceKeyGrantOrigin] != GrantOriginOverride {
		t.Fatalf("override grant wrong: %+v", e2)
	}
	if findEdge(edges, EdgeTypeGrantedTool, "uuid-a1", NodeTypeTool, "denied_tool") != nil {
		t.Fatal("denied tool must not become an edge")
	}
	if !hasExtractError(edges, EdgeTypeGrantedTool, "uuid-a3") {
		t.Fatal("provider failure must yield broken marker")
	}
	// nil provider: granted_tool skipped entirely.
	if edges := extractEdges(t, agentExtractor{}, src); countType(edges, EdgeTypeGrantedTool) != 0 {
		t.Fatal("nil provider must skip granted_tool")
	}
}

func TestExtractor_AgentEnablesMCP(t *testing.T) {
	src := &fakeSource{agents: []AgentRow{{ID: "uuid-a1"}, {ID: "uuid-a2"}, {ID: "uuid-a3"}}}
	prov := fakeProvider{eff: map[string]biz.AgentEffectiveTools{
		// gate open + allow entries → edges（deny 不成边）。
		"uuid-a1": {ToolsEnabled: true,
			Allow: []string{"mcp:GitHub", "mcp:github", "mcp:", "plain_tool"},
			Deny:  []string{"mcp:denied_srv"},
			Items: []biz.EffectiveAgentTool{
				{ToolKey: biz.ToolKeyMCPToolSet, Enabled: true, EffectiveState: "allowed", Origin: "allow"},
			}},
		// gate closed（无 mcp 工具）→ 无边。
		"uuid-a2": {ToolsEnabled: true, Allow: []string{"mcp:github"}},
		// 全局工具开关关闭 → 无边。
		"uuid-a3": {ToolsEnabled: false, Allow: []string{"mcp:github"},
			Items: []biz.EffectiveAgentTool{
				{ToolKey: biz.ToolKeyMCPToolSet, Enabled: false, EffectiveState: "denied"},
			}},
	}}
	edges := extractEdges(t, agentExtractor{provider: prov}, src)
	mcpEdges := 0
	for _, e := range edges {
		if e.Type != EdgeTypeEnablesMCP {
			continue
		}
		mcpEdges++
		if e.SrcRef != "uuid-a1" || e.DstType != NodeTypeMCPServer || e.DstKey != "github" {
			t.Fatalf("enables_mcp wrong: %+v", e)
		}
		if e.Evidence["policy"] != "allow" {
			t.Fatalf("policy evidence: %+v", e.Evidence)
		}
	}
	if mcpEdges != 1 {
		t.Fatalf("want exactly 1 enables_mcp edge (deduped, gated), got %d", mcpEdges)
	}
}

func TestExtractor_AgentAllowsSkill(t *testing.T) {
	src := &fakeSource{
		agents: []AgentRow{{ID: "uuid-a1"}, {ID: "uuid-a2"}},
		policies: []AgentToolPolicyRow{
			{AgentID: "uuid-a1", SkillRuntimeJSON: `{"allowed_slugs":["skill-b"," skill-c ",""]}`},
			{AgentID: "uuid-a2", SkillRuntimeJSON: `{bad json`},
		},
	}
	edges := extractEdges(t, agentExtractor{}, src)
	if findEdge(edges, EdgeTypeAllowsSkill, "uuid-a1", NodeTypeSkill, "skill-b") == nil {
		t.Fatal("allows_skill skill-b missing")
	}
	if findEdge(edges, EdgeTypeAllowsSkill, "uuid-a1", NodeTypeSkill, "skill-c") == nil {
		t.Fatal("allows_skill skill-c missing")
	}
	if countType(edges, EdgeTypeAllowsSkill) != 2 {
		t.Fatalf("bad json yields no slugs (ParseRuntimePolicy safe default), got %d", countType(edges, EdgeTypeAllowsSkill))
	}
}

func TestExtractor_AgentToolOverride(t *testing.T) {
	src := &fakeSource{
		agents: []AgentRow{{ID: "uuid-a1"}},
		overrides: []ToolOverrideRow{
			{ID: "ov1", AgentID: "uuid-a1", ToolID: "uuid-t2", ToolKey: "web_fetch", Mode: "allow"},
			{ID: "ov2", AgentID: "", ToolID: "uuid-t2"},
		},
	}
	edges := extractEdges(t, agentExtractor{}, src)
	e := findEdge(edges, EdgeTypeToolOverride, "uuid-a1", NodeTypeTool, "uuid-t2")
	if e == nil || e.DstKey != "web_fetch" || e.Evidence["mode"] != "allow" {
		t.Fatalf("tool_override wrong: %+v", e)
	}
	if countType(edges, EdgeTypeToolOverride) != 1 {
		t.Fatal("row without agent must be skipped")
	}
}

// linked_graph 列空 → definition_json 兜底。

// 内置模板不成边。

// definition_json 坏 → 标记边 + 直读列边仍在。

// deleted → 无边。

// 旧数据：target_type 空 + team_id 有值 → team。

// target_type=team 缺 team_id → 标记边。

// config_json 坏 → 标记边。

// agent 目标缺 agent_id → 标记边。

// deleted → 无边。

// ── integration: full pipeline over one fixture ──────────────────────────────

// 27 类边全覆盖（design §3.2）。

// 双解命中：channel routing agent_key、graph agent_name、hook agent_id(key)、
// skill slug、mcp server_key、tool_key。

// fallback_agent: backup（key）

// JSON 兜底

// ORPHAN position_key → broken（design 0.4 验收锚点）。

// provider 失败 → broken 标记边。

// 幂等：重跑产出同一 stored 集（确定性 ID + 去重）。
