package sessionmemory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	scopeTypeSession = "session"
	entityTypeEvent  = "event"
	sourceSync       = "session_sync"
)

// EventEntityParams is row input for UpsertEventEntity.
type EventEntityParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	UserID           string
	EntityType       string
	Name             string
	NameNormalized   string
	Description      string
	Importance       float64
	Confidence       float64
	UseCount         int
	MetadataJSON     string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

// UpsertEventEntity stores one conversation event in memory_entities so keyword search can surface it.
// name_normalized must be stable per (session scope, entity_type, logical event) for upserts across turns.
func (st *Store) UpsertEventEntity(ctx context.Context, params EventEntityParams) error {
	if st == nil || st.client == nil {
		return nil
	}
	p := params
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("sessionmemory: entity id required")
	}
	p.ScopeType = strings.TrimSpace(p.ScopeType)
	if p.ScopeType == "" {
		p.ScopeType = scopeTypeSession
	}
	p.ScopeID = strings.TrimSpace(p.ScopeID)
	p.UserID = strings.TrimSpace(p.UserID)
	p.EntityType = strings.TrimSpace(p.EntityType)
	if p.EntityType == "" {
		p.EntityType = entityTypeEvent
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		p.Name = p.NameNormalized
	}
	p.NameNormalized = strings.TrimSpace(strings.ToLower(p.NameNormalized))
	if p.NameNormalized == "" {
		p.NameNormalized = "event"
	}
	now := strings.TrimSpace(p.UpdatedAtRFC3339)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	created := strings.TrimSpace(p.CreatedAtRFC3339)
	if created == "" {
		created = now
	}
	meta := strings.TrimSpace(params.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	if p.Importance <= 0 {
		p.Importance = 0.5
	}
	if p.Confidence <= 0 {
		p.Confidence = 0.7
	}

	const q = `
INSERT INTO memory_entities (
  id, scope_type, scope_id, workspace_id, user_id,
  entity_type, name, name_normalized, aliases_json, description, attributes_json,
  importance, confidence, use_count, source_kind,
  embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
  status, merged_into, metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, entity_type, name_normalized) DO UPDATE SET
  name=excluded.name,
  description=excluded.description,
  importance=excluded.importance,
  metadata_json=excluded.metadata_json,
  user_id=excluded.user_id,
  updated_at=excluded.updated_at,
  embedding_status=excluded.embedding_status
`
	args := []any{
		p.ID, p.ScopeType, p.ScopeID, strings.TrimSpace(p.WorkspaceID), p.UserID,
		p.EntityType, p.Name, p.NameNormalized, "[]", p.Description, "{}",
		p.Importance, p.Confidence, p.UseCount, sourceSync,
		"pending", "", 0, nil, 0.0,
		"active", "", meta, created, now, "", "",
	}
	_, err := st.client.ExecContext(ctx, q, args...)
	return err
}

// DeleteSessionEventEntities removes keyword-memory rows produced by session sync for one session.
func (st *Store) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	if st == nil || st.client == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	_, err := st.client.ExecContext(ctx,
		`DELETE FROM memory_entities WHERE scope_type = ? AND scope_id = ? AND entity_type = ?`,
		scopeTypeSession, sessionID, entityTypeEvent)
	return err
}
