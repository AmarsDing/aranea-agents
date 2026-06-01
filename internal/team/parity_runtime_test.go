package team

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"
)

func stubParityCatalogAgent(_ context.Context, id string) (biz.Agent, error) {
	return biz.Agent{
		ID:          id,
		AgentKey:    "key-" + id,
		DisplayName: id,
		Provider:    "openai",
		Model:       "gpt-4",
	}, nil
}

func buildRuntimeGraphFromDef(t *testing.T, def Definition) int {
	t.Helper()
	cfg, err := CompileToGraphRuntimeConfig(def, compileAgentKey, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("CompileToGraphRuntimeConfig: %v", err)
	}
	trpcCfg := graphtrpc.GraphBuildConfig{
		EntryPoint:  cfg.EntryPoint,
		FinishPoint: cfg.FinishPoint,
		Nodes:       make([]graphtrpc.NodeDef, len(cfg.Nodes)),
		Edges:       make([]graphtrpc.EdgeDef, len(cfg.Edges)),
	}
	for i, n := range cfg.Nodes {
		trpcCfg.Nodes[i] = graphtrpc.NodeDef{
			ID:               n.ID,
			Type:             n.Type,
			AgentName:        n.AgentName,
			RetryMaxAttempts: n.RetryMaxAttempts,
			Destinations:     append([]string(nil), n.Destinations...),
		}
	}
	for i, e := range cfg.Edges {
		trpcCfg.Edges[i] = graphtrpc.EdgeDef{From: e.From, To: e.To}
	}
	g, agents, err := graphtrpc.BuildStateGraphWithAgents(context.Background(), trpcCfg, &graphtrpc.BuildDeps{
		Agents: stubAgentResolver{},
	}, nil)
	if err != nil {
		t.Fatalf("BuildStateGraphWithAgents: %v", err)
	}
	if g == nil {
		t.Fatal("graph nil")
	}
	if _, err := graphtrpc.NewGraphAgent("team-parity-"+def.Mode, g, false, agents...); err != nil {
		t.Fatalf("NewGraphAgent: %v", err)
	}
	return len(agents)
}

// TestParityNativeVsGraph_RuntimeBuildAllModes builds GraphAgent runtime for every mode and
// verifies native member keys align with compiled graph agent nodes (TG-RT-PARITY runtime E2E).
func TestParityNativeVsGraph_RuntimeBuildAllModes(t *testing.T) {
	modes := []string{"sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm"}
	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			def := parityFixture(mode)
			if !SupportsTeamGraphRuntimeMode(mode) {
				t.Fatalf("mode %q should support graph runtime", mode)
			}
			wantAgents := len(EnabledMembers(def))
			graphAgents := buildRuntimeGraphFromDef(t, def)
			if graphAgents != wantAgents {
				t.Fatalf("graph agents: got %d want %d", graphAgents, wantAgents)
			}

			nativeKeys, err := memberAgentKeys(context.Background(), def, stubParityCatalogAgent)
			if err != nil {
				t.Fatalf("memberAgentKeys: %v", err)
			}
			if len(nativeKeys) != wantAgents {
				t.Fatalf("native keys: got %d want %d", len(nativeKeys), wantAgents)
			}

			cfg, err := CompileToGraphRuntimeConfig(def, compileAgentKey, loggateway.NewNoop())
			if err != nil {
				t.Fatalf("CompileToGraphRuntimeConfig: %v", err)
			}
			graphKeys := make(map[string]struct{}, wantAgents)
			for _, n := range cfg.Nodes {
				if n.Type != "agent" {
					continue
				}
				if key := strings.TrimSpace(n.AgentName); key != "" {
					graphKeys[key] = struct{}{}
				}
			}
			if len(graphKeys) != wantAgents {
				t.Fatalf("compiled graph keys: got %d want %d", len(graphKeys), wantAgents)
			}
			for _, key := range nativeKeys {
				if _, ok := graphKeys[key]; !ok {
					t.Fatalf("native key %q missing from graph runtime nodes", key)
				}
			}
		})
	}
}
