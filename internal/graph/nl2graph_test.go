package graph

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- Fakes ---

// fakeNL2GraphModel implements trpcmodel.Model for NL2Graph tests.
type fakeNL2GraphModel struct {
	responses []*trpcmodel.Response
	err       error // function-level error (returned by GenerateContent)
}

func (m *fakeNL2GraphModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, len(m.responses))
	for _, r := range m.responses {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *fakeNL2GraphModel) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: "fake-nl2graph-model"}
}

// --- Helpers ---

func newTestNL2GraphConverter(model trpcmodel.Model) *NL2GraphConverterImpl {
	return &NL2GraphConverterImpl{
		llm: model,
		lg:  loggateway.NewNoop().With(loggateway.Domain("nl2graph")),
	}
}

func sequentialJSONResponse() string {
	return `{
        "subtasks": [
            {"id": "step1", "description": "Research the topic", "depends_on": [], "required_role": "researcher", "required_domain": "search"},
            {"id": "step2", "description": "Write the report", "depends_on": ["step1"], "required_role": "writer", "required_domain": "writing"},
            {"id": "step3", "description": "Review the report", "depends_on": ["step2"], "required_role": "reviewer", "required_domain": "review"}
        ],
        "template": "sequential",
        "entry_point": "step1",
        "finish_point": "step3"
    }`
}

func parallelJSONResponse() string {
	return `{
        "subtasks": [
            {"id": "step1", "description": "Research topic A", "depends_on": [], "required_role": "researcher", "required_domain": "search"},
            {"id": "step2", "description": "Research topic B", "depends_on": [], "required_role": "researcher", "required_domain": "search"},
            {"id": "step3", "description": "Research topic C", "depends_on": [], "required_role": "researcher", "required_domain": "search"}
        ],
        "template": "parallel",
        "entry_point": "step1",
        "finish_point": "step3"
    }`
}

func dagJSONResponse() string {
	return `{
        "subtasks": [
            {"id": "step1", "description": "Gather data", "depends_on": [], "required_role": "researcher", "required_domain": "search"},
            {"id": "step2", "description": "Process data A", "depends_on": ["step1"], "required_role": "processor", "required_domain": "processing"},
            {"id": "step3", "description": "Process data B", "depends_on": ["step1"], "required_role": "processor", "required_domain": "processing"},
            {"id": "step4", "description": "Merge results", "depends_on": ["step2", "step3"], "required_role": "synthesizer", "required_domain": "synthesis"}
        ],
        "template": "dag",
        "entry_point": "step1",
        "finish_point": "step4"
    }`
}

func cyclicJSONResponse() string {
	return `{
        "subtasks": [
            {"id": "step1", "description": "Task A", "depends_on": ["step3"], "required_role": "researcher", "required_domain": "search"},
            {"id": "step2", "description": "Task B", "depends_on": ["step1"], "required_role": "writer", "required_domain": "writing"},
            {"id": "step3", "description": "Task C", "depends_on": ["step2"], "required_role": "reviewer", "required_domain": "review"}
        ],
        "template": "dag",
        "entry_point": "step1",
        "finish_point": "step3"
    }`
}

func testAgents() []biz.AgentCapability {
	return []biz.AgentCapability{
		{AgentKey: "researcher-1", DisplayName: "Researcher", Roles: []string{"researcher"}, Domains: []string{"search"}},
		{AgentKey: "writer-1", DisplayName: "Writer", Roles: []string{"writer"}, Domains: []string{"writing"}},
		{AgentKey: "reviewer-1", DisplayName: "Reviewer", Roles: []string{"reviewer"}, Domains: []string{"review"}},
		{AgentKey: "processor-1", DisplayName: "Processor", Roles: []string{"processor"}, Domains: []string{"processing"}},
		{AgentKey: "synthesizer-1", DisplayName: "Synthesizer", Roles: []string{"synthesizer"}, Domains: []string{"synthesis"}},
	}
}

func singleResponse(content string) []*trpcmodel.Response {
	return []*trpcmodel.Response{
		{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: content}}}},
	}
}

// --- Tests ---

func TestNL2GraphConverter_Convert_Sequential(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse(sequentialJSONResponse())}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Research and write a report", testAgents())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Verify 3 nodes
	if len(cfg.Nodes) != 3 {
		t.Errorf("Nodes count=%d want 3", len(cfg.Nodes))
	}

	// Verify edges form linear chain: step1→step2→step3
	if len(cfg.Edges) != 2 {
		t.Errorf("Edges count=%d want 2", len(cfg.Edges))
	}

	// Verify entry/finish points
	if cfg.EntryPoint != "step1" {
		t.Errorf("EntryPoint=%q want %q", cfg.EntryPoint, "step1")
	}
	if cfg.FinishPoint != "step3" {
		t.Errorf("FinishPoint=%q want %q", cfg.FinishPoint, "step3")
	}

	// Verify engine is BSP (sequential uses BSP)
	if cfg.ExecutionEngine != biz.EngineBSP {
		t.Errorf("ExecutionEngine=%q want %q", cfg.ExecutionEngine, biz.EngineBSP)
	}

	// Verify agents matched by role
	if cfg.Nodes[0].AgentName != "researcher-1" {
		t.Errorf("Node[0] AgentName=%q want %q", cfg.Nodes[0].AgentName, "researcher-1")
	}
	if cfg.Nodes[1].AgentName != "writer-1" {
		t.Errorf("Node[1] AgentName=%q want %q", cfg.Nodes[1].AgentName, "writer-1")
	}
	if cfg.Nodes[2].AgentName != "reviewer-1" {
		t.Errorf("Node[2] AgentName=%q want %q", cfg.Nodes[2].AgentName, "reviewer-1")
	}

	// Verify node type and func_ref
	for i, n := range cfg.Nodes {
		if n.Type != biz.NodeTypeAgent {
			t.Errorf("Node[%d] Type=%q want %q", i, n.Type, biz.NodeTypeAgent)
		}
		if n.FuncRef != "agent.invoke" {
			t.Errorf("Node[%d] FuncRef=%q want %q", i, n.FuncRef, "agent.invoke")
		}
	}
}

func TestNL2GraphConverter_Convert_Parallel(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse(parallelJSONResponse())}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Research multiple topics", testAgents())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Verify 3 nodes
	if len(cfg.Nodes) != 3 {
		t.Errorf("Nodes count=%d want 3", len(cfg.Nodes))
	}

	// No dependencies → no edges
	if len(cfg.Edges) != 0 {
		t.Errorf("Edges count=%d want 0 (parallel has no deps)", len(cfg.Edges))
	}

	// Verify engine is BSP (parallel uses BSP)
	if cfg.ExecutionEngine != biz.EngineBSP {
		t.Errorf("ExecutionEngine=%q want %q", cfg.ExecutionEngine, biz.EngineBSP)
	}
}

func TestNL2GraphConverter_Convert_DAG(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse(dagJSONResponse())}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Complex data processing", testAgents())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Verify 4 nodes
	if len(cfg.Nodes) != 4 {
		t.Errorf("Nodes count=%d want 4", len(cfg.Nodes))
	}

	// Verify edges: step1→step2, step1→step3, step2→step4, step3→step4 = 4 edges
	if len(cfg.Edges) != 4 {
		t.Errorf("Edges count=%d want 4", len(cfg.Edges))
	}

	// Verify engine is DAG
	if cfg.ExecutionEngine != biz.EngineDAG {
		t.Errorf("ExecutionEngine=%q want %q", cfg.ExecutionEngine, biz.EngineDAG)
	}

	// Verify entry/finish points
	if cfg.EntryPoint != "step1" {
		t.Errorf("EntryPoint=%q want %q", cfg.EntryPoint, "step1")
	}
	if cfg.FinishPoint != "step4" {
		t.Errorf("FinishPoint=%q want %q", cfg.FinishPoint, "step4")
	}
}

func TestNL2GraphConverter_Convert_LLMFailure(t *testing.T) {
	model := &fakeNL2GraphModel{err: errors.New("LLM service unavailable")}
	converter := newTestNL2GraphConverter(model)

	_, err := converter.Convert(context.Background(), "Some task", testAgents())
	if err == nil {
		t.Fatal("expected error for LLM failure, got nil")
	}

	// Verify it's an apierror.Internal
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeInternal {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeInternal)
	}
	if apiErr.Domain != apierror.DomainGraph {
		t.Errorf("Domain=%q want %q", apiErr.Domain, apierror.DomainGraph)
	}
}

func TestNL2GraphConverter_Convert_MalformedJSON(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse("this is not JSON")}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Some task", testAgents())
	if err != nil {
		t.Fatalf("Convert failed on malformed JSON (should use fallback): %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Should fall back to sequential with at least one node
	if len(cfg.Nodes) == 0 {
		t.Error("Nodes is empty (fallback should provide at least one node)")
	}

	// Verify engine is BSP (sequential fallback)
	if cfg.ExecutionEngine != biz.EngineBSP {
		t.Errorf("ExecutionEngine=%q want %q (sequential fallback)", cfg.ExecutionEngine, biz.EngineBSP)
	}

	// Verify entry point is set
	if cfg.EntryPoint == "" {
		t.Error("EntryPoint is empty (fallback should set entry point)")
	}
}

func TestNL2GraphConverter_Convert_NoAgents(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse(sequentialJSONResponse())}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Some task", nil)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Verify nodes have empty AgentName when no agents available
	for i, n := range cfg.Nodes {
		if n.AgentName != "" {
			t.Errorf("Node[%d] AgentName=%q want empty (no agents available)", i, n.AgentName)
		}
	}
}

func TestNL2GraphConverter_Convert_CycleDetection(t *testing.T) {
	model := &fakeNL2GraphModel{responses: singleResponse(cyclicJSONResponse())}
	converter := newTestNL2GraphConverter(model)

	cfg, err := converter.Convert(context.Background(), "Cyclic task", testAgents())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}

	// Should fall back to sequential (linear chain)
	// Verify edges form linear chain (no cycles)
	if len(cfg.Nodes) > 1 {
		// Sequential fallback should have len(nodes)-1 edges
		expectedEdges := len(cfg.Nodes) - 1
		if len(cfg.Edges) != expectedEdges {
			t.Errorf("Edges count=%d want %d (sequential fallback)", len(cfg.Edges), expectedEdges)
		}
	}

	// Verify engine is BSP (sequential fallback)
	if cfg.ExecutionEngine != biz.EngineBSP {
		t.Errorf("ExecutionEngine=%q want %q (sequential fallback)", cfg.ExecutionEngine, biz.EngineBSP)
	}

	// Verify no cycles in the fallback (each edge goes forward in node order)
	nodeOrder := make(map[string]int, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		nodeOrder[n.ID] = i
	}
	for _, e := range cfg.Edges {
		fromIdx, fromOk := nodeOrder[e.From]
		toIdx, toOk := nodeOrder[e.To]
		if !fromOk || !toOk {
			t.Errorf("Edge %q→%q references unknown node", e.From, e.To)
			continue
		}
		if fromIdx >= toIdx {
			t.Errorf("Edge %q→%q is not forward (from index %d >= to index %d)",
				e.From, e.To, fromIdx, toIdx)
		}
	}
}

func TestNL2GraphConverter_Convert_EmptyTaskDesc(t *testing.T) {
	model := &fakeNL2GraphModel{}
	converter := newTestNL2GraphConverter(model)

	_, err := converter.Convert(context.Background(), "  ", testAgents())
	if err == nil {
		t.Fatal("expected error for empty task description, got nil")
	}

	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
}

func TestNL2GraphConverter_Convert_NilLLM(t *testing.T) {
	converter := newTestNL2GraphConverter(nil)

	_, err := converter.Convert(context.Background(), "Some task", testAgents())
	if err == nil {
		t.Fatal("expected error for nil LLM, got nil")
	}

	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeInternal {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeInternal)
	}
}
