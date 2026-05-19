package sessionmemory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RelationParams is an L4 graph edge upsert payload.
type RelationParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	SourceID         string
	TargetID         string
	RelationType     string
	Weight           float64
	Confidence       float64
	MetadataJSON     string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

func (st *Store) UpsertRelation(ctx context.Context, params RelationParams) error {
	if st == nil || st.client == nil {
		return nil
	}
	p := params
	if strings.TrimSpace(p.SourceID) == "" || strings.TrimSpace(p.TargetID) == "" {
		return fmt.Errorf("sessionmemory: relation source and target required")
	}
	if strings.TrimSpace(p.ID) == "" {
		p.ID = uuid.NewString()
	}
	p.ScopeType = strings.TrimSpace(p.ScopeType)
	if p.ScopeType == "" {
		p.ScopeType = "agent"
	}
	p.ScopeID = strings.TrimSpace(p.ScopeID)
	p.RelationType = strings.TrimSpace(p.RelationType)
	if p.RelationType == "" {
		p.RelationType = "related_to"
	}
	now := strings.TrimSpace(p.UpdatedAtRFC3339)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	created := strings.TrimSpace(p.CreatedAtRFC3339)
	if created == "" {
		created = now
	}
	meta := strings.TrimSpace(p.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	if p.Weight <= 0 {
		p.Weight = 1.0
	}
	if p.Confidence <= 0 {
		p.Confidence = 0.7
	}
	const q = `
INSERT INTO memory_relations (
  id, scope_type, scope_id, workspace_id, source_id, target_id, relation_type,
  bidirectional, weight, confidence, importance, use_count, attributes_json, evidence_json,
  status, source_kind, metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (?,?,?,?,?,?,?,0,?,?,0.5,0,'{}','[]','active','extracted',?,?,?,'')
ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
  weight=excluded.weight,
  confidence=excluded.confidence,
  metadata_json=excluded.metadata_json,
  updated_at=excluded.updated_at`
	_, err := st.client.ExecContext(ctx, q,
		p.ID, p.ScopeType, p.ScopeID, strings.TrimSpace(p.WorkspaceID),
		p.SourceID, p.TargetID, p.RelationType,
		p.Weight, p.Confidence, meta, created, now,
	)
	return err
}
