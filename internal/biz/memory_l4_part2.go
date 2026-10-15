package biz

import (
	"context"
)

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
