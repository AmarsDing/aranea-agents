package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

// parityFixture builds a minimal team definition for each orchestration mode.
func parityFixture(mode string) Definition {
	base := Definition{
		Mode:          mode,
		RuntimeEngine: "graph",
		Members: []MemberDef{
			{AgentID: "agent-a", Role: "worker", Name: "Worker A", SortOrder: 10},
			{AgentID: "agent-b", Role: "worker", Name: "Worker B", SortOrder: 20},
			{AgentID: "agent-c", Role: "critic", Name: "Critic", SortOrder: 30},
		},
	}
	switch mode {
	case "parallel":
		base.SynthesizerAgentID = "agent-c"
	case "critic_loop":
		base.Members = base.Members[:2]
		base.Members[0].Role = "generator"
		base.Members[1].Role = "critic"
	case "coordinator":
		base.Members[0].Role = "coordinator"
		base.Members[1].Role = "worker"
		base.Members = base.Members[:2]
	}
	return base
}

func compileAgentKey(agentID string) string {
	return "key-" + agentID
}

func countAgentNodes(cfg biz.GraphBuildConfig) int {
	n := 0
	for _, node := range cfg.Nodes {
		if node.Type == "agent" {
			n++
		}
	}
	return n
}

func TestParityNativeVsGraph_CompileAllModes(t *testing.T) {
	modes := []string{"sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm"}
	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			def := parityFixture(mode)
			if !SupportsTeamGraphRuntimeMode(mode) {
				t.Fatalf("mode %q should support graph runtime", mode)
			}
			if !TeamGraphRuntimeEnabled(def) {
				t.Fatalf("graph runtime should be enabled for mode %q", mode)
			}

			buildCfg, err := CompileToGraphBuildConfig(def, compileAgentKey)
			if err != nil {
				t.Fatalf("CompileToGraphBuildConfig: %v", err)
			}
			if buildCfg.EntryPoint == "" || buildCfg.FinishPoint == "" {
				t.Fatalf("entry/finish required: entry=%q finish=%q", buildCfg.EntryPoint, buildCfg.FinishPoint)
			}
			wantAgents := len(EnabledMembers(def))
			if got := countAgentNodes(buildCfg); got != wantAgents {
				t.Fatalf("agent nodes: got %d want %d", got, wantAgents)
			}

			runtimeCfg, err := CompileToGraphRuntimeConfig(def, compileAgentKey)
			if err != nil {
				t.Fatalf("CompileToGraphRuntimeConfig: %v", err)
			}
			if runtimeCfg.EntryPoint != buildCfg.EntryPoint {
				t.Fatalf("runtime entry mismatch: %q vs %q", runtimeCfg.EntryPoint, buildCfg.EntryPoint)
			}
			if countAgentNodes(runtimeCfg) != wantAgents {
				t.Fatalf("runtime agent count mismatch")
			}
		})
	}
}

func TestParityCompileSnapshot_AllModes(t *testing.T) {
	modes := []string{"sequential", "parallel", "coordinator", "critic_loop", "adaptive"}
	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			def := parityFixture(mode)
			snap := BuildCompileSnapshot(def, "", compileAgentKey)
			if !snap.Valid {
				t.Fatalf("compile snapshot invalid: %s", snap.CompileError)
			}
			if snap.Mode != normalizeCompileMode(mode) {
				t.Fatalf("mode: got %q want normalized %q", snap.Mode, normalizeCompileMode(mode))
			}
			if len(snap.Nodes) == 0 {
				t.Fatal("expected compiled nodes")
			}
		})
	}
}
