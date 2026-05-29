package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var ErrCascadeUnavailable = kerrors.BadRequest("MEMORY", "cascade store not available")

var ErrCascadeSagaInProgress = kerrors.Conflict("MEMORY", "cascade saga already in progress")

type CascadeProposalStore interface {
	InsertCascadeProposal(ctx context.Context, in CascadeProposalInsert) ([]byte, error)
	ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error)
	UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error)
}

type CascadeGraphReader interface {
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error)
	GetEntityRow(ctx context.Context, id string) ([]byte, error)
}

type CascadeFactMutator interface {
	ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error)
	SaveCascadeOriginalStatements(ctx context.Context, agentID, oldName string, factIDs []string) error
	RevertCascadeFactStatements(ctx context.Context, agentID string) (int, error)
	ListCascadeFactDiffs(ctx context.Context, agentID, oldName, newName string, limit int) ([]map[string]any, error)
	MarkFactsIndexStaleByAgent(ctx context.Context, agentID string) (int64, error)
}

type CascadeSagaStore interface {
	InitCascadeSagaSteps(ctx context.Context, proposalID string, steps []CascadeSagaStep) error
	GetCascadeSagaSteps(ctx context.Context, proposalID string) ([]CascadeSagaStep, error)
	UpdateSagaStepState(ctx context.Context, stepID int64, state, errMsg string) error
	UpdateSagaStepResult(ctx context.Context, stepID int64, resultJSON string) error
	HasCascadeSaga(ctx context.Context, proposalID string) (bool, error)
}

// CascadeGraphStore is a convenience aggregate for Wire binding.
// Deprecated: consumers should depend on individual sub-interfaces (CascadeProposalStore, CascadeGraphReader, CascadeFactMutator, CascadeSagaStore).
type CascadeGraphStore interface {
	CascadeProposalStore
	CascadeGraphReader
	CascadeFactMutator
	CascadeSagaStore
}

func NewL4CascadeUsecase(proposals CascadeProposalStore, reader CascadeGraphReader, mutator CascadeFactMutator, saga CascadeSagaStore, entityWriter L4EntityWriter) *L4CascadeUsecase {
	if proposals == nil {
		return nil
	}
	return &L4CascadeUsecase{
		proposals:    proposals,
		reader:       reader,
		mutator:      mutator,
		saga:         saga,
		entityWriter: entityWriter,
	}
}

type CascadeSagaStep struct {
	ID          int64  `json:"id"`
	ProposalID  string `json:"proposal_id"`
	StepIndex   int    `json:"step_index"`
	StepName    string `json:"step_name"`
	State       string `json:"state"`
	IsCritical  bool   `json:"is_critical"`
	Attempts    int    `json:"attempts"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	ResultJSON  string `json:"result_json,omitempty"`
	Error       string `json:"error,omitempty"`
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

type CascadePreview struct {
	AffectedEntitiesCount int               `json:"affected_entities_count"`
	AffectedFactsCount    int               `json:"affected_facts_count"`
	FactDiffs             []CascadeFactDiff `json:"fact_diffs"`
	EntityRenames         []CascadeEntityRename `json:"entity_renames"`
}

type CascadeFactDiff struct {
	FactID          string `json:"fact_id"`
	BeforeStatement string `json:"before_statement"`
	AfterStatement  string `json:"after_statement"`
	Scope           string `json:"scope"`
}

type CascadeEntityRename struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
	OldName    string `json:"old_name"`
	NewName    string `json:"new_name"`
}

const (
	SagaStepUpsertEntity   = "upsert_entity"
	SagaStepTouchAffected  = "touch_affected"
	SagaStepReplaceFacts   = "replace_facts"
	SagaStepSyncIndex      = "sync_index"
)

type L4CascadeUsecase struct {
	proposals     CascadeProposalStore
	reader        CascadeGraphReader
	mutator       CascadeFactMutator
	saga          CascadeSagaStore
	entityWriter  L4EntityWriter
	indexSync     MemoryFactIndexSyncer
	indexMu   sync.RWMutex
}

func (uc *L4CascadeUsecase) SetIndexSync(sync MemoryFactIndexSyncer) {
	if uc != nil {
		uc.indexMu.Lock()
		uc.indexSync = sync
		uc.indexMu.Unlock()
	}
}

func (uc *L4CascadeUsecase) ProposeNameConflict(ctx context.Context, agentID, entityID, oldName, newName string) error {
	if uc == nil || uc.proposals == nil {
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
	_, err = uc.proposals.InsertCascadeProposal(ctx, CascadeProposalInsert{
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
	raw, err := uc.reader.NeighborhoodJSON(ctx, centerID, int32(hops), int32(maxNodes), "")
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
	if uc == nil || uc.proposals == nil {
		return nil, nil
	}
	return uc.proposals.ListCascadeProposalRows(ctx, agentID, status, limit)
}

func (uc *L4CascadeUsecase) Preview(ctx context.Context, id string) (*CascadePreview, error) {
	if uc == nil || uc.proposals == nil {
		return nil, ErrCascadeUnavailable
	}
	raw, err := uc.proposals.GetCascadeProposalRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	agentID := jsonutil.IfaceStr(row, "agent_id")
	oldName := jsonutil.IfaceStr(row, "old_value")
	newName := jsonutil.IfaceStr(row, "new_value")
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")

	preview := &CascadePreview{}

	var affected []CascadeAffectedEntity
	rawAffected := jsonutil.IfaceStr(row, "affected_json")
	if rawAffected != "" && rawAffected != "[]" {
		if err := json.Unmarshal([]byte(rawAffected), &affected); err != nil {
			event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to unmarshal affected entities", event.P("raw", strutil.TruncateBytes(rawAffected, 80)), event.P("error", err.Error()))
		}
	}
	preview.AffectedEntitiesCount = len(affected)
	preview.EntityRenames = []CascadeEntityRename{{
		EntityID:   entityID,
		EntityType: jsonutil.IfaceStr(row, "trigger_attribute"),
		OldName:    oldName,
		NewName:    newName,
	}}

	if oldName != "" && newName != "" {
		diffs, err := uc.mutator.ListCascadeFactDiffs(ctx, agentID, oldName, newName, 50)
		if err != nil {
			return nil, err
		}
		preview.AffectedFactsCount = len(diffs)
		for _, d := range diffs {
			fid, _ := d["fact_id"].(string)
			before, _ := d["before_statement"].(string)
			after, _ := d["after_statement"].(string)
			scope, _ := d["scope"].(string)
			if before != after {
				preview.FactDiffs = append(preview.FactDiffs, CascadeFactDiff{
					FactID:          fid,
					BeforeStatement: before,
					AfterStatement:  after,
					Scope:           scope,
				})
			}
		}
	}

	return preview, nil
}

func (uc *L4CascadeUsecase) Approve(ctx context.Context, id, reviewer string) ([]byte, error) {
	if uc == nil || uc.proposals == nil || uc.entityWriter == nil {
		return nil, ErrCascadeUnavailable
	}
	raw, err := uc.proposals.GetCascadeProposalRow(ctx, id)
	if err != nil {
		return nil, err
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	status := jsonutil.IfaceStr(row, "status")
	if status == "applied" {
		return raw, nil
	}
	if status != "pending" && status != "partial" && status != "failed" {
		return raw, nil
	}

	hasSaga, err := uc.saga.HasCascadeSaga(ctx, id)
	if err != nil {
		return nil, err
	}

	agentID := jsonutil.IfaceStr(row, "agent_id")
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")
	oldName := jsonutil.IfaceStr(row, "old_value")
	newName := jsonutil.IfaceStr(row, "new_value")

	if !hasSaga {
		entRaw, err := uc.reader.GetEntityRow(ctx, entityID)
		if err != nil {
			return nil, err
		}
		entJSON := string(entRaw)
		affectedJSON := jsonutil.IfaceStr(row, "affected_json")
		replacePayload, _ := json.Marshal(map[string]string{"agent_id": agentID, "old_name": oldName, "new_name": newName})
		syncPayload, _ := json.Marshal(map[string]string{"agent_id": agentID})
		steps := []CascadeSagaStep{
			{StepName: SagaStepUpsertEntity, IsCritical: true, PayloadJSON: entJSON},
			{StepName: SagaStepTouchAffected, IsCritical: false, PayloadJSON: affectedJSON},
			{StepName: SagaStepReplaceFacts, IsCritical: true, PayloadJSON: string(replacePayload)},
			{StepName: SagaStepSyncIndex, IsCritical: false, PayloadJSON: string(syncPayload)},
		}
		if err := uc.saga.InitCascadeSagaSteps(ctx, id, steps); err != nil {
			return nil, err
		}
	}

	sagaSteps, err := uc.saga.GetCascadeSagaSteps(ctx, id)
	if err != nil {
		return nil, err
	}

	allSucceeded := true
	anyFailed := false
	for i := range sagaSteps {
		step := &sagaSteps[i]
		if step.State == "succeeded" || step.State == "skipped" {
			continue
		}
		if step.State == "failed" || step.State == "pending" || step.State == "running" {
			if err := uc.executeSagaStep(ctx, step, row); err != nil {
				allSucceeded = false
				anyFailed = true
				if step.IsCritical {
					uc.compensateCompletedSteps(ctx, sagaSteps[:i])
					break
				}
			}
		}
	}

	if allSucceeded {
		return uc.proposals.UpdateCascadeProposalStatus(ctx, id, "applied", reviewer, "")
	}
	if anyFailed {
		note := "saga partial failure"
		if _, err := uc.proposals.UpdateCascadeProposalStatus(ctx, id, "partial", reviewer, note); err != nil {
			return nil, err
		}
		return uc.proposals.GetCascadeProposalRow(ctx, id)
	}
	return uc.proposals.GetCascadeProposalRow(ctx, id)
}

func (uc *L4CascadeUsecase) executeSagaStep(ctx context.Context, step *CascadeSagaStep, row map[string]any) error {
	if err := uc.saga.UpdateSagaStepState(ctx, step.ID, "running", ""); err != nil {
		return err
	}

	var execErr error
	switch step.StepName {
	case SagaStepUpsertEntity:
		execErr = uc.execUpsertEntity(ctx, row)
	case SagaStepTouchAffected:
		execErr = uc.execTouchAffected(ctx, row)
	case SagaStepReplaceFacts:
		execErr = uc.execReplaceFacts(ctx, row)
	case SagaStepSyncIndex:
		execErr = uc.execSyncIndex(ctx, row)
	default:
		execErr = kerrors.BadRequest("MEMORY", "unknown saga step")
	}

	if execErr != nil {
		if err := uc.saga.UpdateSagaStepState(ctx, step.ID, "failed", execErr.Error()); err != nil {
			event.SysLogWarn("system.memory.saga_step_fail", "failed to mark saga step as failed",
				event.P("step_id", step.ID), event.P("error", err.Error()))
		}
		return execErr
	}
	return uc.saga.UpdateSagaStepState(ctx, step.ID, "succeeded", "")
}

func (uc *L4CascadeUsecase) execUpsertEntity(ctx context.Context, row map[string]any) error {
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")
	agentID := jsonutil.IfaceStr(row, "agent_id")
	newName := jsonutil.IfaceStr(row, "new_value")
	entRaw, err := uc.reader.GetEntityRow(ctx, entityID)
	if err != nil {
		return err
	}
	var ent map[string]any
	if err := json.Unmarshal(entRaw, &ent); err != nil {
		return err
	}
	return uc.entityWriter.UpsertEntity(ctx, L4EntityWrite{
		ID:             entityID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         jsonutil.IfaceStr(ent, "user_id"),
		EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
		Name:           newName,
		NameNormalized: strings.ToLower(newName),
		Description:    jsonutil.IfaceStr(ent, "description"),
		Importance:     l4CascadeEntImportance,
		Confidence:     l4CascadeEntConfidence,
		MetadataJSON:   mergeCascadeAppliedMeta(jsonutil.IfaceStr(ent, "metadata_json"), newName),
	})
}

func (uc *L4CascadeUsecase) execTouchAffected(ctx context.Context, row map[string]any) error {
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")
	newName := jsonutil.IfaceStr(row, "new_value")
	return uc.touchAffectedEntities(ctx, row, entityID, newName)
}

func (uc *L4CascadeUsecase) execReplaceFacts(ctx context.Context, row map[string]any) error {
	agentID := jsonutil.IfaceStr(row, "agent_id")
	oldName := jsonutil.IfaceStr(row, "old_value")
	newName := jsonutil.IfaceStr(row, "new_value")
	if oldName == "" || newName == "" {
		return nil
	}
	diffs, err := uc.mutator.ListCascadeFactDiffs(ctx, agentID, oldName, newName, 50)
	if err != nil {
		return err
	}
	var factIDs []string
	for _, d := range diffs {
		fid, _ := d["fact_id"].(string)
		before, _ := d["before_statement"].(string)
		after, _ := d["after_statement"].(string)
		if fid != "" && before != after {
			factIDs = append(factIDs, fid)
		}
	}
	if err := uc.mutator.SaveCascadeOriginalStatements(ctx, agentID, oldName, factIDs); err != nil {
		event.SysLogWarn("system.auto_memory.l4_fail", "save_cascade_original_statements_failed", event.P("error", err.Error()))
	}
	updatedRows, _, err := uc.mutator.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
	if err != nil {
		return err
	}
	resultJSON, _ := json.Marshal(map[string]any{"updated_count": len(updatedRows)})
	sagaSteps, err := uc.saga.GetCascadeSagaSteps(ctx, jsonutil.IfaceStr(row, "id"))
	if err != nil {
		event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to get saga steps for replace facts", event.P("proposal_id", jsonutil.IfaceStr(row, "id")), event.P("error", err.Error()))
		return nil
	}
	for _, s := range sagaSteps {
		if s.StepName == SagaStepReplaceFacts && s.State == "running" {
			if err := uc.saga.UpdateSagaStepResult(ctx, s.ID, string(resultJSON)); err != nil {
				event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to update saga step result", event.P("step_id", s.ID), event.P("error", err.Error()))
			}
			break
		}
	}
	return nil
}

func (uc *L4CascadeUsecase) execSyncIndex(ctx context.Context, row map[string]any) error {
	agentID := jsonutil.IfaceStr(row, "agent_id")
	uc.indexMu.RLock()
	syncer := uc.indexSync
	uc.indexMu.RUnlock()
	if agentID == "" || syncer == nil {
		return nil
	}
	marked, err := uc.mutator.MarkFactsIndexStaleByAgent(ctx, agentID)
	if err != nil {
		return err
	}
	resultJSON, _ := json.Marshal(map[string]any{"stale_marked": marked})
	sagaSteps, err := uc.saga.GetCascadeSagaSteps(ctx, jsonutil.IfaceStr(row, "id"))
	if err != nil {
		event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to get saga steps for sync index", event.P("proposal_id", jsonutil.IfaceStr(row, "id")), event.P("error", err.Error()))
		return nil
	}
	for _, s := range sagaSteps {
		if s.StepName == SagaStepSyncIndex && s.State == "running" {
			if err := uc.saga.UpdateSagaStepResult(ctx, s.ID, string(resultJSON)); err != nil {
				event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to update saga step result for sync index", event.P("step_id", s.ID), event.P("error", err.Error()))
			}
			break
		}
	}
	return nil
}

func (uc *L4CascadeUsecase) compensateCompletedSteps(ctx context.Context, steps []CascadeSagaStep) {
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		if s.State != "succeeded" {
			continue
		}
		switch s.StepName {
		case SagaStepReplaceFacts:
			uc.compensateReplaceFacts(ctx, s)
		case SagaStepUpsertEntity:
			uc.compensateUpsertEntity(ctx, s)
		case SagaStepSyncIndex:
			uc.compensateSyncIndex(ctx, s)
		}
		if err := uc.saga.UpdateSagaStepState(ctx, s.ID, "compensated", ""); err != nil {
			event.SysLogWarn("system.auto_memory.cascade_fail", "Cascade: failed to update saga step state to compensated", event.P("step_id", s.ID), event.P("error", err.Error()))
		}
	}
}

func (uc *L4CascadeUsecase) compensateReplaceFacts(ctx context.Context, step CascadeSagaStep) {
	var payload struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(step.PayloadJSON), &payload); err != nil || payload.AgentID == "" {
		return
	}
	reverted, err := uc.mutator.RevertCascadeFactStatements(ctx, payload.AgentID)
	if err != nil {
		event.SysLogWarn("system.auto_memory.l4_fail", "compensate_replace_facts_failed", event.P("error", err.Error()))
		return
	}
	event.SysLogWarn("system.auto_memory.l4", "compensate_replace_facts_reverted", event.P("reverted", reverted))
}

func (uc *L4CascadeUsecase) compensateUpsertEntity(ctx context.Context, step CascadeSagaStep) {
	event.SysLogWarn("system.auto_memory.l4", "compensate_upsert_entity_skipped", event.P("step_id", step.ID))
}

func (uc *L4CascadeUsecase) compensateSyncIndex(ctx context.Context, step CascadeSagaStep) {
	var payload struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(step.PayloadJSON), &payload); err != nil || payload.AgentID == "" {
		return
	}
	marked, err := uc.mutator.MarkFactsIndexStaleByAgent(ctx, payload.AgentID)
	if err != nil {
		event.SysLogWarn("system.auto_memory.l4_fail", "compensate_sync_index_failed", event.P("error", err.Error()))
		return
	}
	event.SysLogWarn("system.auto_memory.l4", "compensate_sync_index_marked_stale", event.P("marked", marked))
}

func (uc *L4CascadeUsecase) GetSagaSteps(ctx context.Context, proposalID string) ([]CascadeSagaStep, error) {
	if uc == nil || uc.saga == nil {
		return nil, ErrCascadeUnavailable
	}
	return uc.saga.GetCascadeSagaSteps(ctx, proposalID)
}

func (uc *L4CascadeUsecase) Retry(ctx context.Context, id, reviewer string) ([]byte, error) {
	if uc == nil || uc.proposals == nil || uc.entityWriter == nil {
		return nil, ErrCascadeUnavailable
	}
	return uc.Approve(ctx, id, reviewer)
}

func (uc *L4CascadeUsecase) Compensate(ctx context.Context, id, reviewer string) ([]byte, error) {
	if uc == nil || uc.saga == nil || uc.proposals == nil {
		return nil, ErrCascadeUnavailable
	}
	sagaSteps, err := uc.saga.GetCascadeSagaSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	uc.compensateCompletedSteps(ctx, sagaSteps)
	return uc.proposals.UpdateCascadeProposalStatus(ctx, id, "failed", reviewer, "manual compensate")
}

func (uc *L4CascadeUsecase) touchAffectedEntities(ctx context.Context, row map[string]any, triggerID, newName string) error {
	if uc == nil || uc.entityWriter == nil || uc.reader == nil {
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
		entRaw, err := uc.reader.GetEntityRow(ctx, id)
		if err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		var ent map[string]any
		if err := json.Unmarshal(entRaw, &ent); err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		if err := uc.entityWriter.UpsertEntity(ctx, L4EntityWrite{
			ID:             id,
			ScopeType:      "agent",
			ScopeID:        jsonutil.IfaceStr(row, "agent_id"),
			UserID:         jsonutil.IfaceStr(ent, "user_id"),
			EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
			Name:           jsonutil.IfaceStr(ent, "name"),
			NameNormalized: jsonutil.IfaceStr(ent, "name_normalized"),
			Description:    jsonutil.IfaceStr(ent, "description"),
			Importance:     l4CascadeTouchImportance,
			Confidence:     l4CascadeTouchConfidence,
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
	if uc == nil || uc.proposals == nil {
		return nil, ErrCascadeUnavailable
	}
	note := strings.TrimSpace(reason)
	if note == "" {
		note = "rejected"
	}
	return uc.proposals.UpdateCascadeProposalStatus(ctx, id, "rejected", reviewer, note)
}

func cascadeNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
