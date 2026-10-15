package adapter

import (
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

func convertGraphTemplate(t graphtrpc.GraphTemplate) biz.GraphTemplateRef {
	nodes := make([]biz.GraphTemplateNodeRef, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = biz.GraphTemplateNodeRef{
			NodeID: n.NodeID, Type: n.Type, Label: n.Label, Description: n.Description,
		}
	}
	edges := make([]biz.GraphTemplateEdgeRef, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = biz.GraphTemplateEdgeRef{
			FromNode: e.FromNode, ToNode: e.ToNode, Type: e.Type, Label: e.Label,
		}
	}
	stateFields := make([]biz.StateFieldDef, len(t.StateFields))
	copy(stateFields, t.StateFields)
	return biz.GraphTemplateRef{
		ID: t.ID, Name: t.Name, Description: t.Description, Category: t.Category,
		Nodes: nodes, Edges: edges, StateFields: stateFields,
		EntryPoint: t.EntryPoint, FinishPoint: t.FinishPoint,
	}
}

// revertGraphTemplate converts a biz GraphTemplateRef back to the trpc layer
// GraphTemplate so it can be fed into graphtrpc.TemplateToBuildConfig.
func revertGraphTemplate(t biz.GraphTemplateRef) graphtrpc.GraphTemplate {
	nodes := make([]graphtrpc.TemplateNode, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = graphtrpc.TemplateNode{
			NodeID: n.NodeID, Type: n.Type, Label: n.Label, Description: n.Description,
		}
	}
	edges := make([]graphtrpc.TemplateEdge, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = graphtrpc.TemplateEdge{
			FromNode: e.FromNode, ToNode: e.ToNode, Type: e.Type, Label: e.Label,
		}
	}
	stateFields := make([]graphtrpc.StateFieldDef, len(t.StateFields))
	copy(stateFields, t.StateFields)
	return graphtrpc.GraphTemplate{
		ID: t.ID, Name: t.Name, Description: t.Description, Category: t.Category,
		Nodes: nodes, Edges: edges, StateFields: stateFields,
		EntryPoint: t.EntryPoint, FinishPoint: t.FinishPoint,
	}
}

func convertRawEvent(e *trpcevent.Event) biz.GraphRawEvent {
	if e == nil {
		return biz.GraphRawEvent{}
	}
	return biz.GraphRawEvent{
		Object: e.Object,
	}
}
