package orchestrator

import (
	"fmt"

	"aranea-agents/internal/biz"
)

// VerificationType defines the type of verification gate.
type VerificationType string

const (
	VerifyTypeOutputFormat   VerificationType = "output_format"
	VerifyTypeTaskCompletion VerificationType = "task_completion"
	VerifyTypeHumanApproval  VerificationType = "human_approval"
)

// VerificationConfig defines a verification gate to inject into the graph.
type VerificationConfig struct {
	Type          VerificationType
	AfterNode     string // Insert verification after this node
	FailureAction string // "skip" | "retry_then_block" | "interrupt_before"
}

// DefaultVerificationConfigs returns the default verification gates based on mode.
func DefaultVerificationConfigs(mode string) []VerificationConfig {
	switch mode {
	case "parallel", "hybrid":
		return []VerificationConfig{
			{Type: VerifyTypeOutputFormat, AfterNode: "merge_results", FailureAction: biz.FailurePolicySkip},
			{Type: VerifyTypeTaskCompletion, AfterNode: "merge_results", FailureAction: biz.FailurePolicyRetryThenBlock},
		}
	case "coordinator":
		return []VerificationConfig{
			{Type: VerifyTypeOutputFormat, AfterNode: "merge_results", FailureAction: biz.FailurePolicySkip},
		}
	default:
		return nil
	}
}

// injectVerificationNodes adds verification nodes to the graph configuration.
// It finds edges going into the AfterNode, inserts a verification node between
// the source and AfterNode, and sets the verification node's properties based on FailureAction.
// Returns the IDs of the verification nodes that were added.
func injectVerificationNodes(config *biz.GraphBuildConfig, mode string) []string {
	configs := DefaultVerificationConfigs(mode)
	var nodeIDs []string

	for i, vc := range configs {
		nodeID := fmt.Sprintf("verify_%s_%d", vc.Type, i)

		// Build the verification node definition.
		vNode := biz.NodeDef{
			ID:            nodeID,
			Type:          biz.NodeTypeFunction,
			Description:   fmt.Sprintf("Verification gate: %s", vc.Type),
			FailureAction: vc.FailureAction,
		}

		// If FailureAction requires human approval, set interrupt flag.
		if vc.Type == VerifyTypeHumanApproval {
			vNode.InterruptBefore = true
		}

		// Find all edges whose To equals vc.AfterNode and rewire them
		// through the verification node: source → verify_node → AfterNode.
		var newEdges []biz.EdgeDef
		for _, edge := range config.Edges {
			if edge.To == vc.AfterNode {
				// Rewire: source → verification node
				newEdges = append(newEdges, biz.EdgeDef{From: edge.From, To: nodeID})
			} else {
				newEdges = append(newEdges, edge)
			}
		}
		// Add edge: verification node → AfterNode
		newEdges = append(newEdges, biz.EdgeDef{From: nodeID, To: vc.AfterNode})

		config.Edges = newEdges
		config.Nodes = append(config.Nodes, vNode)
		nodeIDs = append(nodeIDs, nodeID)
	}

	return nodeIDs
}
