package team

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestCompileToGraphBuildConfig_sequential(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a1", Role: "worker", SortOrder: 1},
			{AgentID: "a2", Role: "worker", SortOrder: 2},
			{AgentID: "a3", Role: "synthesizer", SortOrder: 3},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, func(id string) string { return "key-" + id }, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Nodes) != 3 {
		t.Fatalf("nodes=%d want 3", len(cfg.Nodes))
	}
	if cfg.EntryPoint != "member-1" || cfg.FinishPoint != "member-3" {
		t.Fatalf("entry/finish=%q/%q", cfg.EntryPoint, cfg.FinishPoint)
	}
	if len(cfg.Edges) != 2 {
		t.Fatalf("edges=%d want 2", len(cfg.Edges))
	}
	if cfg.Nodes[0].AgentName != "key-a1" {
		t.Fatalf("agent name=%q", cfg.Nodes[0].AgentName)
	}
}

func TestCompileToGraphBuildConfig_parallel(t *testing.T) {
	def := Definition{
		Mode:               "parallel",
		SynthesizerAgentID: "synth",
		Members: []MemberDef{
			{AgentID: "w1", SortOrder: 1},
			{AgentID: "w2", SortOrder: 2},
			{AgentID: "synth", Role: "synthesizer", SortOrder: 3},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FinishPoint != "member-3" {
		t.Fatalf("finish=%q want member-3", cfg.FinishPoint)
	}
	if len(cfg.Edges) < 2 {
		t.Fatalf("expected fan-out/join edges, got %d", len(cfg.Edges))
	}
	if CompileTemplateID(def.Mode) != "parallel_review" {
		t.Fatalf("template=%q", CompileTemplateID(def.Mode))
	}
}

func TestCompileToGraphBuildConfig_coordinator(t *testing.T) {
	def := Definition{
		Mode: "coordinator",
		Members: []MemberDef{
			{AgentID: "lead", SortOrder: 1, Role: "coordinator"},
			{AgentID: "w1", SortOrder: 2},
			{AgentID: "w2", SortOrder: 3},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if CompileTemplateID(def.Mode) != "dispatch" {
		t.Fatalf("template=%q", CompileTemplateID(def.Mode))
	}
	if len(cfg.Edges) != 4 {
		t.Fatalf("edges=%d want 4", len(cfg.Edges))
	}
	var transfers, dispatches, flows int
	for _, e := range cfg.Edges {
		switch e.Kind {
		case "transfer":
			transfers++
		case "dispatch":
			dispatches++
		case "flow":
			flows++
		}
	}
	if dispatches != 2 || flows != 2 {
		t.Fatalf("dispatch=%d flow=%d want 2/2", dispatches, flows)
	}
}

func TestCompileToGraphBuildConfig_coordinator_noSelfLoopOnFinish(t *testing.T) {
	def := Definition{
		Mode: "coordinator",
		Members: []MemberDef{
			{AgentID: "lead", SortOrder: 10, Role: "coordinator"},
			{AgentID: "w1", SortOrder: 20},
			{AgentID: "w2", SortOrder: 30},
			{AgentID: "report", SortOrder: 90, Role: "synthesizer"},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range cfg.Edges {
		if e.From == e.To {
			t.Fatalf("self-loop edge %q -> %q", e.From, e.To)
		}
	}
}

func TestCompileToGraphBuildConfig_criticLoop(t *testing.T) {
	def := Definition{
		Mode: "critic_loop",
		Members: []MemberDef{
			{AgentID: "gen", SortOrder: 1, Role: "generator"},
			{AgentID: "crit", SortOrder: 2, Role: "critic"},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ConditionalEdges) != 1 {
		t.Fatalf("cond=%d want 1", len(cfg.ConditionalEdges))
	}
	if cfg.ConditionalEdges[0].PathMap["retry"] != "member-1" {
		t.Fatalf("retry path=%q", cfg.ConditionalEdges[0].PathMap["retry"])
	}
	// approved 必须路由到终止哨兵 __end__；映射到 critic 节点会造成自循环，
	// 图永远不结束（critic_loop 不收敛 bug 的回归守卫）。
	if cfg.ConditionalEdges[0].PathMap["approved"] != biz.EndNodeID {
		t.Fatalf("approved path=%q want %q", cfg.ConditionalEdges[0].PathMap["approved"], biz.EndNodeID)
	}
	// 未配置 CriticLoop 时默认 maxIterations=3（死配置修复：迭代上限必须接线）。
	// ref 编码 critic 节点 ID（finish=member-2），cond func 按节点隔离轮次。
	if want := biz.CriticLoopCondFuncRefForNode(0, 3, "member-2"); cfg.ConditionalEdges[0].CondFuncRef != want {
		t.Fatalf("cond ref=%q want %q", cfg.ConditionalEdges[0].CondFuncRef, want)
	}
	// 显式步数天花板：(maxIter+2)*(nodes+1) = (3+2)*(2+1) = 15，
	// 失控图在贴近预期迭代数处截断，而非框架默认 100。
	if cfg.MaxSteps != 15 {
		t.Fatalf("MaxSteps=%d want 15", cfg.MaxSteps)
	}
}

func TestCompileToGraphBuildConfig_criticLoopExplicitConfig(t *testing.T) {
	def := Definition{
		Mode:       "critic_loop",
		CriticLoop: &CriticLoopConfig{MaxIterations: 5, ScoreThreshold: 0.8},
		Members: []MemberDef{
			{AgentID: "gen", SortOrder: 1, Role: "generator"},
			{AgentID: "crit", SortOrder: 2, Role: "critic"},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ConditionalEdges) != 1 {
		t.Fatalf("cond=%d want 1", len(cfg.ConditionalEdges))
	}
	if cfg.ConditionalEdges[0].PathMap["approved"] != biz.EndNodeID {
		t.Fatalf("approved path=%q want %q", cfg.ConditionalEdges[0].PathMap["approved"], biz.EndNodeID)
	}
	if want := biz.CriticLoopCondFuncRefForNode(0.8, 5, "member-2"); cfg.ConditionalEdges[0].CondFuncRef != want {
		t.Fatalf("cond ref=%q want %q", cfg.ConditionalEdges[0].CondFuncRef, want)
	}
	// (5+2)*(2+1) = 21
	if cfg.MaxSteps != 21 {
		t.Fatalf("MaxSteps=%d want 21", cfg.MaxSteps)
	}
}

func TestCompileToGraphBuildConfig_adaptive(t *testing.T) {
	def := Definition{
		Mode: "adaptive",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1},
			{AgentID: "b", SortOrder: 2},
		},
	}
	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Edges) != 2 {
		t.Fatalf("edges=%d want 2", len(cfg.Edges))
	}
	transferCount := 0
	for _, e := range cfg.Edges {
		if e.Kind == "transfer" {
			transferCount++
		}
	}
	if transferCount != 1 {
		t.Fatalf("transfer edges=%d want 1", transferCount)
	}
	if CompileTemplateID("swarm") != "dispatch" {
		t.Fatalf("swarm template=%q", CompileTemplateID("swarm"))
	}
}

func TestCompileToGraphBuildConfig_noMembers(t *testing.T) {
	_, _, err := CompileToGraphBuildConfig(Definition{Mode: "sequential"}, nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error")
	}
}
