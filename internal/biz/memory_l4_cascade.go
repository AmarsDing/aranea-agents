package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/jsonutil"
)

var ErrCascadeUnavailable = errors.New("memory: cascade store not available")

// CascadeGraphStore supports cascade proposal persistence and neighborhood BFS.
type CascadeGraphStore interface {
	InsertCascadeProposal(ctx context.Context, in CascadeProposalInsert) ([]byte, error)
	ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error)
	UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error)
	GetEntityRow(ctx context.Context, id string) ([]byte, error)
	ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error)
}

type CascadeProposalInsert struct {
	AgentID           string
	WorkspaceID       string
	TriggerEntityID   string
	TriggerEntityName string
	TriggerAttribute  string
	OldValue          string
	NewValue          string
	AffectedJSON      string
	RiskLevel         string
	Rationale         string
	MetadataJSON      string
	ExpiresAt         string
}

type CascadeAffectedEntity struct {
	EntityID     string `json:"entity_id"`
	EntityName   string `json:"entity_name"`
	EntityType   string `json:"entity_type"`
	RelationType string `json:"relation_type"`
	Hops         int    `json:"hops"`
}

type L4CascadeUsecase struct {
	store     CascadeGraphStore
	graph     L4GraphRepo
	indexSync MemoryFactIndexSyncer
}

func NewL4CascadeUsecase(store CascadeGraphStore, graph L4GraphRepo) *L4CascadeUsecase {
	if store == nil {
		return nil
	}
	return &L4CascadeUsecase{store: store, graph: graph}
}

// SetIndexSync wires pgvector re-indexing after cascade fact renames.
func (uc *L4CascadeUsecase) SetIndexSync(sync MemoryFactIndexSyncer) {
	if uc != nil {
		uc.indexSync = sync
	}
}

func (uc *L4CascadeUsecase) ProposeNameConflict(ctx context.Context, agentID, entityID, oldName, newName string) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	agentID = strings.TrimSpace(agentID)
	entityID = strings.TrimSpace(entityID)
	if agentID == "" || entityID == "" || strings.EqualFold(strings.TrimSpace(oldName), strings.TrimSpace(newName)) {
		return nil
	}
	affected, err := uc.collectAffected(ctx, entityID, 2, 20)
	if err != nil {
		return err
	}
	affectedJSON, err := json.Marshal(affected)
	if err != nil {
		return err
	}
	_, err = uc.store.InsertCascadeProposal(ctx, CascadeProposalInsert{
		AgentID:           agentID,
		TriggerEntityID:   entityID,
		TriggerEntityName: strings.TrimSpace(newName),
		TriggerAttribute:  "name",
		OldValue:          strings.TrimSpace(oldName),
		NewValue:          strings.TrimSpace(newName),
		AffectedJSON:      string(affectedJSON),
		RiskLevel:         cascadeRiskLevel(len(affected)),
		Rationale:         "Detected conflicting person name during auto-memory consolidation",
		MetadataJSON:      `{"source":"auto_memory","kind":"name_conflict"}`,
	})
	return err
}

func cascadeRiskLevel(affectedCount int) string {
	switch {
	case affectedCount >= 5:
		return "high"
	case affectedCount >= 2:
		return "medium"
	default:
		return "low"
	}
}

func (uc *L4CascadeUsecase) collectAffected(ctx context.Context, centerID string, hops, maxNodes int) ([]CascadeAffectedEntity, error) {
	raw, err := uc.store.NeighborhoodJSON(ctx, centerID, int32(hops), int32(maxNodes), "")
	if err != nil {
		return nil, err
	}
	var nb struct {
		Entities  []map[string]any `json:"entities"`
		Relations []map[string]any `json:"relations"`
	}
	if err := json.Unmarshal(raw, &nb); err != nil {
		return nil, err
	}
	relByTarget := map[string]string{}
	for _, rel := range nb.Relations {
		tgt := jsonutil.IfaceStr(rel, "target_id")
		if tgt != "" {
			relByTarget[tgt] = jsonutil.IfaceStr(rel, "relation_type")
		}
	}
	out := make([]CascadeAffectedEntity, 0, len(nb.Entities))
	for _, ent := range nb.Entities {
		id := jsonutil.IfaceStr(ent, "id")
		if id == "" || id == centerID {
			continue
		}
		out = append(out, CascadeAffectedEntity{
			EntityID:     id,
			EntityName:   jsonutil.IfaceStr(ent, "name"),
			EntityType:   jsonutil.IfaceStr(ent, "entity_type"),
			RelationType: relByTarget[id],
			Hops:         cascadeEntityHops(ent),
		})
	}
	return out, nil
}

func cascadeEntityHops(ent map[string]any) int {
	if h := jsonutil.IfaceInt(ent, "hop"); h > 0 {
		return h
	}
	return 1
}

func (uc *L4CascadeUsecase) ListRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	return uc.store.ListCascadeProposalRows(ctx, agentID, status, limit)
}

func (uc *L4CascadeUsecase) Approve(ctx context.Context, id, reviewer string) ([]byte, error) {
	if uc == nil || uc.store == nil || uc.graph == nil {
		return nil, ErrCascadeUnavailable
	}
	raw, err := uc.store.GetCascadeProposalRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	if jsonutil.IfaceStr(row, "status") != "pending" {
		return raw, nil
	}
	agentID := jsonutil.IfaceStr(row, "agent_id")
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")
	oldName := jsonutil.IfaceStr(row, "old_value")
	newName := jsonutil.IfaceStr(row, "new_value")
	entRaw, err := uc.store.GetEntityRow(ctx, entityID)
	if err != nil {
		return nil, err
	}
	var ent map[string]any
	if err := json.Unmarshal(entRaw, &ent); err != nil {
		return nil, err
	}
	if err := uc.graph.UpsertEntity(ctx, L4EntityWrite{
		ID:             entityID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         jsonutil.IfaceStr(ent, "user_id"),
		EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
		Name:           newName,
		NameNormalized: strings.ToLower(newName),
		Description:    jsonutil.IfaceStr(ent, "description"),
		Importance:     0.85,
		Confidence:     0.8,
		MetadataJSON:   mergeCascadeAppliedMeta(jsonutil.IfaceStr(ent, "metadata_json"), newName),
	}); err != nil {
		return nil, err
	}
	if err := uc.touchAffectedEntities(ctx, row, entityID, newName); err != nil {
		return nil, err
	}
	if oldName != "" && newName != "" {
		updatedRows, _, err := uc.store.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
		if err != nil {
			return nil, err
		}
		// MEM-OPT-01: log index sync failures instead of silently ignoring them.
		// Each fact that fails to sync gets a warn-level log so operators can
		// detect index drift without blocking the Approve write path.
		if uc.indexSync != nil {
			for _, raw := range updatedRows {
				if err := uc.indexSync.SyncFactIndexFromRow(ctx, raw); err != nil {
					event.SysLogWarn("system.auto_memory.l4_fail", "memory.cascade_approve.index_sync_fail", event.P("error", err.Error()))
				}
			}
		}
	}
	return uc.store.UpdateCascadeProposalStatus(ctx, id, "applied", reviewer, "")
}

func (uc *L4CascadeUsecase) touchAffectedEntities(ctx context.Context, row map[string]any, triggerID, newName string) error {
	if uc == nil || uc.graph == nil || uc.store == nil {
		return nil
	}
	rawAffected := jsonutil.IfaceStr(row, "affected_json")
	if rawAffected == "" || rawAffected == "[]" {
		return nil
	}
	var affected []CascadeAffectedEntity
	if err := json.Unmarshal([]byte(rawAffected), &affected); err != nil {
		return err
	}
	var failedIDs []string
	for _, aff := range affected {
		id := strings.TrimSpace(aff.EntityID)
		if id == "" || id == triggerID {
			continue
		}
		entRaw, err := uc.store.GetEntityRow(ctx, id)
		if err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		var ent map[string]any
		if err := json.Unmarshal(entRaw, &ent); err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		if err := uc.graph.UpsertEntity(ctx, L4EntityWrite{
			ID:             id,
			ScopeType:      "agent",
			ScopeID:        jsonutil.IfaceStr(row, "agent_id"),
			UserID:         jsonutil.IfaceStr(ent, "user_id"),
			EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
			Name:           jsonutil.IfaceStr(ent, "name"),
			NameNormalized: jsonutil.IfaceStr(ent, "name_normalized"),
			Description:    jsonutil.IfaceStr(ent, "description"),
			Importance:     0.5,
			Confidence:     0.7,
			MetadataJSON:   mergeCascadeLinkedMeta(jsonutil.IfaceStr(ent, "metadata_json"), triggerID, newName),
		}); err != nil {
			failedIDs = append(failedIDs, id)
		}
	}
	if len(failedIDs) > 0 {
		event.SysLogWarn("system.auto_memory.l4_fail", "touchAffectedEntities: some entities failed", event.P("trigger_id", triggerID), event.P("failed_ids", fmt.Sprint(failedIDs)))
	}
	return nil
}

func mergeCascadeAppliedMeta(base, newName string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["source"] = "cascade_approve"
	m["cascade_applied_name"] = newName
	delete(m, "pending_name")
	delete(m, "gate")
	delete(m, "conflict")
	b, _ := json.Marshal(m)
	return string(b)
}

func mergeCascadeLinkedMeta(base, triggerID, newName string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["cascade_linked_trigger_id"] = triggerID
	m["cascade_linked_name"] = newName
	b, _ := json.Marshal(m)
	return string(b)
}

func (uc *L4CascadeUsecase) Reject(ctx context.Context, id, reviewer, reason string) ([]byte, error) {
	if uc == nil || uc.store == nil {
		return nil, ErrCascadeUnavailable
	}
	note := strings.TrimSpace(reason)
	if note == "" {
		note = "rejected"
	}
	return uc.store.UpdateCascadeProposalStatus(ctx, id, "rejected", reviewer, note)
}
