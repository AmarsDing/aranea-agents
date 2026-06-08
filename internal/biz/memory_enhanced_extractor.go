package biz

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// EnhancedExtractionResult holds the output of a combined Episode + Entity + Relation extraction.
// Mirrors compress.EnhancedExtractionResult but lives in biz to avoid import cycles.
type EnhancedExtractionResult struct {
	Episode   EnhancedEpisodeData
	Entities  []ExtractedEntity
	Relations []ExtractedRelation
}

// EnhancedEpisodeData holds the LLM-enhanced episode fields.
type EnhancedEpisodeData struct {
	Title          string
	Goal           string
	Outcome        string
	OutcomeSummary string
	KeyDecisions   []string
	KeyArtifacts   []string
	Importance     float64
	Confidence     float64
}

// ExtractedEntity represents a named entity extracted from conversation.
type ExtractedEntity struct {
	Name        string
	EntityType  string // "person", "preference", "concept", "project"
	Description string
	Confidence  float64
}

// ExtractedRelation represents a relationship between two entities.
type ExtractedRelation struct {
	SourceEntity string
	TargetEntity string
	RelationType string // "depends_on", "implements", "references", "contains", "knows_as", "prefers"
	Confidence   float64
}

// EnhancedTextExtractor is the interface for LLM-based enhanced extraction.
type EnhancedTextExtractor interface {
	ExtractEnhanced(ctx context.Context, input ConsolidateInput) (*EnhancedExtractionResult, error)
}

// PathBExtractor orchestrates Path B enhanced extraction and writes results to L4.
type PathBExtractor struct {
	extractor EnhancedTextExtractor
	l4Writer  L4EntityWriter
	lg        loggateway.Logger
}

// NewPathBExtractor creates a new PathBExtractor.
func NewPathBExtractor(extractor EnhancedTextExtractor, l4Writer L4EntityWriter, lg loggateway.Logger) *PathBExtractor {
	return &PathBExtractor{
		extractor: extractor,
		l4Writer:  l4Writer,
		lg:        lg,
	}
}

// Extract performs the enhanced extraction and returns the result.
// Entity and relation writing to L4 is done separately via WriteEntities.
func (pe *PathBExtractor) Extract(ctx context.Context, input ConsolidateInput) (*EnhancedExtractionResult, error) {
	if pe == nil || pe.extractor == nil {
		return nil, nil
	}
	return pe.extractor.ExtractEnhanced(ctx, input)
}

// WriteEntities writes extracted entities and relations to the L4 graph.
// This is best-effort: errors are logged but do not fail the caller.
func (pe *PathBExtractor) WriteEntities(ctx context.Context, agentID, userID string, result *EnhancedExtractionResult) {
	if pe == nil || pe.l4Writer == nil || result == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}

	// Build a name→entityID map so relations can reference the correct IDs.
	entityIDMap := make(map[string]string, len(result.Entities))
	anchorID := userProfileEntityID(agentID)

	// Ensure the anchor entity exists.
	written := 0
	for _, ent := range result.Entities {
		name := strings.TrimSpace(ent.Name)
		if name == "" {
			continue
		}
		entID := fmt.Sprintf("l4-%s-%s-%s", ent.EntityType, agentID, slugEntityName(name))
		entityIDMap[strings.ToLower(name)] = entID

		if err := pe.l4Writer.UpsertEntity(ctx, L4EntityWrite{
			ID:             entID,
			ScopeType:      "agent",
			ScopeID:        agentID,
			UserID:         userID,
			EntityType:     ent.EntityType,
			Name:           truncateRunes(name, 80),
			NameNormalized: strings.ToLower(truncateRunes(name, 80)),
			Description:    ent.Description,
			Importance:     entityImportance(ent.EntityType),
			Confidence:     ent.Confidence,
			MetadataJSON:   `{"source":"path_b_enhanced"}`,
		}); err != nil {
			pe.lg.Warn("PathB: failed to upsert entity",
				loggateway.StepID("memory.path_b_entity_fail"),
				loggateway.Str("entity_id", entID),
				loggateway.Err(err))
			continue
		}

		// Create relation from anchor to entity (knows_as or prefers).
		relType := "knows_as"
		if ent.EntityType == "preference" {
			relType = "prefers"
		}
		if err := pe.l4Writer.UpsertRelation(ctx, L4RelationWrite{
			ScopeType:    "agent",
			ScopeID:      agentID,
			SourceID:     anchorID,
			TargetID:     entID,
			RelationType: relType,
			Weight:       1.0,
			Confidence:   ent.Confidence,
		}); err != nil {
			pe.lg.Warn("PathB: failed to upsert anchor relation",
				loggateway.StepID("memory.path_b_anchor_rel_fail"),
				loggateway.Str("entity_id", entID),
				loggateway.Err(err))
		}
		written++
	}

	// Write inter-entity relations.
	for _, rel := range result.Relations {
		srcID, ok := entityIDMap[strings.ToLower(strings.TrimSpace(rel.SourceEntity))]
		if !ok {
			continue
		}
		tgtID, ok := entityIDMap[strings.ToLower(strings.TrimSpace(rel.TargetEntity))]
		if !ok {
			continue
		}
		if err := pe.l4Writer.UpsertRelation(ctx, L4RelationWrite{
			ScopeType:    "agent",
			ScopeID:      agentID,
			SourceID:     srcID,
			TargetID:     tgtID,
			RelationType: rel.RelationType,
			Weight:       rel.Confidence,
			Confidence:   rel.Confidence,
		}); err != nil {
			pe.lg.Warn("PathB: failed to upsert inter-entity relation",
				loggateway.StepID("memory.path_b_rel_fail"),
				loggateway.Str("source", srcID),
				loggateway.Str("target", tgtID),
				loggateway.Err(err))
		}
	}

	if written > 0 {
		pe.lg.Info("PathB: wrote entities to L4",
			loggateway.StepID("memory.path_b_entities_written"),
			loggateway.Int("count", written),
			loggateway.Int("relations", len(result.Relations)))
	}
}

// entityImportance returns a default importance for an entity type.
func entityImportance(entityType string) float64 {
	switch entityType {
	case "person":
		return 0.85
	case "preference":
		return 0.7
	case "project":
		return 0.75
	case "concept":
		return 0.6
	default:
		return 0.5
	}
}
