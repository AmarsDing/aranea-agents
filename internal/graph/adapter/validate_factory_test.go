package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"
)

func TestFactory_Validate_EmptyGraphIsCompileFailure(t *testing.T) {
	t.Parallel()
	factory := NewGraphBuilderFactory(graphtrpc.NewRegistry(), nil, nil, nil, nil, graphtrpc.GraphNodeResolverSet{}, nil, loggateway.NewNoop(), nil)
	result, err := factory.Validate(context.Background(), biz.GraphBuildConfig{})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if result == nil || !result.HasErrors() {
		t.Fatal("empty graph must be a validation/compile failure, not a silent success")
	}
	found := false
	for _, e := range result.Errors {
		if e.Code == string(graphtrpc.ValidationErrEmptyGraph) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want empty_graph error, got %+v", result.Errors)
	}
}

func TestFactory_Validate_MissingAgentIsCompileFailure(t *testing.T) {
	t.Parallel()
	factory := NewGraphBuilderFactory(
		graphtrpc.NewRegistry(), nil, nil, nil,
		func(context.Context, string) bool { return false },
		graphtrpc.GraphNodeResolverSet{}, nil, loggateway.NewNoop(), nil,
	)
	result, err := factory.Validate(context.Background(), biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "missing"}},
		EntryPoint:  "n1",
		FinishPoint: "n1",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if result == nil || !result.HasErrors() {
		t.Fatal("missing agent must fail validation")
	}
	found := false
	for _, e := range result.Errors {
		if e.Code == string(graphtrpc.ValidationErrAgentNotFound) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want agent_not_found, got %+v", result.Errors)
	}
}

func TestValidateBizGraphBuildConfig_EmptyGraph(t *testing.T) {
	t.Parallel()
	result := ValidateBizGraphBuildConfig(context.Background(), biz.GraphBuildConfig{}, nil)
	if result == nil || !result.HasErrors() {
		t.Fatal("empty graph must fail")
	}
}

// TestFactory_Validate_ParameterizedCriticLoopRefAccepted is the regression
// guard for the standalone-graph path (graph_definitions): a parameterized
// critic_loop CondFuncRef (critic_loop#<maxIter>%<nodeID>) must validate —
// Validate registers cfg-scoped refs via EnsureCriticLoopCondFuncs before
// running the validator, mirroring BuildTeamGraphRoot / buildRuntime.
func TestFactory_Validate_ParameterizedCriticLoopRefAccepted(t *testing.T) {
	t.Parallel()
	factory := NewGraphBuilderFactory(graphtrpc.NewRegistry(), nil, nil, nil, nil, graphtrpc.GraphNodeResolverSet{}, nil, loggateway.NewNoop(), nil)
	ref := biz.CriticLoopCondFuncRefForNode(0, 2, "verify")
	result, err := factory.Validate(context.Background(), biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "worker", Type: biz.NodeTypeAgent},
			{ID: "verify", Type: biz.NodeTypeAgent},
		},
		Edges: []biz.EdgeDef{{From: "worker", To: "verify"}},
		ConditionalEdges: []biz.ConditionalEdgeDef{{
			From:        "verify",
			CondFuncRef: ref,
			PathMap: map[string]string{
				"approved":                         biz.EndNodeID,
				"retry":                            "worker",
				biz.CriticLoopResultApprovedForced: biz.EndNodeID,
			},
		}},
		EntryPoint:  "worker",
		FinishPoint: "verify",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	for _, e := range result.Errors {
		if e.Code == string(graphtrpc.ValidationErrCondFuncNotFound) {
			t.Fatalf("parameterized critic_loop ref %q must resolve, got %+v", ref, result.Errors)
		}
	}
}

func TestFactory_AgentExists_NilChecker(t *testing.T) {
	t.Parallel()
	factory := NewGraphBuilderFactory(graphtrpc.NewRegistry(), nil, nil, nil, nil, graphtrpc.GraphNodeResolverSet{}, nil, loggateway.NewNoop(), nil)
	if factory.AgentExists(context.Background(), "any") {
		t.Fatal("nil checker must report agent missing")
	}
}

func TestHasDeliverableStateField(t *testing.T) {
	t.Parallel()
	if hasDeliverableStateField(biz.GraphBuildConfig{}) {
		t.Fatal("empty schema has no deliverable field")
	}
	cfg := biz.GraphBuildConfig{StateFields: []biz.StateFieldDef{{Name: biz.DeliverableStateKey}}}
	if !hasDeliverableStateField(cfg) {
		t.Fatal("expected deliverable field")
	}
}
