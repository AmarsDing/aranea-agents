// Package configgraph builds and queries the config-asset dependency graph (M81).
//
// The graph persists 12 kinds of config assets as nodes and 27 kinds of
// references as edges inside Postgres (tables config_graph_nodes /
// config_graph_edges, DDL migration 20261260). Full rebuilds write a new
// generation and atomically switch the current pointer; queries always read
// the current generation, so there is no clear-table window. The package is
// read-only analysis infrastructure: P0-P2 must not intercept or alter any
// business write path.
package configgraph

import "time"

// Node types (12).
const (
	NodeTypeAgent               = "agent"
	NodeTypeTeam                = "team"
	NodeTypeSkill               = "skill"
	NodeTypeTool                = "tool"
	NodeTypePromptFile          = "prompt_file"
	NodeTypeCronTask            = "cron_task"
	NodeTypeChannel             = "channel"
	NodeTypeOrganization        = "organization"
	NodeTypeGraph               = "graph"
	NodeTypeKnowledgeCollection = "knowledge_collection"
	NodeTypeMCPServer           = "mcp_server"
	NodeTypeHook                = "hook"
)

// Edge types (27). Keep in sync with design 81-config-graph.design.md §3.2.
const (
	EdgeTypeHasMember        = "has_member"         // team→agent
	EdgeTypeSynthesizer      = "synthesizer"        // team→agent
	EdgeTypeIntentAnchor     = "intent_anchor"      // team→agent
	EdgeTypeFallbackAgent    = "fallback_agent"     // team→agent
	EdgeTypeDeptLead         = "dept_lead"          // team→agent
	EdgeTypeCrossDeptMember  = "cross_dept_member"  // team→agent
	EdgeTypeBelongsTo        = "belongs_to"         // team→organization
	EdgeTypeLinkedGraph      = "linked_graph"       // team→graph
	EdgeTypeGraphTemplate    = "graph_template"     // team→graph
	EdgeTypeScopedKnowledge  = "scoped_knowledge"   // team→knowledge_collection
	EdgeTypeGraphAgent       = "graph_agent"        // graph→agent
	EdgeTypeGraphTool        = "graph_tool"         // graph→tool
	EdgeTypeGraphOwnedBy     = "graph_owned_by"     // graph→team
	EdgeTypeHasPromptFile    = "has_prompt_file"    // agent→prompt_file
	EdgeTypeGrantedTool      = "granted_tool"       // agent→tool
	EdgeTypeToolOverride     = "tool_override"      // agent→tool
	EdgeTypeEnablesMCP       = "enables_mcp"        // agent→mcp_server
	EdgeTypeOwnsSkill        = "owns_skill"         // agent→skill
	EdgeTypeAllowsSkill      = "allows_skill"       // agent→skill
	EdgeTypeSkillParent      = "skill_parent"       // skill→skill
	EdgeTypeBoundPosition    = "bound_position"     // agent→organization
	EdgeTypeBoundPositionKey = "bound_position_key" // agent→organization (key dual-resolution; ORPHAN anchor)
	EdgeTypeOrgDeptLead      = "org_dept_lead"      // organization→agent
	EdgeTypeOrgParent        = "org_parent"         // organization→organization
	EdgeTypeRuns             = "runs"               // cron_task→agent/team
	EdgeTypeRoutesTo         = "routes_to"          // channel→agent/team
	EdgeTypeHookRef          = "hook_ref"           // hook→agent/tool
)

// Grant origins recorded in granted_tool edge evidence (grant_origin).
const (
	GrantOriginProfile  = "profile"
	GrantOriginAllow    = "allow"
	GrantOriginOverride = "override"
)

// Node status values.
const (
	NodeStatusActive  = "active"
	NodeStatusDeleted = "deleted"
)

// Evidence keys with reserved meaning (rest is extractor-specific provenance:
// table/field/path).
const (
	EvidenceKeyBroken      = "broken"
	EvidenceKeyDstKey      = "dst_key"
	EvidenceKeyGrantOrigin = "grant_origin"
)

// Node is one config-asset vertex (stored form, scoped to one generation).
// Attrs is the type-specific attribute snapshot (e.g. agent.kind,
// tool.risk_level, prompt_file.body_hash); never full bodies.
type Node struct {
	ID          string
	NodeType    string
	RefID       string
	NodeKey     string
	DisplayName string
	WorkspaceID string
	Status      string
	Attrs       map[string]any
	Generation  int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Edge is one extracted reference (extractor output, ref-based). DstRef is the
// target ref_id when known; DstKey keeps the human-readable target key and is
// mandatory when the reference is already known-broken at extraction time.
// SrcType is the source node_type — edges are emitted by the extractor that
// reads the row holding the reference, which is not always the source asset's
// own extractor (e.g. owns_skill: agent→skill is read from skill rows).
type Edge struct {
	SrcType     string
	SrcRef      string
	DstRef      string
	DstType     string
	DstKey      string
	Type        string
	Evidence    map[string]any
	WorkspaceID string
}

// StoredEdge is one resolved edge row (id-based, scoped to one generation).
// Broken edges carry DstID="" with evidence broken=true and dst_key preserved.
type StoredEdge struct {
	ID          string
	SrcID       string
	DstID       string
	Type        string
	Evidence    map[string]any
	WorkspaceID string
	Generation  int64
	CreatedAt   time.Time
}

// Broken reports whether the edge failed dst resolution.
func (e StoredEdge) Broken() bool {
	b, _ := e.Evidence[EvidenceKeyBroken].(bool)
	return b
}

// NodeFilter selects nodes for the nodes-search API. KeyContains is a
// substring match on node_key (LIKE-escaped). Limit <= 0 defaults (repo-side).
type NodeFilter struct {
	NodeType    string
	KeyContains string
	WorkspaceID string
	Generation  int64
	Limit       int
}

// Counts aggregates per-generation statistics for the status API.
type Counts struct {
	Nodes  int64
	Edges  int64
	Broken int64
}
