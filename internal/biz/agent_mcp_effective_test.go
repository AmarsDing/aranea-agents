package biz

import (
	"context"
	stderrors "errors"
	"testing"

	"aranea-agents/internal/workspace"
)

func TestMCPPolicyFromAgentEffectiveTools_allowDeny(t *testing.T) {
	eff := AgentEffectiveTools{
		Allow: []string{"read_file", "mcp:alpha", "mcp:beta"},
		Deny:  []string{"mcp:gamma"},
	}
	pol := MCPPolicyFromAgentEffectiveTools(eff)
	if len(pol.AllowServerKeys) != 2 || pol.AllowServerKeys[0] != "alpha" || pol.AllowServerKeys[1] != "beta" {
		t.Fatalf("allow keys: %#v", pol.AllowServerKeys)
	}
	if len(pol.DenyServerKeys) != 1 || pol.DenyServerKeys[0] != "gamma" {
		t.Fatalf("deny keys: %#v", pol.DenyServerKeys)
	}
}

func TestFilterEffectiveMCPServers_allowAndDeny(t *testing.T) {
	servers := []EffectiveMCPServer{
		{ServerKey: "alpha", ConfigJSON: "{}"},
		{ServerKey: "beta", ConfigJSON: "{}"},
		{ServerKey: "gamma", ConfigJSON: "{}"},
	}
	pol := EffectiveMCPPolicy{AllowServerKeys: []string{"alpha", "beta"}, DenyServerKeys: []string{"beta"}}
	out := FilterEffectiveMCPServers(servers, pol)
	if len(out) != 1 || out[0].ServerKey != "alpha" {
		t.Fatalf("got %#v", out)
	}
}

func TestFilterEffectiveMCPServers_emptyAllowMeansAllExceptDeny(t *testing.T) {
	servers := []EffectiveMCPServer{{ServerKey: "a"}, {ServerKey: "b"}}
	out := FilterEffectiveMCPServers(servers, EffectiveMCPPolicy{DenyServerKeys: []string{"b"}})
	if len(out) != 1 || out[0].ServerKey != "a" {
		t.Fatalf("got %#v", out)
	}
}

func TestFilterEnabledMCPServerRows(t *testing.T) {
	rows := []MCPServer{
		{ID: "1", Key: "enabled-active", Enabled: true, Status: string(AgentStatusActive)},
		{ID: "2", Key: "enabled-empty-status", Enabled: true, Status: ""},
		{ID: "3", Key: "disabled", Enabled: false},
		{ID: "4", Key: "deleted", Enabled: true, DeletedAt: "2026-08-07T00:00:00Z"},
		{ID: "5", Key: "draft", Enabled: true, Status: "draft"},
	}
	out := filterEnabledMCPServerRows(rows)
	if len(out) != 2 {
		t.Fatalf("want 2 surviving rows, got %d: %#v", len(out), out)
	}
	if out[0].ServerKey != "enabled-active" || out[1].ServerKey != "enabled-empty-status" {
		t.Fatalf("unexpected survivors: %#v", out)
	}
	// ConfigJSON must carry over for the runtime conversion path.
	for _, s := range out {
		if s.ID == "" {
			t.Fatalf("ID must be preserved: %#v", s)
		}
	}
}

// captureMCPRepo captures the last ListMCPServers query and can inject errors.
// Only ListMCPServers is implemented; other MCPServerRepo methods panic via the
// embedded nil interface (never called by the effective-servers paths).
type captureMCPRepo struct {
	MCPServerRepo
	rows      []MCPServer
	err       error
	lastQuery MCPListQuery
	calls     int
}

func (s *captureMCPRepo) ListMCPServers(_ context.Context, q MCPListQuery) ([]MCPServer, error) {
	s.calls++
	s.lastQuery = q
	return s.rows, s.err
}

func mcpGateOpenEff() *AgentEffectiveTools {
	return &AgentEffectiveTools{
		ToolsEnabled: true,
		Items:        []EffectiveAgentTool{{ToolKey: ToolKeyMCPToolSet, Enabled: true}},
	}
}

func enabledMCPRows() []MCPServer {
	return []MCPServer{
		{ID: "s1", Key: "alpha", Enabled: true, Status: string(AgentStatusActive), ConfigJSON: `{"enc":"RAW-alpha"}`},
		{ID: "s2", Key: "beta", Enabled: true, ConfigJSON: `{"enc":"RAW-beta"}`},
		{ID: "s3", Key: "disabled", Enabled: false, ConfigJSON: `{"enc":"RAW-disabled"}`},
	}
}

func TestEffectiveServersForHash_Guards(t *testing.T) {
	eff := mcpGateOpenEff()
	if out := (*AgentMCPTooling)(nil).EffectiveServersForHash(context.Background(), "a1", eff); out != nil {
		t.Fatalf("nil tooling must yield nil, got %#v", out)
	}
	if out := NewAgentMCPTooling(nil, nil).EffectiveServersForHash(context.Background(), "a1", eff); out != nil {
		t.Fatalf("nil mcp usecase must yield nil, got %#v", out)
	}
	repo := &captureMCPRepo{rows: enabledMCPRows()}
	tool := NewAgentMCPTooling(nil, NewMCPServerUsecase(repo, nil, nil, nil, nil))
	if out := tool.EffectiveServersForHash(context.Background(), "a1", nil); out != nil {
		t.Fatalf("nil eff must yield nil, got %#v", out)
	}
	if repo.calls != 0 {
		t.Fatalf("nil eff must not hit the repo, got %d calls", repo.calls)
	}
}

func TestEffectiveServersForHash_GateClosedSkipsList(t *testing.T) {
	repo := &captureMCPRepo{rows: enabledMCPRows()}
	tool := NewAgentMCPTooling(nil, NewMCPServerUsecase(repo, nil, nil, nil, nil))

	// ToolsEnabled on but no mcp_tool_set / mcp_broker item -> gate closed.
	eff := &AgentEffectiveTools{
		ToolsEnabled: true,
		Items:        []EffectiveAgentTool{{ToolKey: ToolKeyKnowledgeSearch, Enabled: true}},
	}
	if out := tool.EffectiveServersForHash(context.Background(), "a1", eff); out != nil {
		t.Fatalf("gate closed must yield nil, got %#v", out)
	}
	// Tools disabled entirely -> gate closed.
	effOff := &AgentEffectiveTools{ToolsEnabled: false}
	if out := tool.EffectiveServersForHash(context.Background(), "a1", effOff); out != nil {
		t.Fatalf("tools disabled must yield nil, got %#v", out)
	}
	if repo.calls != 0 {
		t.Fatalf("gate closed must not hit the repo, got %d calls", repo.calls)
	}
}

func TestEffectiveServersForHash_RawConfigAndPolicyFilter(t *testing.T) {
	repo := &captureMCPRepo{rows: enabledMCPRows()}
	tool := NewAgentMCPTooling(nil, NewMCPServerUsecase(repo, nil, nil, nil, nil))

	eff := mcpGateOpenEff()
	eff.Deny = []string{"mcp:beta"}
	out := tool.EffectiveServersForHash(context.Background(), "a1", eff)
	if len(out) != 1 || out[0].ServerKey != "alpha" {
		t.Fatalf("want only alpha (beta denied, s3 disabled), got %#v", out)
	}
	// P3-1: the hash path must carry the RAW stored ConfigJSON (no C-05
	// decryption) — decryption stays on the assembly path only.
	if out[0].ConfigJSON != `{"enc":"RAW-alpha"}` {
		t.Fatalf("hash path must use raw ConfigJSON, got %q", out[0].ConfigJSON)
	}
}

func TestEffectiveServersForHash_WorkspaceScoping(t *testing.T) {
	repo := &captureMCPRepo{rows: enabledMCPRows()}
	tool := NewAgentMCPTooling(nil, NewMCPServerUsecase(repo, nil, nil, nil, nil))
	eff := mcpGateOpenEff()

	ctx := workspace.WithContext(context.Background(), "ws-a")
	tool.EffectiveServersForHash(ctx, "a1", eff)
	if repo.lastQuery.WorkspaceID != "ws-a" {
		t.Fatalf("tenant ctx must scope List by workspace, got %q", repo.lastQuery.WorkspaceID)
	}

	sysCtx := workspace.WithSystemWorkspace(context.Background())
	tool.EffectiveServersForHash(sysCtx, "a1", eff)
	if repo.lastQuery.WorkspaceID != "" {
		t.Fatalf("system caller must bypass workspace filter, got %q", repo.lastQuery.WorkspaceID)
	}
}

func TestEffectiveServers_ListErrorSemantics(t *testing.T) {
	repo := &captureMCPRepo{err: stderrors.New("store down")}
	tool := NewAgentMCPTooling(nil, NewMCPServerUsecase(repo, nil, nil, nil, nil))
	eff := mcpGateOpenEff()

	// Hash path: best-effort degrade to empty (a missed hash only costs one
	// rebuild, never blocks the turn).
	if out := tool.EffectiveServersForHash(context.Background(), "a1", eff); out != nil {
		t.Fatalf("hash path must degrade to nil on List error, got %#v", out)
	}
	// Assembly path: the error must PROPAGATE (silently assembling an agent
	// without its MCP servers is worse than failing the build).
	if _, err := tool.effectiveServersForEffectiveTools(context.Background(), eff); err == nil {
		t.Fatal("assembly path must propagate List errors")
	}
}
