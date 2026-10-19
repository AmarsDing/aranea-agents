package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// stubMCPListRepo implements only ListMCPServers (the sole repo method the
// effective-MCP-servers path calls); other biz.MCPServerRepo methods panic via
// the embedded nil interface if ever reached.
type stubMCPListRepo struct {
	biz.MCPServerRepo
	rows  []biz.MCPServer
	calls int
}

func (s *stubMCPListRepo) ListMCPServers(_ context.Context, q biz.MCPListQuery) ([]biz.MCPServer, error) {
	s.calls++
	return s.rows, nil
}

func mcpGateOpenEffectiveTools() *biz.AgentEffectiveTools {
	return &biz.AgentEffectiveTools{
		ToolsEnabled: true,
		Items:        []biz.EffectiveAgentTool{{ToolKey: biz.ToolKeyMCPToolSet, Enabled: true}},
	}
}

// P3-1: with the turn's effective-tools result in hand, computeMCPHash must use
// the hash-only path (no duplicate effective-tools resolution, no decryption).
// The stub's agents side is nil: the legacy EffectiveServersForAgent path
// would yield an empty hash, so a non-empty hash PROVES the cached-eff path.
func TestComputeMCPHash_ReusesCachedEffectiveTools(t *testing.T) {
	repo := &stubMCPListRepo{rows: []biz.MCPServer{
		{ID: "s1", Key: "alpha", Enabled: true, Status: string(biz.AgentStatusActive), ConfigJSON: `{"enc":"RAW"}`},
	}}
	d := &chatAgentBuildDirector{td: rt.TurnDeps{
		Persist: rt.PersistenceSet{AgentMCP: biz.NewAgentMCPTooling(nil, biz.NewMCPServerUsecase(repo, nil, nil, nil, nil))},
	}}

	got := d.computeMCPHash(context.Background(), "a1", mcpGateOpenEffectiveTools())
	if got == "" {
		t.Fatal("hash-only path with enabled servers must produce a non-empty hash")
	}
	if repo.calls != 1 {
		t.Fatalf("exactly one List call expected (workspace-scoped server rows only), got %d", repo.calls)
	}
}

// Nil eff (narrow stub / failed effective-tools arm) falls back to the legacy
// EffectiveServersForAgent resolution. With a nil agents usecase that path
// yields an empty hash — same as the pre-P3-1 behavior for this stub shape.
func TestComputeMCPHash_NilEffLegacyFallback(t *testing.T) {
	repo := &stubMCPListRepo{rows: []biz.MCPServer{
		{ID: "s1", Key: "alpha", Enabled: true},
	}}
	d := &chatAgentBuildDirector{td: rt.TurnDeps{
		Persist: rt.PersistenceSet{AgentMCP: biz.NewAgentMCPTooling(nil, biz.NewMCPServerUsecase(repo, nil, nil, nil, nil))},
	}}

	if got := d.computeMCPHash(context.Background(), "a1", nil); got != "" {
		t.Fatalf("legacy fallback with nil agents usecase must yield empty hash, got %q", got)
	}
	if repo.calls != 0 {
		t.Fatalf("legacy fallback must not reach the repo when agents usecase is nil, got %d calls", repo.calls)
	}
}

// Nil AgentMCP tooling -> empty hash, no panic.
func TestComputeMCPHash_NilTooling(t *testing.T) {
	d := &chatAgentBuildDirector{}
	if got := d.computeMCPHash(context.Background(), "a1", mcpGateOpenEffectiveTools()); got != "" {
		t.Fatalf("nil AgentMCP must yield empty hash, got %q", got)
	}
}
