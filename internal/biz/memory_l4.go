package biz

import (
	"context"
	"encoding/json"
	"strings"
)

type L4EntityWrite struct {
	ID             string
	ScopeType      string
	ScopeID        string
	UserID         string
	EntityType     string
	Name           string
	NameNormalized string
	Description    string
	Importance     float64
	Confidence     float64
	MetadataJSON   string
}

type L4RelationWrite struct {
	ScopeType    string
	ScopeID      string
	SourceID     string
	TargetID     string
	RelationType string
	Weight       float64
	Confidence   float64
}

// L4Entity represents a full entity read from the knowledge graph.
type L4Entity struct {
	ID             string
	ScopeType      string
	ScopeID        string
	UserID         string
	EntityType     string
	Name           string
	NameNormalized string
	Description    string
	Importance     float64
	Confidence     float64
	MetadataJSON   string
}

// L4Relation represents a relation read from the knowledge graph.
type L4Relation struct {
	ID           string
	ScopeType    string
	ScopeID      string
	SourceID     string
	TargetID     string
	RelationType string
	Weight       float64
	Confidence   float64
	MetadataJSON string
}

type L4EntitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
}

type ReinforcementSignal string

const (
	ReinforcementHit       ReinforcementSignal = "hit"
	ReinforcementConfirmed ReinforcementSignal = "confirmed"
	ReinforcementRefuted   ReinforcementSignal = "refuted"
	ReinforcementEdited    ReinforcementSignal = "edited"
)

type L4DecayConfig struct {
	HalfLifeDays map[string]float64
	Alpha        float64
}

func DefaultL4DecayConfig() L4DecayConfig {
	return L4DecayConfig{
		HalfLifeDays: map[string]float64{
			"person_core":  180,
			"person":       60,
			"place":        365,
			"preference":   90,
			"event":        14,
			"concept":      270,
			"user_profile": 365,
		},
		Alpha: 0.15,
	}
}

// MergeDecayOverrides parses overridesJSON as map[string]float64 and merges
// the entries into base.HalfLifeDays. Invalid JSON is silently ignored.
// This connects the existing L4DecayOverridesJSON field to the runtime decay config.
func MergeDecayOverrides(base L4DecayConfig, overridesJSON string) L4DecayConfig {
	overridesJSON = strings.TrimSpace(overridesJSON)
	if overridesJSON == "" {
		return base
	}
	var overrides map[string]float64
	if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
		return base
	}
	if len(overrides) == 0 {
		return base
	}
	// Clone the map to avoid mutating the base.
	merged := make(map[string]float64, len(base.HalfLifeDays)+len(overrides))
	for k, v := range base.HalfLifeDays {
		merged[k] = v
	}
	for k, v := range overrides {
		if v > 0 {
			merged[k] = v
		}
	}
	return L4DecayConfig{
		HalfLifeDays: merged,
		Alpha:        base.Alpha,
	}
}

func (c L4DecayConfig) HalfLifeForEntityType(entityType string, isCore bool) float64 {
	if isCore {
		if hl, ok := c.HalfLifeDays["person_core"]; ok {
			return hl
		}
	}
	if hl, ok := c.HalfLifeDays[entityType]; ok {
		return hl
	}
	return 60
}

type L4DecayResult struct {
	Decayed  int
	Archived int
}

// Stability:stable
type L4GraphWriter interface {
	WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error)
	RunDecay(ctx context.Context, agentID string)
	RunDecayWithConfig(ctx context.Context, agentID string, cfg L4DecayConfig) L4DecayResult
	RecordEntityReinforcement(ctx context.Context, entityID string, signal ReinforcementSignal, source string) error
}

type L4EntityReader interface {
	GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error)
	GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (L4EntitySnapshot, bool, error)
	GetEntityRelations(ctx context.Context, entityID string) ([]L4Relation, error)
	GetEntitiesByType(ctx context.Context, scope, entityType string) ([]L4Entity, error)
	SearchEntitiesByName(ctx context.Context, scope, nameQuery string, limit int) ([]L4Entity, error)
}

// Stability:stable
type L4EntityWriter interface {
	UpsertEntity(ctx context.Context, params L4EntityWrite) error
	UpsertRelation(ctx context.Context, params L4RelationWrite) error
}

// L4EntityReadWriter is the combined interface for read-then-write entity
// resolution. PathBExtractor uses it to look up existing entities by scope
// key before minting a fresh UUID, ensuring ID stability across re-extraction.
//
// Stability:stable
type L4EntityReadWriter interface {
	L4EntityReader
	L4EntityWriter
}

// PathBL4Writer is the narrow interface used by PathBExtractor for
// read-then-write entity resolution. It only requires the three methods
// PathBExtractor actually calls, avoiding forcing implementers to satisfy
// the full L4EntityReader (which includes search/list methods irrelevant
// to extraction-time writes).
//
// Stability:stable
type PathBL4Writer interface {
	GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (L4EntitySnapshot, bool, error)
	UpsertEntity(ctx context.Context, params L4EntityWrite) error
	UpsertRelation(ctx context.Context, params L4RelationWrite) error
}

type L4DecayWriter interface {
	ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID, olderThanRFC3339 string, factor float64) (int64, error)
	ApplyBusinessConfidenceDecay(ctx context.Context, scopeType, scopeID string, cfg L4DecayConfig, nowUnixMs int64) (int64, error)
	ArchiveLowConfidenceEntities(ctx context.Context, scopeType, scopeID string, threshold float64) (int64, error)
	RecordEntityReinforcement(ctx context.Context, entityID string, signal ReinforcementSignal, source string) error
	GetRecentReinforcementCounts(ctx context.Context, scopeType, scopeID string, windowDays int) (map[string]int, error)
}

// Stability:stable
type L4GraphRepo interface {
	L4EntityReader
	L4EntityWriter
	L4DecayWriter
}

// ──────────────────────────────────────────────────────────
// Relation type constants (Phase E, FR-10.7)
// ──────────────────────────────────────────────────────────

// L4 relation type constants. These extend the existing RELATED_TO/EVOLVED_FROM
// pair (originally defined in internal/memory/link_evolution.go as LinkType*)
// with three new types for causal reasoning, temporal sequencing, and conflict
// resolution (inhibition). The DB column relation_type is free-form TEXT, so
// these constants live in the application layer as the canonical vocabulary.
const (
	RelationRelatedTo    = "RELATED_TO"    // bidirectional: general association
	RelationEvolvedFrom  = "EVOLVED_FROM"  // directed: new memory supersedes old
	RelationCausal       = "CAUSAL"        // directed: A causes B (FR-10.7)
	RelationTemporalNext = "TEMPORAL_NEXT" // directed: B follows A in time (FR-10.7)
	RelationInhibit      = "INHIBIT"       // directed: A suppresses B (conflict resolution, FR-10.7)
)

// RelationTypeProp describes the behavioral properties of a relation type.
// Used by SpreadingActivationEngine to decide whether a relation propagates
// activation (ReinforcesTarget), attenuates it (InhibitsTarget), or is neutral,
// and whether traversal should follow the reverse direction (Bidirectional).
type RelationTypeProp struct {
	Bidirectional    bool
	ReinforcesTarget bool
	InhibitsTarget   bool
}

// relationTypeProps is the canonical property table for the 5 relation types.
// SpreadingActivationEngine and ConflictResolver consult this table to apply
// the correct propagation semantics per edge type.
var relationTypeProps = map[string]RelationTypeProp{
	RelationRelatedTo:    {Bidirectional: true, ReinforcesTarget: true, InhibitsTarget: false},
	RelationEvolvedFrom:  {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: true},
	RelationCausal:       {Bidirectional: false, ReinforcesTarget: true, InhibitsTarget: false},
	RelationTemporalNext: {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: false},
	RelationInhibit:      {Bidirectional: false, ReinforcesTarget: false, InhibitsTarget: true},
}

// LookupRelationTypeProp returns the property table entry for the given relation
// type string. Returns ok=false for unknown types (callers should treat unknown
// types as neutral — no reinforcement, no inhibition, not bidirectional).
func LookupRelationTypeProp(relationType string) (RelationTypeProp, bool) {
	prop, ok := relationTypeProps[relationType]
	return prop, ok
}

// ──────────────────────────────────────────────────────────
// Recursive CTE graph traversal (Phase E, FR-10.11 / NFR-1.7)
// ──────────────────────────────────────────────────────────

// MemoryGraphNode represents a node in a traversed memory subgraph, carrying
// the activation value propagated from the center node through weighted edges.
type MemoryGraphNode struct {
	ID         string
	EntityType string
	Name       string
	Activation float64
	Hop        int
}

// MemoryGraphEdge represents a directed relationship between two nodes in the
// traversed memory subgraph. Weight is the edge weight at traversal time.
type MemoryGraphEdge struct {
	SourceID     string
	TargetID     string
	RelationType string
	Weight       float64
}

// MemoryGraphTraversal is the result of a recursive CTE graph traversal. It
// contains the center node ID, all reachable nodes within the hop limit (with
// propagated activation values), and the edges traversed.
//
// Neighbors returns the edges incident to the given node, suitable for
// spreading-activation propagation in the application layer.
type MemoryGraphTraversal struct {
	CenterID string
	Hops     int
	Nodes    []MemoryGraphNode
	Edges    []MemoryGraphEdge
}

// Neighbors returns all edges incident to nodeID (either as source or target).
// For bidirectional relation types the caller may follow either direction;
// for directed types the caller decides based on RelationTypeProp.Bidirectional.
func (g *MemoryGraphTraversal) Neighbors(nodeID string) []MemoryGraphEdge {
	if g == nil {
		return nil
	}
	var out []MemoryGraphEdge
	for i := range g.Edges {
		e := &g.Edges[i]
		if e.SourceID == nodeID || e.TargetID == nodeID {
			out = append(out, *e)
		}
	}
	return out
}

// NodeByID returns the MemoryGraphNode with the given ID and ok=true, or
// ok=false if not present in the traversal result.
func (g *MemoryGraphTraversal) NodeByID(nodeID string) (MemoryGraphNode, bool) {
	if g == nil {
		return MemoryGraphNode{}, false
	}
	for i := range g.Nodes {
		if g.Nodes[i].ID == nodeID {
			return g.Nodes[i], true
		}
	}
	return MemoryGraphNode{}, false
}

// L4GraphTraverser is the narrow interface for recursive CTE graph traversal.
// It is intentionally separate from L4EntityStore (which already has 5 methods)
// and L4GraphRepo (Stable composite) to respect the ≤5 method guideline
// (DB-N3) and the Interface Segregation Principle.
//
// SpreadingActivationEngine depends on this narrow interface rather than the
// full L4GraphRepo, so that mock implementations only need to stub traversal.
//
// Stability:evolving
type L4GraphTraverser interface {
	// GraphTraverseCTE performs a single recursive-CTE traversal starting from
	// centerID, propagating activation = parent_activation * edge_weight up to
	// `hops` levels. Returns nodes (with activation + hop) and edges.
	// topK limits the number of returned nodes (highest activation first).
	GraphTraverseCTE(ctx context.Context, centerID string, hops, topK int) (*MemoryGraphTraversal, error)
}

// ──────────────────────────────────────────────────────────
// Spreading activation (Phase E, FR-10.3 / FR-10.8 / AC-9)
// ──────────────────────────────────────────────────────────

// L4ActivationResult is the output of a spreading-activation query for a single
// node. ActivationPath traces the strongest propagation path from the center
// node (FR-10.8 explainability).
type L4ActivationResult struct {
	NodeID         string
	Activation     float64
	HopCount       int
	ActivationPath []L4PathStep
}

// L4PathStep is one edge in the activation propagation path.
type L4PathStep struct {
	FromNodeID   string
	ToNodeID     string
	EdgeWeight   float64
	RelationType string
}

// L4SpreadingActivationEngine is the narrow port for spreading-activation
// retrieval. It is intentionally separate from L4GraphTraverser (which only
// fetches the subgraph) so the service layer can mock the activation engine
// without stubbing the CTE traversal.
//
// The concrete implementation lives in internal/memory and depends on
// L4GraphTraverser for subgraph fetching.
//
// Stability:evolving
type L4SpreadingActivationEngine interface {
	// SpreadingActivation executes a spreading-activation query from centerID.
	// hops defaults to 3 when <= 0; topK defaults to 20 when <= 0.
	// Returns results sorted by activation descending (center first, activation=1.0).
	SpreadingActivation(ctx context.Context, centerID string, hops, topK int) ([]L4ActivationResult, error)
}

// ──────────────────────────────────────────────────────────
// Hebbian weight update (Phase E, FR-10.4 / AC-10)
// ──────────────────────────────────────────────────────────

// L4HebbianRelation carries the data needed by the Hebbian update rule:
// the current weight, co-activation count, and the activation values of
// both endpoints (read from memory_entities.activation via JOIN).
type L4HebbianRelation struct {
	ID                string
	SourceID          string
	TargetID          string
	RelationType      string
	Weight            float64
	CoActivationCount int
	SourceActivation  float64 // memory_entities.activation for source node
	TargetActivation  float64 // memory_entities.activation for target node
}

// L4HebbianStore is the narrow interface for Hebbian weight updates. It is
// separate from L4EntityWriter (Stable, 2 methods) and L4GraphRepo (Stable
// composite) to respect DB-N3 and avoid expanding Stable interfaces.
//
// HebbianUpdater depends on this narrow interface so that mock
// implementations only need to stub three methods.
//
// Stability:evolving
type L4HebbianStore interface {
	// FindRelation finds an active relation between nodeA and nodeB with the
	// given relation type. For bidirectional types, either (A→B) or (B→A)
	// matches. Returns ok=false if no relation is found.
	FindRelation(ctx context.Context, nodeA, nodeB, relationType string) (L4HebbianRelation, bool, error)

	// UpdateRelationWeight updates weight, co_activation_count, and
	// last_reinforced_at for the relation with the given ID.
	UpdateRelationWeight(ctx context.Context, relationID string, newWeight float64, coActivationCount int, lastReinforcedAtRFC3339 string) error

	// DecayUnusedRelations decays weights of active relations whose
	// last_reinforced_at is older than olderThanRFC3339 (empty
	// last_reinforced_at falls back to created_at). Weight *= 0.95; relations
	// whose weight drops below 0.1 are marked status='archived'.
	// Returns counts of decayed and archived relations.
	DecayUnusedRelations(ctx context.Context, olderThanRFC3339 string) (L4DecayResult, error)
}

// ──────────────────────────────────────────────────────────
// Memory reconsolidation (Phase E, FR-10.5 / AC-10)
// ──────────────────────────────────────────────────────────

// L4ReconsolidationStore is the narrow interface for memory reconsolidation
// updates triggered when an entity is recalled. It provides atomic activation
// boosting and use_count increment without expanding L4EntityWriter (Stable)
// or L4GraphRepo (Stable composite).
//
// Stability:evolving
type L4ReconsolidationStore interface {
	// BoostActivation atomically increases activation by delta (saturated to
	// 1.0) and sets activation_updated_at to nowRFC3339. Returns ok=false if
	// the entity was not found or is deleted.
	BoostActivation(ctx context.Context, nodeID string, delta float64, nowRFC3339 string) (bool, error)

	// IncrementUseCount atomically increments use_count by 1 for the given
	// entity. Returns ok=false if the entity was not found or is deleted.
	IncrementUseCount(ctx context.Context, nodeID string) (bool, error)
}

// L4Reconsolidator is the narrow port for triggering memory reconsolidation
// when L4 entities are recalled into the prompt (design §15.7, FR-10.5).
// The concrete implementation lives in internal/memory
// (ReconsolidationService); the agent layer depends only on this port so the
// before-model hook can fire it asynchronously without importing the memory
// package.
//
// Stability:evolving
type L4Reconsolidator interface {
	// OnRecall boosts the recalled node's activation and reinforces
	// connections to recalledWith via the Hebbian rule. It does not increment
	// use_count (C2: recall is not usage). Best-effort semantics live inside
	// the implementation.
	OnRecall(ctx context.Context, nodeID string, recalledWith []string) error
}

// ──────────────────────────────────────────────────────────
// Conflict resolution (Phase E, FR-10.6 / AC-11 / IR-7)
// ──────────────────────────────────────────────────────────

// L4InhibitRelationCreate carries the data needed to create an INHIBIT
// relation between two entities for conflict resolution. The relation is
// directed: SourceID (new entity) suppresses TargetID (old entity).
type L4InhibitRelationCreate struct {
	ScopeType   string
	ScopeID     string
	SourceID    string  // new entity (suppressor)
	TargetID    string  // old entity (suppressed)
	Weight      float64 // default 0.8 (strong inhibition) if zero
	ContextNote string  // conflict reason / context
}

// L4ConflictStore is the narrow interface for conflict resolution: creating
// INHIBIT relations and adjusting entity confidence. It is separate from
// L4EntityWriter (Stable) and L4GraphRepo (Stable composite) to respect
// DB-N3 and AS-FIT-01 (Stable interfaces cannot be expanded).
//
// Stability:evolving
type L4ConflictStore interface {
	// CreateInhibitRelation creates (or upserts) a directed INHIBIT relation
	// from SourceID to TargetID with the given weight and context_note.
	CreateInhibitRelation(ctx context.Context, params L4InhibitRelationCreate) error

	// AdjustConfidence atomically adjusts confidence by delta (saturated to
	// [0, 1]) for the given entity. Returns ok=false if not found.
	AdjustConfidence(ctx context.Context, entityID string, delta float64) (bool, error)
}

// ──────────────────────────────────────────────────────────
// Knowledge base ↔ memory synergy (Phase E, FR-11.7 / FR-10.10 / AC-12)
// ──────────────────────────────────────────────────────────

// L4KnowledgeBridgeStore is the narrow interface for knowledge base ↔ memory
// synergy. FindBySourceCollection looks up entities by their
// metadata_json.source_collection_id; AdjustConfidence is shared with
// L4ConflictStore (same method signature, data layer implements once).
//
// Stability:evolving
type L4KnowledgeBridgeStore interface {
	// FindBySourceCollection returns all active, non-deleted entities whose
	// metadata_json.source_collection_id matches collectionID.
	FindBySourceCollection(ctx context.Context, collectionID string) ([]L4EntitySnapshot, error)

	// AdjustConfidence atomically adjusts confidence by delta (saturated to
	// [0, 1]) for the given entity. Returns ok=false if not found.
	AdjustConfidence(ctx context.Context, entityID string, delta float64) (bool, error)
}
