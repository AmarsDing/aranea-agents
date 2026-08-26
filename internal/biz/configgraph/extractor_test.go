package configgraph

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestExtractor_Team(t *testing.T) {
	defJSON := `{
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
	}`
	src := &fakeSource{teams: []TeamRow{
		{ID: "uuid-team1", TeamKey: "main", DepartmentID: "uuid-org-dept", DeptLeadAgentID: "uuid-a1",
			CrossDeptMemberIDs: `["uuid-a2"," uuid-a3 "]`, LinkedGraphID: "uuid-g1", DefinitionJSON: defJSON},
		// linked_graph 列空 → definition_json 兜底。
		{ID: "uuid-team2", TeamKey: "legacy", DefinitionJSON: `{"linked_graph_id":"uuid-g2","members":[]}`},
		// 内置模板不成边。
		{ID: "uuid-team3", TeamKey: "builtin", DefinitionJSON: `{"graph_template_id":"pipeline","members":[]}`},
		// definition_json 坏 → 标记边 + 直读列边仍在。
		{ID: "uuid-team4", TeamKey: "broken", DeptLeadAgentID: "uuid-a1", DefinitionJSON: `{oops`},
		// deleted → 无边。
		{ID: "uuid-team5", TeamKey: "dead", DeptLeadAgentID: "uuid-a1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	nodes := extractNodes(t, teamExtractor{}, src)
	if len(nodes) != 5 || nodes[0].Attrs["is_default"] != false {
		t.Fatalf("nodes: %+v", nodes)
	}
	edges := extractEdges(t, teamExtractor{}, src)
	cases := []struct{ typ, src, dstType, dst string }{
		{EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeSynthesizer, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeIntentAnchor, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeFallbackAgent, "uuid-team1", NodeTypeAgent, "uuid-a3"},
		{EdgeTypeDeptLead, "uuid-team1", NodeTypeAgent, "uuid-a1"},
		{EdgeTypeCrossDeptMember, "uuid-team1", NodeTypeAgent, "uuid-a2"},
		{EdgeTypeCrossDeptMember, "uuid-team1", NodeTypeAgent, "uuid-a3"},
		{EdgeTypeBelongsTo, "uuid-team1", NodeTypeOrganization, "uuid-org-dept"},
		{EdgeTypeLinkedGraph, "uuid-team1", NodeTypeGraph, "uuid-g1"},
		{EdgeTypeLinkedGraph, "uuid-team2", NodeTypeGraph, "uuid-g2"},
		{EdgeTypeGraphTemplate, "uuid-team1", NodeTypeGraph, "uuid-g2"},
		{EdgeTypeScopedKnowledge, "uuid-team1", NodeTypeKnowledgeCollection, "uuid-kc1"},
	}
	for _, c := range cases {
		if findEdge(edges, c.typ, c.src, c.dstType, c.dst) == nil {
			t.Fatalf("%s %s→%s missing", c.typ, c.src, c.dst)
		}
	}
	if e := findEdge(edges, EdgeTypeHasMember, "uuid-team1", NodeTypeAgent, "uuid-a1"); e.Evidence["role"] != "leader" {
		t.Fatalf("member role evidence: %+v", e.Evidence)
	}
	if e := findEdge(edges, EdgeTypeFallbackAgent, "uuid-team1", NodeTypeAgent, "uuid-a3"); e.Evidence["node_key"] != "member-1" {
		t.Fatalf("fallback node_key evidence: %+v", e.Evidence)
	}
	if findEdge(edges, EdgeTypeGraphTemplate, "uuid-team3", NodeTypeGraph, "pipeline") != nil {
		t.Fatal("builtin graph template must not emit edge")
	}
	if !hasExtractError(edges, EdgeTypeHasMember, "uuid-team4") {
		t.Fatal("bad definition_json must yield extract_error marker")
	}
	if findEdge(edges, EdgeTypeDeptLead, "uuid-team4", NodeTypeAgent, "uuid-a1") == nil {
		t.Fatal("column edges must survive definition_json parse failure")
	}
	for _, e := range edges {
		if e.SrcRef == "uuid-team5" {
			t.Fatal("deleted team must not emit edges")
		}
	}
}

func TestExtractor_CronTask(t *testing.T) {
	src := &fakeSource{crons: []CronTaskRow{
		{ID: "uuid-c1", TaskKey: "agent-job", AgentID: "uuid-a1", ConfigJSON: `{"target_type":"agent"}`},
		{ID: "uuid-c2", TaskKey: "team-job", ConfigJSON: `{"target_type":"team","team_id":"uuid-team1"}`},
		{ID: "uuid-c3", TaskKey: "sync-job", ConfigJSON: `{"target_type":"model_registry_sync"}`},
		// 旧数据：target_type 空 + team_id 有值 → team。
		{ID: "uuid-c4", TaskKey: "legacy-team", ConfigJSON: `{"team_id":"uuid-team2"}`},
		// target_type=team 缺 team_id → 标记边。
		{ID: "uuid-c5", TaskKey: "broken-team", ConfigJSON: `{"target_type":"team"}`},
		// config_json 坏 → 标记边。
		{ID: "uuid-c6", TaskKey: "bad-json", AgentID: "uuid-a1", ConfigJSON: `{oops`},
		// agent 目标缺 agent_id → 标记边。
		{ID: "uuid-c7", TaskKey: "no-agent", ConfigJSON: `{}`},
		// deleted → 无边。
		{ID: "uuid-c8", TaskKey: "dead", AgentID: "uuid-a1", DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	edges := extractEdges(t, cronTaskExtractor{}, src)
	if e := findEdge(edges, EdgeTypeRuns, "uuid-c1", NodeTypeAgent, "uuid-a1"); e == nil || e.Evidence["target_type"] != "agent" {
		t.Fatalf("agent runs wrong: %+v", e)
	}
	if findEdge(edges, EdgeTypeRuns, "uuid-c2", NodeTypeTeam, "uuid-team1") == nil {
		t.Fatal("team runs missing")
	}
	if findEdge(edges, EdgeTypeRuns, "uuid-c4", NodeTypeTeam, "uuid-team2") == nil {
		t.Fatal("legacy team inference missing")
	}
	for _, id := range []string{"uuid-c3", "uuid-c8"} {
		for _, e := range edges {
			if e.SrcRef == id {
				t.Fatalf("%s must not emit edges: %+v", id, e)
			}
		}
	}
	for _, id := range []string{"uuid-c5", "uuid-c6", "uuid-c7"} {
		if !hasExtractError(edges, EdgeTypeRuns, id) {
			t.Fatalf("%s must yield extract_error marker", id)
		}
	}
}

func TestExtractor_Channel(t *testing.T) {
	cfg := `{"routing":{"default_agent_id":"ops_master","default_team_id":"uuid-team2","rules":[` +
		`{"peer_pattern":"u_*","agent_id":"uuid-a2"},` +
		`{"peer_pattern":"g_*","team_id":"uuid-team1"}]}}`
	src := &fakeSource{channels: []ChannelRow{
		{ID: "uuid-ch1", ChannelKey: "feishu", ConfigJSON: cfg},
		{ID: "uuid-ch2", ChannelKey: "empty"},
		{ID: "uuid-ch3", ChannelKey: "bad", ConfigJSON: `{oops`},
		{ID: "uuid-ch4", ChannelKey: "dead", ConfigJSON: cfg, DeletedAt: "2026-01-01T00:00:00Z"},
	}}
	edges := extractEdges(t, channelExtractor{}, src)
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeAgent, "ops_master") == nil {
		t.Fatal("default agent (key) missing")
	}
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeTeam, "uuid-team2") == nil {
		t.Fatal("default team missing")
	}
	e := findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeAgent, "uuid-a2")
	if e == nil || e.Evidence["peer_pattern"] != "u_*" {
		t.Fatalf("rule agent wrong: %+v", e)
	}
	if findEdge(edges, EdgeTypeRoutesTo, "uuid-ch1", NodeTypeTeam, "uuid-team1") == nil {
		t.Fatal("rule team missing")
	}
	if countType(edges, EdgeTypeRoutesTo) != 4 {
		t.Fatalf("want 4 routes_to, got %d", countType(edges, EdgeTypeRoutesTo))
	}
	if !hasExtractError(edges, EdgeTypeRoutesTo, "uuid-ch3") {
		t.Fatal("bad config must yield extract_error marker")
	}
	for _, e := range edges {
		if e.SrcRef == "uuid-ch4" {
			t.Fatal("deleted channel must not emit edges")
		}
	}
}

// ── integration: full pipeline over one fixture ──────────────────────────────

func TestResolveEdges_FullPipeline(t *testing.T) {
	src, prov := fullFixture()
	xs := Extractors(prov)

	var nodes []Node
	var edges []Edge
	for _, x := range xs {
		nodes = append(nodes, extractNodes(t, x, src)...)
		edges = append(edges, extractEdges(t, x, src)...)
	}
	idx := NewNodeIndex(nodes)
	gen := int64(1)
	now := parseTime(t, "2026-08-26T00:00:00Z")
	stored := ResolveEdges(edges, idx, gen, now)

	// 27 类边全覆盖（design §3.2）。
	allTypes := []string{
		EdgeTypeHasMember, EdgeTypeSynthesizer, EdgeTypeIntentAnchor, EdgeTypeFallbackAgent,
		EdgeTypeDeptLead, EdgeTypeCrossDeptMember, EdgeTypeBelongsTo, EdgeTypeLinkedGraph,
		EdgeTypeGraphTemplate, EdgeTypeScopedKnowledge, EdgeTypeGraphAgent, EdgeTypeGraphTool,
		EdgeTypeGraphOwnedBy, EdgeTypeHasPromptFile, EdgeTypeGrantedTool, EdgeTypeToolOverride,
		EdgeTypeEnablesMCP, EdgeTypeOwnsSkill, EdgeTypeAllowsSkill, EdgeTypeSkillParent,
		EdgeTypeBoundPosition, EdgeTypeBoundPositionKey, EdgeTypeOrgDeptLead, EdgeTypeOrgParent,
		EdgeTypeRuns, EdgeTypeRoutesTo, EdgeTypeHookRef,
	}
	if len(allTypes) != 27 {
		t.Fatalf("test must track 27 edge types, got %d", len(allTypes))
	}
	byType := map[string][]StoredEdge{}
	for _, se := range stored {
		byType[se.Type] = append(byType[se.Type], se)
	}
	for _, typ := range allTypes {
		if len(byType[typ]) == 0 {
			t.Fatalf("edge type %s missing from full pipeline", typ)
		}
	}

	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	resolved := func(typ, srcRef, dstRef string) *StoredEdge {
		for i := range stored {
			se := &stored[i]
			if se.Type != typ || se.Broken() {
				continue
			}
			if byID[se.SrcID].RefID == srcRef && byID[se.DstID].RefID == dstRef {
				return se
			}
		}
		return nil
	}

	// 双解命中：channel routing agent_key、graph agent_name、hook agent_id(key)、
	// skill slug、mcp server_key、tool_key。
	dualCases := []struct{ typ, src, dst string }{
		{EdgeTypeRoutesTo, "uuid-ch1", "uuid-a1"},
		{EdgeTypeGraphAgent, "uuid-g1", "uuid-a1"},
		{EdgeTypeGraphAgent, "uuid-g1", "uuid-a2"}, // fallback_agent: backup（key）
		{EdgeTypeGraphTool, "uuid-g1", "uuid-t1"},
		{EdgeTypeHookRef, "uuid-h1", "uuid-a1"},
		{EdgeTypeAllowsSkill, "uuid-a1", "uuid-s2"},
		{EdgeTypeEnablesMCP, "uuid-a1", "uuid-m1"},
		{EdgeTypeGrantedTool, "uuid-a1", "uuid-t1"},
		{EdgeTypeLinkedGraph, "uuid-team2", "uuid-g2"}, // JSON 兜底
		{EdgeTypeRuns, "uuid-c2", "uuid-team1"},
	}
	for _, c := range dualCases {
		if resolved(c.typ, c.src, c.dst) == nil {
			t.Fatalf("resolved %s %s→%s missing", c.typ, c.src, c.dst)
		}
	}
	if se := resolved(EdgeTypeGrantedTool, "uuid-a1", "uuid-t2"); se == nil ||
		se.Evidence[EvidenceKeyGrantOrigin] != GrantOriginOverride {
		t.Fatalf("granted_tool override origin: %+v", se)
	}

	// ORPHAN position_key → broken（design 0.4 验收锚点）。
	var orphan *StoredEdge
	for i := range stored {
		se := &stored[i]
		if se.Type == EdgeTypeBoundPositionKey && se.Broken() {
			orphan = se
		}
	}
	if orphan == nil {
		t.Fatal("ORPHAN position_key must produce broken bound_position_key")
	}
	if orphan.Evidence[EvidenceKeyDstKey] != "ORPHAN-KEY" || orphan.DstID != "" {
		t.Fatalf("broken orphan edge wrong: %+v", orphan)
	}

	// provider 失败 → broken 标记边。
	var provBroken bool
	for i := range stored {
		se := &stored[i]
		if se.Type == EdgeTypeGrantedTool && se.Broken() {
			if msg, _ := se.Evidence["extract_error"].(string); strings.Contains(msg, "boom") {
				provBroken = true
			}
		}
	}
	if !provBroken {
		t.Fatal("provider failure must surface as broken granted_tool")
	}

	// 幂等：重跑产出同一 stored 集（确定性 ID + 去重）。
	stored2 := ResolveEdges(edges, idx, gen, now)
	if len(stored) != len(stored2) {
		t.Fatalf("rerun size drift: %d vs %d", len(stored), len(stored2))
	}
	m1 := map[string]StoredEdge{}
	for _, se := range stored {
		m1[se.ID] = se
	}
	for _, se := range stored2 {
		prev, ok := m1[se.ID]
		if !ok {
			t.Fatalf("rerun produced new edge id %s", se.ID)
		}
		if prev.DstID != se.DstID || prev.Type != se.Type || prev.Broken() != se.Broken() {
			t.Fatalf("rerun mismatch for %s", se.ID)
		}
	}
}

func parseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
