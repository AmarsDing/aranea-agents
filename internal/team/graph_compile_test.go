package team

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// S9（K3 降级覆盖）：未知编排模式静默 fallback 为 pipeline，此前无任何日志。
// 修复后必须发射 warn 流程日志（team.compile.unknown_mode_fallback），让业务
// 用户在「流程日志」Tab 感知降级。
func TestGenerateModeEdges_UnknownModeFallbackEmitsWarn(t *testing.T) {
	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID: "tr-s9",
		Domain:  event.TraceDomainTeam,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	nodes := []embeddedGraphNode{
		{ID: "start", Type: "start"},
		{ID: "member-0", Type: "agent"},
		{ID: "member-1", Type: "agent"},
		{ID: "end", Type: "end"},
	}
	// "squenctial" 是拼写错误的未知模式，应 fallback 到 pipeline 拓扑。
	edges := generateModeEdges(ctx, "squenctial", Definition{}, nodes, loggateway.NewNoop())

	// pipeline 拓扑：member-0 → member-1 链式边（无 transfer 边）。
	for _, e := range edges {
		if e.Label == "transfer" {
			t.Fatalf("unknown mode must fall back to pipeline (no transfer edges), got %+v", edges)
		}
	}

	foundWarn := false
	for _, ev := range monBus.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		step, _ := ev.Metadata["step_id"].(string)
		sev, _ := ev.Metadata["severity"].(string)
		if step == "team.compile.unknown_mode_fallback" && sev == "warn" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("expected warn flow-log for unknown mode fallback, got %+v", monBus.evs)
	}
}

// 合法模式不得触发降级日志。
func TestGenerateModeEdges_KnownModeNoFallbackWarn(t *testing.T) {
	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID: "tr-s9b",
		Domain:  event.TraceDomainTeam,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	nodes := []embeddedGraphNode{
		{ID: "start", Type: "start"},
		{ID: "member-0", Type: "agent"},
		{ID: "end", Type: "end"},
	}
	generateModeEdges(ctx, "sequential", Definition{}, nodes, loggateway.NewNoop())

	for _, ev := range monBus.evs {
		step, _ := ev.Metadata["step_id"].(string)
		if step == "team.compile.unknown_mode_fallback" {
			t.Fatal("known mode must not emit fallback warn")
		}
	}
}

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
	spec := generateGraphSpecFromMode(context.Background(), def, "parallel", loggateway.NewNoop())
	if spec == nil {
		t.Fatal("spec nil")
	}
	fromStart := map[string]bool{}
	serialPrefix := false
	for _, e := range spec.Edges {
		if e.Source == "start" {
			fromStart[e.Target] = true
		}
		if e.Source == "member-1" && e.Target == "member-2" {
			serialPrefix = true
		}
	}
	if !fromStart["member-1"] || !fromStart["member-2"] {
		t.Fatalf("start must fan out to both workers, got %v", fromStart)
	}
	if fromStart["member-3"] {
		t.Fatal("start should not skip workers to synthesizer")
	}
	if serialPrefix {
		t.Fatal("parallel must not use first worker as serial prefix")
	}

	cfg, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FinishPoint != "member-3" {
		t.Fatalf("finish=%q want member-3", cfg.FinishPoint)
	}
	join := map[string]bool{}
	for _, e := range cfg.Edges {
		if e.To == "member-3" {
			join[e.From] = true
		}
		if e.From == "member-1" && e.To == "member-2" {
			t.Fatal("compiled edges must not serialize workers")
		}
	}
	if !join["member-1"] || !join["member-2"] {
		t.Fatalf("both workers must join synthesizer, got %v", join)
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
	// approved_forced（迭代上限兜底收敛）同样终止图，仅作观测区分。
	if cfg.ConditionalEdges[0].PathMap[biz.CriticLoopResultApprovedForced] != biz.EndNodeID {
		t.Fatalf("approved_forced path=%q want %q", cfg.ConditionalEdges[0].PathMap[biz.CriticLoopResultApprovedForced], biz.EndNodeID)
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

// ADR-08 A2：模板路径下 DefinitionGraphSpecJSON 输出后端权威内嵌图（含 start/end 装饰节点），
// 前端以此为准，废弃本地 mode→graph 生成逻辑。
func TestDefinitionGraphSpecJSON_templatePath(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a1", Role: "worker", SortOrder: 1},
			{AgentID: "a2", Role: "worker", SortOrder: 2},
		},
	}
	raw := `{"mode":"sequential","members":[{"agent_id":"a1","role":"worker","sort_order":1},{"agent_id":"a2","role":"worker","sort_order":2}]}`
	out := DefinitionGraphSpecJSON(context.Background(), def, raw, loggateway.NewNoop())
	if out == "" {
		t.Fatal("expected template-generated spec JSON")
	}
	var spec struct {
		Layout string `json:"layout"`
		Nodes  []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			AgentID string `json:"agent_id"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(out), &spec); err != nil {
		t.Fatalf("spec not JSON: %v", err)
	}
	if spec.Layout != "sequential" {
		t.Fatalf("layout=%q want sequential", spec.Layout)
	}
	if len(spec.Nodes) != 4 || spec.Nodes[0].ID != "start" || spec.Nodes[3].ID != "end" {
		t.Fatalf("nodes=%+v want start/member-1/member-2/end", spec.Nodes)
	}
	if spec.Nodes[1].AgentID != "a1" || spec.Nodes[2].AgentID != "a2" {
		t.Fatalf("agent ids=%q/%q", spec.Nodes[1].AgentID, spec.Nodes[2].AgentID)
	}
	if len(spec.Edges) != 3 {
		t.Fatalf("edges=%d want 3 (start->m1, m1->m2, m2->end)", len(spec.Edges))
	}
	if spec.Edges[0].Source != "start" || spec.Edges[2].Target != "end" {
		t.Fatalf("decor edges missing: %+v", spec.Edges)
	}
}

// 定义自带 embedded graph（custom 路径）时不输出——前端已有该图，无需后端重发。
func TestDefinitionGraphSpecJSON_embeddedPathEmpty(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a1", Role: "worker", SortOrder: 1},
		},
	}
	raw := `{"mode":"sequential","members":[{"agent_id":"a1","role":"worker","sort_order":1}],"graph":{"version":1,"layout":"custom","nodes":[{"id":"start","type":"start"},{"id":"n1","type":"agent","agent_id":"a1"},{"id":"end","type":"end"}],"edges":[{"source":"start","target":"n1"},{"source":"n1","target":"end"}]}}`
	if out := DefinitionGraphSpecJSON(context.Background(), def, raw, loggateway.NewNoop()); out != "" {
		t.Fatalf("embedded path must not emit spec, got %q", out)
	}
}

func TestDefinitionGraphSpecJSON_noMembers(t *testing.T) {
	if out := DefinitionGraphSpecJSON(context.Background(), Definition{Mode: "sequential"}, `{"mode":"sequential"}`, loggateway.NewNoop()); out != "" {
		t.Fatalf("no members must not emit spec, got %q", out)
	}
}

func TestBuildRoleManifest_UsesRequiredRoleNotNodeType(t *testing.T) {
	got := buildRoleManifest(biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "n1", Type: "agent", AgentName: "key-a", Description: "Alpha", RequiredRole: biz.RoleCoordinator},
			{ID: "n2", Type: "agent", AgentName: "key-b"},
			{ID: "join", Type: "join"},
		},
	})
	if got["n1"].Role != biz.RoleCoordinator || got["n1"].DisplayName != "Alpha" {
		t.Fatalf("n1=%+v", got["n1"])
	}
	if got["n2"].Role != biz.RoleWorker || got["n2"].DisplayName != "key-b" {
		t.Fatalf("n2=%+v", got["n2"])
	}
	if _, ok := got["join"]; ok {
		t.Fatal("join node must not appear in role manifest")
	}
}

func TestCompileToCompiledTeam_LinkedGraph_RebuildsTaskMeta(t *testing.T) {
	loader := stubGraphLoader{configs: map[string]biz.GraphBuildConfig{
		"g-linked": {
			Nodes: []biz.NodeDef{
				{ID: "agent-1", Type: "agent", AgentName: "worker-1", Description: "Worker A", RequiredRole: biz.RoleWorker},
				{ID: "task-1", Type: "task", AgentName: "reviewer-agent", Description: "Human review", RequiredRole: biz.RoleCritic, AssignmentMode: "dynamic"},
			},
			EntryPoint:  "agent-1",
			FinishPoint: "task-1",
		},
	}}
	def := Definition{Mode: "sequential", Members: []MemberDef{{AgentID: "a1", Role: "worker", SortOrder: 1}}}
	raw := `{"linked_graph_id":"g-linked","mode":"sequential","members":[{"agent_id":"a1","sort_order":1}]}`
	ct, err := CompileToCompiledTeam(context.Background(), def, raw, nil, loader, loggateway.NewNoop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := ct.TaskMeta["task-1"]
	if !ok || meta.RequiredRole != biz.RoleCritic || meta.AssignmentMode != "dynamic" {
		t.Fatalf("taskMeta=%+v", ct.TaskMeta)
	}
	if _, ok := ct.TaskMeta["agent-1"]; ok {
		t.Fatal("agent nodes must not get team-graph task meta")
	}
	role := ct.RoleManifest["agent-1"]
	if role.Role != biz.RoleWorker || role.DisplayName != "Worker A" {
		t.Fatalf("agent role=%+v", role)
	}
	if ct.RoleManifest["task-1"].Role != biz.RoleCritic {
		t.Fatalf("task role=%+v", ct.RoleManifest["task-1"])
	}
}
