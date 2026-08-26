package configgraph

import (
	"strings"
	"time"
)

// indexSep separates node_type from the ref/key in index map keys. NUL never
// appears in either segment.
const indexSep = "\x00"

func indexKey(nodeType, ref string) string { return nodeType + indexSep + ref }

// NodeIndex resolves nodes by (node_type, ref_id) or (node_type, node_key) —
// the dual resolution needed for indirect references whose value may be either
// a uuid (ref_id) or a human key (channel routing target, graph agent_name…).
//
// Ref lookup always wins over key lookup on collision: ref_id is the
// authoritative identity, node_key only a display alias. Lookups are exact
// (no case folding / normalization — keys are stored slugs).
type NodeIndex struct {
	byRef map[string]Node   // nodeType+sep+refID → node
	byKey map[string]string // nodeType+sep+nodeKey → refID
}

// NewNodeIndex builds an index over nodes (first occurrence wins on duplicate
// (type, ref) — the store guarantees uniqueness per generation).
func NewNodeIndex(nodes []Node) *NodeIndex {
	idx := &NodeIndex{
		byRef: make(map[string]Node, len(nodes)),
		byKey: make(map[string]string, len(nodes)),
	}
	for _, n := range nodes {
		idx.Add(n)
	}
	return idx
}

// Add inserts one node. Nodes without type or ref_id are ignored.
func (idx *NodeIndex) Add(n Node) {
	if idx == nil || n.NodeType == "" || n.RefID == "" {
		return
	}
	rk := indexKey(n.NodeType, n.RefID)
	if _, dup := idx.byRef[rk]; dup {
		return
	}
	idx.byRef[rk] = n
	if n.NodeKey != "" {
		kk := indexKey(n.NodeType, n.NodeKey)
		if _, exists := idx.byKey[kk]; !exists {
			idx.byKey[kk] = n.RefID
		}
	}
}

// ByRef resolves by (node_type, ref_id).
func (idx *NodeIndex) ByRef(nodeType, refID string) (Node, bool) {
	if idx == nil {
		return Node{}, false
	}
	n, ok := idx.byRef[indexKey(nodeType, refID)]
	return n, ok
}

// ByKey resolves by (node_type, node_key).
func (idx *NodeIndex) ByKey(nodeType, key string) (Node, bool) {
	if idx == nil {
		return Node{}, false
	}
	refID, ok := idx.byKey[indexKey(nodeType, key)]
	if !ok {
		return Node{}, false
	}
	return idx.ByRef(nodeType, refID)
}

// Resolve dual-resolves a value that may be a ref_id or a node_key; ref wins.
func (idx *NodeIndex) Resolve(nodeType, refOrKey string) (Node, bool) {
	if n, ok := idx.ByRef(nodeType, refOrKey); ok {
		return n, true
	}
	return idx.ByKey(nodeType, refOrKey)
}

// Len returns the number of indexed nodes.
func (idx *NodeIndex) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.byRef)
}

// Resolve maps the extracted edge to its stored form: dst is dual-resolved
// (DstRef first — itself possibly a key — then DstKey). Unresolvable targets
// yield a broken edge (DstID="", evidence.broken=true, dst_key preserved);
// extraction evidence is always carried over.
func (e Edge) Resolve(idx *NodeIndex, src Node, generation int64, id string, now time.Time) StoredEdge {
	se := StoredEdge{
		ID:          id,
		SrcID:       src.ID,
		Type:        e.Type,
		WorkspaceID: e.WorkspaceID,
		Generation:  generation,
		CreatedAt:   now,
	}
	ev := make(map[string]any, len(e.Evidence)+2)
	for k, v := range e.Evidence {
		ev[k] = v
	}
	se.Evidence = ev

	dstRef := strings.TrimSpace(e.DstRef)
	dstKey := strings.TrimSpace(e.DstKey)

	var dst Node
	var ok bool
	if dstRef != "" {
		dst, ok = idx.Resolve(e.DstType, dstRef)
	} else if dstKey != "" {
		dst, ok = idx.ByKey(e.DstType, dstKey)
	}
	if ok {
		se.DstID = dst.ID
		return se
	}

	se.DstID = ""
	ev[EvidenceKeyBroken] = true
	switch {
	case dstKey != "":
		ev[EvidenceKeyDstKey] = dstKey
	case dstRef != "":
		ev[EvidenceKeyDstKey] = dstRef
	}
	return se
}
