package configgraph

import (
	"testing"
	"time"
)

func idxFixture() *NodeIndex {
	return NewNodeIndex([]Node{
		{ID: "n-agent-1", NodeType: NodeTypeAgent, RefID: "uuid-agent-1", NodeKey: "ops_master"},
		{ID: "n-agent-2", NodeType: NodeTypeAgent, RefID: "uuid-agent-2", NodeKey: "eval_memory_probe"},
		{ID: "n-tool-1", NodeType: NodeTypeTool, RefID: "uuid-tool-1", NodeKey: "shell_exec"},
		// key collides with another node's ref_id — ref must win on Resolve.
		{ID: "n-tool-2", NodeType: NodeTypeTool, RefID: "uuid-tool-2", NodeKey: "uuid-tool-1"},
		// no key — ref-only node.
		{ID: "n-org-1", NodeType: NodeTypeOrganization, RefID: "uuid-org-1"},
	})
}

func TestNodeIndex_ByRef(t *testing.T) {
	idx := idxFixture()
	n, ok := idx.ByRef(NodeTypeAgent, "uuid-agent-1")
	if !ok || n.ID != "n-agent-1" {
		t.Fatalf("ByRef hit failed: %+v ok=%v", n, ok)
	}
	if _, ok := idx.ByRef(NodeTypeTool, "uuid-agent-1"); ok {
		t.Fatal("ByRef must be scoped by node_type")
	}
	if _, ok := idx.ByRef(NodeTypeAgent, "nope"); ok {
		t.Fatal("ByRef unknown ref must miss")
	}
}

func TestNodeIndex_ByKey(t *testing.T) {
	idx := idxFixture()
	n, ok := idx.ByKey(NodeTypeAgent, "eval_memory_probe")
	if !ok || n.ID != "n-agent-2" {
		t.Fatalf("ByKey hit failed: %+v ok=%v", n, ok)
	}
	if _, ok := idx.ByKey(NodeTypeAgent, "uuid-agent-1"); ok {
		t.Fatal("ByKey must not resolve ref_ids")
	}
	if _, ok := idx.ByKey(NodeTypeOrganization, ""); ok {
		t.Fatal("ByKey empty key must miss")
	}
}

func TestNodeIndex_ResolveDual(t *testing.T) {
	idx := idxFixture()
	// uuid value → ref path.
	if n, ok := idx.Resolve(NodeTypeAgent, "uuid-agent-2"); !ok || n.ID != "n-agent-2" {
		t.Fatalf("Resolve ref failed: %+v ok=%v", n, ok)
	}
	// key value → key path.
	if n, ok := idx.Resolve(NodeTypeAgent, "ops_master"); !ok || n.ID != "n-agent-1" {
		t.Fatalf("Resolve key failed: %+v ok=%v", n, ok)
	}
	// collision: value equals tool-1 ref_id AND tool-2 key → ref wins.
	if n, ok := idx.Resolve(NodeTypeTool, "uuid-tool-1"); !ok || n.ID != "n-tool-1" {
		t.Fatalf("Resolve collision must prefer ref: %+v ok=%v", n, ok)
	}
}

func TestNodeIndex_AddAndLen(t *testing.T) {
	idx := NewNodeIndex(nil)
	if idx.Len() != 0 {
		t.Fatal("empty index len != 0")
	}
	idx.Add(Node{})
	idx.Add(Node{NodeType: NodeTypeAgent})
	if idx.Len() != 0 {
		t.Fatal("invalid nodes must be ignored")
	}
	n := Node{ID: "a", NodeType: NodeTypeAgent, RefID: "r", NodeKey: "k"}
	idx.Add(n)
	idx.Add(n) // duplicate
	if idx.Len() != 1 {
		t.Fatalf("len=%d want 1", idx.Len())
	}
	var nilIdx *NodeIndex
	if nilIdx.Len() != 0 {
		t.Fatal("nil index len != 0")
	}
	if _, ok := nilIdx.Resolve(NodeTypeAgent, "x"); ok {
		t.Fatal("nil index must miss")
	}
}

func TestEdgeResolve_ByRef(t *testing.T) {
	idx := idxFixture()
	src, _ := idx.ByRef(NodeTypeAgent, "uuid-agent-1")
	now := time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC)
	e := Edge{
		DstRef:   "uuid-tool-1",
		DstType:  NodeTypeTool,
		Type:     EdgeTypeGrantedTool,
		Evidence: map[string]any{"table": "agent_runtime_settings", "grant_origin": GrantOriginAllow},
	}
	se := e.Resolve(idx, src, 7, "edge-1", now)
	if se.DstID != "n-tool-1" {
		t.Fatalf("DstID=%q want n-tool-1", se.DstID)
	}
	if se.Broken() {
		t.Fatal("resolved edge must not be broken")
	}
	if se.Evidence["table"] != "agent_runtime_settings" || se.Evidence["grant_origin"] != GrantOriginAllow {
		t.Fatalf("evidence not carried: %+v", se.Evidence)
	}
	if se.SrcID != src.ID || se.Generation != 7 || se.ID != "edge-1" || !se.CreatedAt.Equal(now) {
		t.Fatalf("scalars wrong: %+v", se)
	}
}

func TestEdgeResolve_DstRefCarryingKey(t *testing.T) {
	idx := idxFixture()
	src, _ := idx.ByRef(NodeTypeAgent, "uuid-agent-1")
	// channel routing style: value is a key, not a uuid.
	e := Edge{DstRef: "shell_exec", DstType: NodeTypeTool, Type: EdgeTypeGrantedTool}
	se := e.Resolve(idx, src, 1, "e", time.Now())
	if se.DstID != "n-tool-1" || se.Broken() {
		t.Fatalf("key-valued DstRef must dual-resolve: %+v", se)
	}
}

func TestEdgeResolve_BrokenWhenUnresolved(t *testing.T) {
	idx := idxFixture()
	src, _ := idx.ByRef(NodeTypeAgent, "uuid-agent-1")

	// ORPHAN position_key shape: DstKey set, target never existed.
	e := Edge{DstType: NodeTypeOrganization, DstKey: "ghost_position", Type: EdgeTypeBoundPositionKey}
	se := e.Resolve(idx, src, 1, "e1", time.Now())
	if se.DstID != "" || !se.Broken() {
		t.Fatalf("must be broken: %+v", se)
	}
	if se.Evidence[EvidenceKeyDstKey] != "ghost_position" {
		t.Fatalf("dst_key not preserved: %+v", se.Evidence)
	}

	// DstRef set but unresolvable → broken with dst_key = the ref.
	e2 := Edge{DstRef: "uuid-ghost", DstType: NodeTypeTool, Type: EdgeTypeGrantedTool}
	se2 := e2.Resolve(idx, src, 1, "e2", time.Now())
	if se2.DstID != "" || !se2.Broken() {
		t.Fatalf("must be broken: %+v", se2)
	}
	if se2.Evidence[EvidenceKeyDstKey] != "uuid-ghost" {
		t.Fatalf("dst_key fallback wrong: %+v", se2.Evidence)
	}

	// both empty → broken, no dst_key.
	e3 := Edge{DstType: NodeTypeTool, Type: EdgeTypeGrantedTool}
	se3 := e3.Resolve(idx, src, 1, "e3", time.Now())
	if !se3.Broken() {
		t.Fatalf("empty dst must be broken: %+v", se3)
	}
	if _, has := se3.Evidence[EvidenceKeyDstKey]; has {
		t.Fatalf("dst_key must be absent: %+v", se3.Evidence)
	}
}

func TestEdgeResolve_ByKeyOnly(t *testing.T) {
	idx := idxFixture()
	src, _ := idx.ByRef(NodeTypeAgent, "uuid-agent-1")
	e := Edge{DstType: NodeTypeTool, DstKey: "shell_exec", Type: EdgeTypeGrantedTool}
	se := e.Resolve(idx, src, 1, "e", time.Now())
	if se.DstID != "n-tool-1" || se.Broken() {
		t.Fatalf("DstKey-only edge must resolve via key: %+v", se)
	}
}
