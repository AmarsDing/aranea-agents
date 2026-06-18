package graph

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- Helpers ---

func newTestTopologyEvolver(llm trpcmodel.Model, bus *recordingReplanBus) *TopologyEvolverImpl {
	if bus == nil {
		bus = &recordingReplanBus{}
	}
	return &TopologyEvolverImpl{
		llm:        llm,
		eventBus:   bus,
		lg:         loggateway.NewNoop().With(loggateway.Domain("topology_evolver")),
		addedEdges: make(map[string]map[string]bool),
	}
}

func testEvolverExecution() *biz.GraphExecution {
	return biz.NewGraphExecution(
		context.Background(),
		"exec-evo-1",
		"graph-evo-1",
		"session-evo-1",
		"running",
	)
}

func testInsight() ExecutionInsight {
	return ExecutionInsight{
		SourceNode: "nodeA",
		TargetNode: "nodeC",
		Reason:     "nodeA output requires translation capability that nodeC provides",
		Evidence:   "nodeA output: 'please translate this document'",
	}
}

func edgeDecisionJSON(shouldAdd bool) string {
	if shouldAdd {
		return `{"should_add_edge": true, "reason": "edge is needed for translation capability"}`
	}
	return `{"should_add_edge": false, "reason": "edge is redundant"}`
}

// --- Tests ---

func TestTopologyEvolver_AddEdge(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("OnExecutionInsight failed: %v", err)
	}
	if edge == nil {
		t.Fatal("expected non-nil edge")
	}
	if edge.From != insight.SourceNode {
		t.Errorf("From=%q want %q", edge.From, insight.SourceNode)
	}
	if edge.To != insight.TargetNode {
		t.Errorf("To=%q want %q", edge.To, insight.TargetNode)
	}
	if edge.Kind != biz.EdgeKindTransfer {
		t.Errorf("Kind=%q want %q", edge.Kind, biz.EdgeKindTransfer)
	}

	// Verify event was published
	envelopes := bus.envelopes()
	if len(envelopes) != 1 {
		t.Fatalf("expected 1 published envelope, got %d", len(envelopes))
	}
	env := envelopes[0]
	if env.Type != contract.EnvelopeTypeGraphTopologyEvolved {
		t.Errorf("Type=%q want %q", env.Type, contract.EnvelopeTypeGraphTopologyEvolved)
	}
	if env.SessionID != exec.SessionID {
		t.Errorf("SessionID=%q want %q", env.SessionID, exec.SessionID)
	}
	if v, ok := env.Metadata["execution_id"].(string); !ok || v != exec.ID {
		t.Errorf("Metadata[execution_id]=%v want %q", env.Metadata["execution_id"], exec.ID)
	}
	if v, ok := env.Metadata["from_node"].(string); !ok || v != insight.SourceNode {
		t.Errorf("Metadata[from_node]=%v want %q", env.Metadata["from_node"], insight.SourceNode)
	}
	if v, ok := env.Metadata["to_node"].(string); !ok || v != insight.TargetNode {
		t.Errorf("Metadata[to_node]=%v want %q", env.Metadata["to_node"], insight.TargetNode)
	}
	if v, ok := env.Metadata["edge_kind"].(string); !ok || v != biz.EdgeKindTransfer {
		t.Errorf("Metadata[edge_kind]=%v want %q", env.Metadata["edge_kind"], biz.EdgeKindTransfer)
	}
}

func TestTopologyEvolver_SkipEdge(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(false))}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("OnExecutionInsight failed: %v", err)
	}
	if edge != nil {
		t.Errorf("expected nil edge when LLM decides not to add, got %+v", edge)
	}
	// No event should be published when edge is not added
	if len(bus.envelopes()) != 0 {
		t.Errorf("expected 0 envelopes, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_LLMFailure(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{err: errors.New("LLM service unavailable")}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("OnExecutionInsight should not return error on LLM failure (degrade gracefully), got: %v", err)
	}
	if edge != nil {
		t.Errorf("expected nil edge on LLM failure, got %+v", edge)
	}
	// No event should be published on LLM failure
	if len(bus.envelopes()) != 0 {
		t.Errorf("expected 0 envelopes on LLM failure, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_MalformedJSON(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse("this is not valid JSON")}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("OnExecutionInsight should not return error on malformed JSON (degrade gracefully), got: %v", err)
	}
	if edge != nil {
		t.Errorf("expected nil edge on malformed JSON, got %+v", edge)
	}
	if len(bus.envelopes()) != 0 {
		t.Errorf("expected 0 envelopes on malformed JSON, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_NilLLM(t *testing.T) {
	bus := &recordingReplanBus{}
	evolver := newTestTopologyEvolver(nil, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("OnExecutionInsight should not return error on nil LLM (degrade gracefully), got: %v", err)
	}
	if edge != nil {
		t.Errorf("expected nil edge on nil LLM, got %+v", edge)
	}
	if len(bus.envelopes()) != 0 {
		t.Errorf("expected 0 envelopes on nil LLM, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_SelfLoop(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := ExecutionInsight{
		SourceNode: "nodeA",
		TargetNode: "nodeA",
		Reason:     "self-loop test",
		Evidence:   "evidence",
	}

	edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err == nil {
		t.Fatal("expected error for self-loop edge, got nil")
	}
	if edge != nil {
		t.Errorf("expected nil edge on self-loop, got %+v", edge)
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
	if apiErr.Domain != apierror.DomainGraph {
		t.Errorf("Domain=%q want %q", apiErr.Domain, apierror.DomainGraph)
	}
	// No event should be published on validation error
	if len(bus.envelopes()) != 0 {
		t.Errorf("expected 0 envelopes on self-loop, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_DuplicateEdge(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()
	insight := testInsight()

	// First call should add the edge
	edge1, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("first OnExecutionInsight failed: %v", err)
	}
	if edge1 == nil {
		t.Fatal("expected non-nil edge on first call")
	}
	if len(bus.envelopes()) != 1 {
		t.Fatalf("expected 1 envelope after first call, got %d", len(bus.envelopes()))
	}

	// Second call with same insight should skip (duplicate)
	edge2, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
	if err != nil {
		t.Fatalf("second OnExecutionInsight failed: %v", err)
	}
	if edge2 != nil {
		t.Errorf("expected nil edge on duplicate, got %+v", edge2)
	}
	// No new event should be published for duplicate
	if len(bus.envelopes()) != 1 {
		t.Errorf("expected 1 envelope after duplicate call, got %d", len(bus.envelopes()))
	}
}

func TestTopologyEvolver_NilExec(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	insight := testInsight()

	edge, err := evolver.OnExecutionInsight(context.Background(), nil, insight)
	if err == nil {
		t.Fatal("expected error for nil execution, got nil")
	}
	if edge != nil {
		t.Errorf("expected nil edge on nil execution, got %+v", edge)
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
	if apiErr.Domain != apierror.DomainGraph {
		t.Errorf("Domain=%q want %q", apiErr.Domain, apierror.DomainGraph)
	}
}

func TestTopologyEvolver_EmptyNodes(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	exec := testEvolverExecution()

	tests := []struct {
		name   string
		source string
		target string
	}{
		{"empty source", "", "nodeC"},
		{"empty target", "nodeA", ""},
		{"both empty", "", ""},
		{"whitespace source", "  ", "nodeC"},
		{"whitespace target", "nodeA", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insight := ExecutionInsight{
				SourceNode: tt.source,
				TargetNode: tt.target,
				Reason:     "test",
				Evidence:   "evidence",
			}
			edge, err := evolver.OnExecutionInsight(context.Background(), exec, insight)
			if err == nil {
				t.Fatal("expected error for empty nodes, got nil")
			}
			if edge != nil {
				t.Errorf("expected nil edge on empty nodes, got %+v", edge)
			}
			apiErr, ok := err.(*apierror.Error)
			if !ok {
				t.Fatalf("expected *apierror.Error, got %T", err)
			}
			if apiErr.Code != apierror.CodeBadRequest {
				t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
			}
		})
	}
}

// TestTopologyEvolver_PerExecutionIsolation verifies that the duplicate-edge
// tracking is isolated per execution ID (BD5 concurrency test).
func TestTopologyEvolver_PerExecutionIsolation(t *testing.T) {
	bus := &recordingReplanBus{}
	model := &fakeNL2GraphModel{responses: singleResponse(edgeDecisionJSON(true))}
	evolver := newTestTopologyEvolver(model, bus)
	insight := testInsight()

	// exec1 adds the edge
	exec1 := testEvolverExecution()
	edge1, err := evolver.OnExecutionInsight(context.Background(), exec1, insight)
	if err != nil {
		t.Fatalf("exec1 OnExecutionInsight failed: %v", err)
	}
	if edge1 == nil {
		t.Fatal("exec1 expected non-nil edge")
	}

	// exec2 with same insight should also add (different execution, not duplicate)
	exec2 := biz.NewGraphExecution(context.Background(), "exec-evo-2", "graph-evo-1", "session-evo-2", "running")
	edge2, err := evolver.OnExecutionInsight(context.Background(), exec2, insight)
	if err != nil {
		t.Fatalf("exec2 OnExecutionInsight failed: %v", err)
	}
	if edge2 == nil {
		t.Error("exec2 expected non-nil edge (different execution should not be duplicate)")
	}

	// Both events should be published
	if len(bus.envelopes()) != 2 {
		t.Errorf("expected 2 envelopes, got %d", len(bus.envelopes()))
	}
}
