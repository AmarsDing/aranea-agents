// Package memory provides conflict resolution for the L4 entity graph.
//
// When new knowledge contradicts existing memory, the ConflictResolver creates
// a directed INHIBIT relation (new → old) so that SpreadingActivationEngine
// skips the suppressed entity during retrieval, and lowers the old entity's
// confidence to reflect the contradiction. The old memory is never deleted —
// bi-temporal history is preserved for audit and potential rollback.
package memory

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Conflict resolution constants (per design §15.8).
const (
	// inhibitWeight is the default weight for INHIBIT relations. A high weight
	// (0.8) indicates strong suppression — the relation will reliably block
	// activation spreading to the suppressed entity.
	inhibitWeight = 0.8

	// confidencePenalty is the confidence adjustment applied to the old
	// (suppressed) entity. -0.3 reflects a moderate demotion that preserves
	// the memory for history without fully discarding it.
	confidencePenalty = -0.3
)

// ErrSelfInhibition is returned when ResolveConflict is called with the same
// entity as both the new and old entity (self-inhibition is not allowed).
var ErrSelfInhibition = errors.New("conflict resolver: self-inhibition not allowed (newEntityID == oldEntityID)")

// ErrEmptyEntityID is returned when ResolveConflict is called with an empty
// entity ID for either the new or old entity.
var ErrEmptyEntityID = errors.New("conflict resolver: entity ID must not be empty")

// ConflictResolver implements conflict resolution between new and existing
// memories. When new knowledge contradicts an existing entity, it:
//  1. Creates a directed INHIBIT relation from the new entity to the old one.
//  2. Lowers the old entity's confidence by confidencePenalty (-0.3).
//
// It depends on the narrow biz.L4ConflictStore (2 methods) rather than the
// full L4GraphRepo (Stable composite), so mock implementations only need to
// stub CreateInhibitRelation and AdjustConfidence (per DB-N3 / AS-FIT-01).
type ConflictResolver struct {
	store biz.L4ConflictStore
	lg    loggateway.Logger
}

// NewConflictResolver constructs a ConflictResolver. store may be nil
// (ResolveConflict becomes a no-op); lg falls back to a noop logger if nil.
func NewConflictResolver(store biz.L4ConflictStore, lg loggateway.Logger) *ConflictResolver {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ConflictResolver{
		store: store,
		lg:    lg.With(loggateway.Domain("conflict_resolver")),
	}
}

// ResolveConflict resolves a contradiction between a new memory and an existing
// memory by:
//  1. Creating a directed INHIBIT relation (new → old, weight=inhibitWeight)
//     with the given conflictReason stored in context_note.
//  2. Lowering the old entity's confidence by confidencePenalty (-0.3),
//     saturated to [0, 1].
//
// Step 1 is hard-failing — errors propagate and Step 2 is skipped. Step 2
// errors propagate too, but if the entity is not found (ok=false) the call is
// treated as a graceful no-op (the entity may have been concurrently deleted).
//
// The old memory is NOT deleted; bi-temporal history is preserved. The INHIBIT
// relation ensures SpreadingActivationEngine skips it during retrieval.
//
// The resolver is safe for nil receivers and nil stores (returns nil).
// Self-inhibition (newEntityID == oldEntityID) and empty IDs are rejected.
func (r *ConflictResolver) ResolveConflict(
	ctx context.Context, newEntityID, oldEntityID, conflictReason string,
) error {
	if r == nil || r.store == nil {
		return nil
	}

	newEntityID = strings.TrimSpace(newEntityID)
	oldEntityID = strings.TrimSpace(oldEntityID)
	if newEntityID == "" || oldEntityID == "" {
		return ErrEmptyEntityID
	}
	if newEntityID == oldEntityID {
		return ErrSelfInhibition
	}

	// Step 1: create INHIBIT relation (new → old, weight=0.8).
	createParams := biz.L4InhibitRelationCreate{
		SourceID:    newEntityID,
		TargetID:    oldEntityID,
		Weight:      inhibitWeight,
		ContextNote: conflictReason,
	}
	if err := r.store.CreateInhibitRelation(ctx, createParams); err != nil {
		r.lg.Warn("conflict_resolver: CreateInhibitRelation failed",
			loggateway.Str("newEntityID", newEntityID),
			loggateway.Str("oldEntityID", oldEntityID),
			loggateway.Str("conflictReason", conflictReason),
			loggateway.Err(err))
		return err
	}

	// Step 2: lower old entity's confidence by -0.3 (saturated to [0, 1]).
	ok, err := r.store.AdjustConfidence(ctx, oldEntityID, confidencePenalty)
	if err != nil {
		r.lg.Warn("conflict_resolver: AdjustConfidence failed",
			loggateway.Str("oldEntityID", oldEntityID),
			loggateway.Err(err))
		return err
	}
	if !ok {
		// Entity not found — graceful no-op (may have been concurrently deleted).
		r.lg.Info("conflict_resolver: old entity not found during AdjustConfidence (concurrent delete?)",
			loggateway.Str("oldEntityID", oldEntityID))
	}
	return nil
}
