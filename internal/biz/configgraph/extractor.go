package configgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Extractor pulls one asset type's nodes and outgoing edges from the source
// tables. Implementations are pure: the same rows always yield the same
// output, and a single bad row degrades to broken edges (or an attrs parse
// marker), never an aborted run.
//
// Emission rule: extractor X reads asset X's table(s) and emits the node for
// X plus one edge for every reference stored in those rows — regardless of
// which asset the edge points from/to (e.g. owns_skill: agent→skill is read
// from skill rows; has_prompt_file: agent→prompt_file from prompt rows).
type Extractor interface {
	NodeType() string
	ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error)
	ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error)
}

// Extractors returns the 12 asset extractors in target-first display order
// (referenced assets before referrers). provider feeds granted_tool edges and
// may be nil (granted_tool extraction is then skipped — tests only).
func Extractors(provider EffectiveToolsProvider) []Extractor {
	return []Extractor{
		toolExtractor{},
		skillExtractor{},
		organizationExtractor{},
		knowledgeCollectionExtractor{},
		mcpServerExtractor{},
		graphExtractor{},
		hookExtractor{},
		promptFileExtractor{},
		agentExtractor{provider: provider},
		teamExtractor{},
		cronTaskExtractor{},
		channelExtractor{},
	}
}

// graphNamespace scopes deterministic node/edge IDs (uuid v5) to the config
// graph so they never collide with ad-hoc uuid5 users.
var graphNamespace = uuid.Must(uuid.Parse("9f3b2c7a-5d6e-4f1a-8b9c-0d1e2f3a4b5c"))

// NodeID derives the deterministic node id for (node_type, ref_id). The same
// source row always maps to the same id, so repeated rebuilds are byte-stable
// and idempotent (acceptance: 幂等重跑结果一致).
func NodeID(nodeType, refID string) string {
	return uuid.NewSHA1(graphNamespace, []byte("node\x00"+nodeType+"\x00"+refID)).String()
}

// edgeID derives the deterministic edge id for one stored edge row. Broken
// edges share dst="" per (src,type) — matching the merge rule in
// ResolveEdges and the (src_id,dst_id,edge_type,generation) unique key.
func edgeID(srcID, dstID, edgeType string) string {
	return uuid.NewSHA1(graphNamespace, []byte("edge\x00"+srcID+"\x00"+dstID+"\x00"+edgeType)).String()
}

// newNode builds one node with its deterministic id.
func newNode(nodeType, refID, key, displayName, workspaceID, status string, attrs map[string]any) Node {
	if status == "" {
		status = NodeStatusActive
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	return Node{
		ID:          NodeID(nodeType, refID),
		NodeType:    nodeType,
		RefID:       refID,
		NodeKey:     key,
		DisplayName: displayName,
		WorkspaceID: workspaceID,
		Status:      status,
		Attrs:       attrs,
	}
}

// statusFromDeletedAt maps a soft-delete marker to node status.
func statusFromDeletedAt(deletedAt string) string {
	if strings.TrimSpace(deletedAt) != "" {
		return NodeStatusDeleted
	}
	return NodeStatusActive
}

// evidence builds the standard provenance envelope {table, field, path}.
// Extractor-specific extras (role, peer_pattern, mode…) are added by callers.
func evidence(table, field, path string) map[string]any {
	return map[string]any{"table": table, "field": field, "path": path}
}

// withExtra returns ev with extra keys merged in (empty values skipped).
func withExtra(ev map[string]any, kv ...string) map[string]any {
	for i := 0; i+1 < len(kv); i += 2 {
		if strings.TrimSpace(kv[i+1]) != "" {
			ev[kv[i]] = kv[i+1]
		}
	}
	return ev
}

// bodyHash is the prompt_file content fingerprint (attrs.body_hash; duplicate
// prompt detection groups on it).
func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// dstKeysEvidenceKey lists all merged broken target keys (broken merge rule).
const dstKeysEvidenceKey = "dst_keys"

// extractErrorEdge builds a broken-marker edge with no target identity: it
// survives ResolveEdges as a broken row flagging a per-row parse failure
// (extractors never abort on single-row errors).
func extractErrorEdge(srcType, srcRef, edgeType, workspaceID, table, field, path string, err error) Edge {
	ev := evidence(table, field, path)
	ev["extract_error"] = err.Error()
	return Edge{SrcType: srcType, SrcRef: srcRef, Type: edgeType, Evidence: ev, WorkspaceID: workspaceID}
}

// ResolveEdges maps extracted edges to stored form against the full node
// index and enforces the graph's write-time invariants:
//
//   - src resolution: (SrcType, SrcRef) must exist in the index; edges whose
//     source node is missing are dropped (extractor/node mismatch — the node
//     set is built from the same rows, so this only fires on extractor bugs).
//   - dst resolution: per Edge.Resolve (DstRef dual-resolve, then DstKey);
//     unresolvable targets become broken edges.
//   - dedupe: non-broken edges dedupe on (src_id, dst_id, edge_type),
//     first evidence wins (extractor order is deterministic).
//   - broken merge: broken edges collapse per (src_id, edge_type) because the
//     unique constraint (src_id, dst_id, edge_type, generation) cannot hold
//     multiple dst_id=” rows. Every merged target key is preserved under
//     evidence.dst_keys (first key also stays in dst_key).
func ResolveEdges(edges []Edge, idx *NodeIndex, generation int64, now time.Time) []StoredEdge {
	if idx == nil {
		return nil
	}
	out := make([]StoredEdge, 0, len(edges))
	seenOK := make(map[string]int, len(edges))    // src+dst+type → index in out
	brokenIdx := make(map[string]int, len(edges)) // src+type → index in out
	for _, e := range edges {
		src, ok := idx.ByRef(e.SrcType, e.SrcRef)
		if !ok {
			continue
		}
		if e.DstRef == "" && e.DstKey == "" && !brokenOnlyMarker(e) {
			// No target identity at all and not an explicit extract-error
			// marker: nothing to resolve or report.
			continue
		}
		se := e.Resolve(idx, src, generation, "", now)
		if se.Broken() {
			bk := se.SrcID + "\x00" + se.Type
			if at, dup := brokenIdx[bk]; dup {
				mergeBrokenEdge(&out[at], se)
				continue
			}
			se.ID = edgeID(se.SrcID, "", se.Type)
			brokenIdx[bk] = len(out)
			out = append(out, se)
			continue
		}
		okk := se.SrcID + "\x00" + se.DstID + "\x00" + se.Type
		if _, dup := seenOK[okk]; dup {
			continue
		}
		seenOK[okk] = len(out)
		se.ID = edgeID(se.SrcID, se.DstID, se.Type)
		out = append(out, se)
	}
	return out
}

// brokenOnlyMarker reports whether the edge is an explicit extract-error
// marker (e.g. granted_tool provider failure) that must survive as a broken
// row even without any target key.
func brokenOnlyMarker(e Edge) bool {
	if e.Evidence == nil {
		return false
	}
	_, hasErr := e.Evidence["extract_error"]
	return hasErr
}

// mergeBrokenEdge folds dup into the first broken edge of its (src, type):
// the dup's dst_key is appended to evidence.dst_keys (order-preserving,
// deduped) and a secondary extract_error is joined with '; '.
func mergeBrokenEdge(first *StoredEdge, dup StoredEdge) {
	key, _ := dup.Evidence[EvidenceKeyDstKey].(string)
	keys := dstKeyList(first.Evidence)
	if key != "" && !containsString(keys, key) {
		keys = append(keys, key)
	}
	if len(keys) > 0 {
		first.Evidence[dstKeysEvidenceKey] = keys
	}
	if msg, _ := dup.Evidence["extract_error"].(string); msg != "" {
		if prev, _ := first.Evidence["extract_error"].(string); prev != "" && !strings.Contains(prev, msg) {
			first.Evidence["extract_error"] = prev + "; " + msg
		} else if prev == "" {
			first.Evidence["extract_error"] = msg
		}
	}
}

// dstKeyList returns the merged broken target keys recorded so far.
func dstKeyList(ev map[string]any) []string {
	switch v := ev[dstKeysEvidenceKey].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if single, _ := ev[EvidenceKeyDstKey].(string); single != "" {
		return []string{single}
	}
	return nil
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// sortedKeys returns map keys in deterministic order (map iteration in
// extractors — e.g. failure-policy node_overrides — must not leak into edge
// order, or rebuilds stop being byte-stable).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
