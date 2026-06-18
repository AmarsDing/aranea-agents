package team

// TECH-DEBT(P2-18): ExportSnapshot 适配器已实现但未接入生产路径。
// 项目 Team 编排当前不使用框架 structure.Snapshot 导出能力，此适配器
// 保留作为未来 Team 可视化/diff/版本控制需求的桥接点
// （alignment-plan.md §四 协同包 C）。

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/structure"
)

// ExportSnapshot converts a project biz.Agent (team definition) into a
// framework structure.Snapshot. It builds the team's member agents on the fly
// using the provided memberAgents, exports each member via the framework's
// LLMAgent.Export, and assembles the full snapshot with edges.
//
// This adapter bridges the project's team definition model (biz.Agent) to the
// framework's Export() structure format, enabling structure-level tooling
// (visualization, diff, version control) for team definitions.
//
// TECH-DEBT(P2-18): 未接入生产路径，见文件头说明。
//
// P2-18: The adapter re-implements the path allocation and rebasing logic
// locally (matching the framework's internal istructure package) because the
// framework's internal/structure package is not accessible from the main module.
func ExportSnapshot(
	ctx context.Context,
	def Definition,
	memberAgents []trpcagent.Agent,
	lg loggateway.Logger,
) (*structure.Snapshot, error) {
	if len(memberAgents) == 0 {
		return nil, fmt.Errorf("team export: no member agents")
	}

	rootNodeID := escapeLocalName(def.SynthesizerAgentID)
	snapshot := &structure.Snapshot{
		EntryNodeID: rootNodeID,
		Nodes: []structure.Node{
			{
				NodeID: rootNodeID,
				Kind:   structure.NodeKindAgent,
				Name:   def.SynthesizerAgentID,
			},
		},
	}

	allocator := newPathAllocator(rootNodeID)
	var exportChild structure.ChildExporter
	exportChild = func(_ context.Context, a trpcagent.Agent) (*structure.Snapshot, error) {
		if exporter, ok := a.(structure.Exporter); ok {
			return exporter.Export(ctx, exportChild)
		}
		// Fallback: produce a minimal node for agents that don't implement Exporter.
		childNodeID := allocator.next(a.Info().Name)
		return &structure.Snapshot{
			EntryNodeID: childNodeID,
			Nodes: []structure.Node{
				{NodeID: childNodeID, Kind: structure.NodeKindAgent, Name: a.Info().Name},
			},
			Surfaces: []structure.Surface{
				{
					NodeID: childNodeID,
					Type:   structure.SurfaceTypeInstruction,
					Value:  structure.SurfaceValue{Text: stringPtr(a.Info().Description)},
				},
			},
		}, nil
	}

	for _, member := range memberAgents {
		childSnapshot, err := exportChild(ctx, member)
		if err != nil {
			lg.Warn("Team Export: 成员导出失败",
				loggateway.StepID("team.export_member_fail"),
				loggateway.Str("member", member.Info().Name),
				loggateway.Err(err))
			continue
		}
		mountPath := allocator.next(member.Info().Name)
		rebased, err := rebaseSnapshot(childSnapshot, mountPath)
		if err != nil {
			return nil, fmt.Errorf("team export: rebase member %q: %w", member.Info().Name, err)
		}
		snapshot.Nodes = append(snapshot.Nodes, rebased.Nodes...)
		snapshot.Edges = append(snapshot.Edges, rebased.Edges...)
		snapshot.Surfaces = append(snapshot.Surfaces, rebased.Surfaces...)
		snapshot.Edges = append(snapshot.Edges, structure.Edge{
			FromNodeID: rootNodeID,
			ToNodeID:   rebased.EntryNodeID,
		})
	}

	return snapshot, nil
}

// ---------------------------------------------------------------------------
// Local re-implementation of framework internal structure utilities.
// These match the logic in pkg/trpc-agent-go/internal/structure/structure.go
// but are accessible from the main module.
// ---------------------------------------------------------------------------

// pathAllocator allocates stable child paths under one parent node.
type pathAllocator struct {
	parentNodeID string
	used         map[string]int
}

// newPathAllocator creates a new path allocator for one parent node.
func newPathAllocator(parentNodeID string) *pathAllocator {
	return &pathAllocator{
		parentNodeID: parentNodeID,
		used:         make(map[string]int),
	}
}

// next returns the next stable child path for the given local name.
func (a *pathAllocator) next(localName string) string {
	escaped := escapeLocalName(localName)
	count := a.used[escaped] + 1
	a.used[escaped] = count
	if count == 1 {
		return joinEscapedNodeID(a.parentNodeID, escaped)
	}
	return joinEscapedNodeID(a.parentNodeID, fmt.Sprintf("%s~%d", escaped, count))
}

// escapeLocalName escapes one path segment into a stable node-id segment.
// Matches framework's istructure.EscapeLocalName.
func escapeLocalName(name string) string {
	if name == "" {
		return "_"
	}
	replacer := strings.NewReplacer("~", "~0", "/", "~1")
	escaped := replacer.Replace(name)
	if escaped == "" {
		return "_"
	}
	return escaped
}

// joinEscapedNodeID joins a parent node id and an escaped local name.
func joinEscapedNodeID(parentNodeID string, escaped string) string {
	if parentNodeID == "" {
		return escaped
	}
	if escaped == "" {
		return parentNodeID
	}
	return parentNodeID + "/" + escaped
}

// rebaseSnapshot rewrites one snapshot to a new mounted root node id.
// Matches framework's istructure.RebaseSnapshot.
func rebaseSnapshot(
	snapshot *structure.Snapshot,
	newRootNodeID string,
) (*structure.Snapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}
	oldRoot := snapshot.EntryNodeID
	if oldRoot == "" {
		return nil, fmt.Errorf("entry node id is empty")
	}
	rebased := &structure.Snapshot{
		EntryNodeID: newRootNodeID,
		Nodes:       make([]structure.Node, 0, len(snapshot.Nodes)),
		Edges:       make([]structure.Edge, 0, len(snapshot.Edges)),
		Surfaces:    make([]structure.Surface, 0, len(snapshot.Surfaces)),
	}
	for _, node := range snapshot.Nodes {
		nodeID, err := rebaseNodeID(node.NodeID, oldRoot, newRootNodeID)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		rebased.Nodes = append(rebased.Nodes, node)
	}
	for _, edge := range snapshot.Edges {
		fromNodeID, err := rebaseNodeID(edge.FromNodeID, oldRoot, newRootNodeID)
		if err != nil {
			return nil, err
		}
		toNodeID, err := rebaseNodeID(edge.ToNodeID, oldRoot, newRootNodeID)
		if err != nil {
			return nil, err
		}
		rebased.Edges = append(rebased.Edges, structure.Edge{
			FromNodeID: fromNodeID,
			ToNodeID:   toNodeID,
		})
	}
	for _, surface := range snapshot.Surfaces {
		nodeID, err := rebaseNodeID(surface.NodeID, oldRoot, newRootNodeID)
		if err != nil {
			return nil, err
		}
		surface.NodeID = nodeID
		surface.SurfaceID = ""
		rebased.Surfaces = append(rebased.Surfaces, surface)
	}
	return rebased, nil
}

// rebaseNodeID rewrites a single node id from oldRoot to newRoot.
func rebaseNodeID(nodeID string, oldRoot string, newRoot string) (string, error) {
	if nodeID == oldRoot {
		return newRoot, nil
	}
	prefix := oldRoot + "/"
	if !strings.HasPrefix(nodeID, prefix) {
		return "", fmt.Errorf("node id %q is outside root %q", nodeID, oldRoot)
	}
	return newRoot + strings.TrimPrefix(nodeID, oldRoot), nil
}

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

// Compile-time check: LLMAgent implements structure.Exporter.
var _ structure.Exporter = (*llmagent.LLMAgent)(nil)
