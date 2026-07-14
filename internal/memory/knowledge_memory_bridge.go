// Package memory provides the knowledge base ↔ memory synergy bridge.
//
// KnowledgeMemoryBridge coordinates between the knowledge base (module 37) and
// the L4 entity graph (module 70) without merging their storage. When a user
// confirms or rejects a knowledge base retrieval result, the bridge adjusts
// the confidence of L4 entities sourced from that collection. When an agent
// reports task success/failure, the bridge reinforces or suppresses related
// memory neurons.
//
// Boundary constraint (FR-11.8): knowledge chunks never enter memory_entities
// and memory facts never enter knowledge_chunks. Synergy happens only through
// metadata_json.source_collection_id references + confidence adjustments.
package memory

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Knowledge-memory bridge constants (per design §15.9).
const (
	// knowledgeConfirmBoost is the confidence boost applied when a user
	// confirms a knowledge base retrieval result.
	knowledgeConfirmBoost = 0.1

	// knowledgeRejectPenalty is the confidence penalty applied when a user
	// rejects a knowledge base retrieval result.
	knowledgeRejectPenalty = -0.1

	// taskSuccessBoost is the confidence boost applied to entities related to
	// a successfully completed task.
	taskSuccessBoost = 0.1

	// taskFailurePenalty is the confidence penalty applied to entities related
	// to a failed task.
	taskFailurePenalty = -0.1
)

// KnowledgeMemoryBridge coordinates knowledge base retrieval feedback and
// agent task feedback with L4 entity confidence adjustments. It depends on
// the narrow biz.L4KnowledgeBridgeStore (2 methods) so mock implementations
// only need to stub FindBySourceCollection and AdjustConfidence (per DB-N3).
type KnowledgeMemoryBridge struct {
	store biz.L4KnowledgeBridgeStore
	lg    loggateway.Logger
}

// NewKnowledgeMemoryBridge constructs a KnowledgeMemoryBridge. store may be
// nil (all methods become no-ops); lg falls back to a noop logger if nil.
func NewKnowledgeMemoryBridge(store biz.L4KnowledgeBridgeStore, lg loggateway.Logger) *KnowledgeMemoryBridge {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeMemoryBridge{
		store: store,
		lg:    lg.With(loggateway.Domain("knowledge_bridge")),
	}
}

// OnKnowledgeConfirmed adjusts confidence of all L4 entities sourced from the
// given collection. confirmed=true → +0.1 per entity; confirmed=false → -0.1.
//
// FindBySourceCollection is hard-failing (errors propagate). AdjustConfidence
// failures are best-effort (logged but do not abort the loop).
//
// Empty collectionID is a no-op. The bridge is safe for nil receivers and nil
// stores (returns nil).
func (s *KnowledgeMemoryBridge) OnKnowledgeConfirmed(
	ctx context.Context, collectionID, query string, confirmed bool,
) error {
	if s == nil || s.store == nil {
		return nil
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil
	}

	entities, err := s.store.FindBySourceCollection(ctx, collectionID)
	if err != nil {
		s.lg.Warn("knowledge_bridge: FindBySourceCollection failed",
			loggateway.Str("collectionID", collectionID),
			loggateway.Err(err))
		return err
	}

	delta := knowledgeConfirmBoost
	if !confirmed {
		delta = knowledgeRejectPenalty
	}

	for _, entity := range entities {
		if entity.ID == "" {
			continue
		}
		if _, err := s.store.AdjustConfidence(ctx, entity.ID, delta); err != nil {
			// Best-effort: log and continue with remaining entities.
			s.lg.Warn("knowledge_bridge: AdjustConfidence failed (best-effort)",
				loggateway.Str("entityID", entity.ID),
				loggateway.Err(err))
		}
	}
	return nil
}

// OnTaskFeedback adjusts confidence of related entities based on task result.
// taskResult="success" (or "succeeded"/"ok") → +0.1 per entity;
// taskResult="failure" (or "failed"/"error") → -0.1 per entity;
// any other value is a no-op.
//
// AdjustConfidence failures are best-effort (logged but do not abort).
//
// The bridge is safe for nil receivers and nil stores (returns nil).
func (s *KnowledgeMemoryBridge) OnTaskFeedback(
	ctx context.Context, agentID, taskResult string, relatedEntityIDs []string,
) error {
	if s == nil || s.store == nil {
		return nil
	}

	delta, ok := taskResultDelta(taskResult)
	if !ok {
		// Unknown result — no-op.
		return nil
	}

	for _, entityID := range relatedEntityIDs {
		entityID = strings.TrimSpace(entityID)
		if entityID == "" {
			continue
		}
		if _, err := s.store.AdjustConfidence(ctx, entityID, delta); err != nil {
			// Best-effort: log and continue.
			s.lg.Warn("knowledge_bridge: OnTaskFeedback AdjustConfidence failed (best-effort)",
				loggateway.Str("agentID", agentID),
				loggateway.Str("entityID", entityID),
				loggateway.Err(err))
		}
	}
	return nil
}

// taskResultDelta maps a task result string to a confidence delta. Returns
// ok=false for unrecognized results (no-op).
func taskResultDelta(taskResult string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(taskResult)) {
	case "success", "succeeded", "ok":
		return taskSuccessBoost, true
	case "failure", "failed", "error":
		return taskFailurePenalty, true
	default:
		return 0, false
	}
}
