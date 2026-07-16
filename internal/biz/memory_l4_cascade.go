package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

var ErrCascadeUnavailable = apierror.BadRequest("MEMORY", "cascade store not available")

var ErrCascadeSagaInProgress = apierror.Conflict("MEMORY", "cascade saga already in progress")

// ErrCascadeRejectNotAllowed is returned when Reject CAS fails because the
// proposal is not in a rejectable status (pending/partial/failed).
var ErrCascadeRejectNotAllowed = apierror.Conflict("MEMORY", "cascade proposal cannot be rejected in its current status")

type CascadeProposalStore interface {
	InsertCascadeProposal(ctx context.Context, in CascadeProposalInsert) ([]byte, error)
	ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error)
	UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error)
	// CompareAndSwapProposalStatus atomically transitions a proposal from one of the
	// expected statuses to the target status. Returns the updated row and true on
	// success, or the current row and false if the current status doesn't match.
	CompareAndSwapProposalStatus(ctx context.Context, id string, fromStatuses []string, toStatus, reviewedBy, reviewNote string) ([]byte, bool, error)
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
	UpdateSagaStepState(ctx context.Context, stepID string, state, errMsg string) error
	UpdateSagaStepResult(ctx context.Context, stepID string, resultJSON string) error
	HasCascadeSaga(ctx context.Context, proposalID string) (bool, error)
}

// L4CascadeDeps aggregates the dependencies for L4CascadeUsecase, keeping the
// constructor parameter count within the CS-B7 limit (≤5) while allowing all
// required dependencies to be injected at construction time.
type L4CascadeDeps struct {
	Proposals    CascadeProposalStore
	Reader       CascadeGraphReader
	Mutator      CascadeFactMutator
	Saga         CascadeSagaStore
	EntityWriter L4EntityWriter
	IndexSync    MemoryFactIndexSyncer
	LG           loggateway.Logger
}

func NewL4CascadeUsecase(deps L4CascadeDeps) *L4CascadeUsecase {
	if deps.Proposals == nil {
		return nil
	}
	return &L4CascadeUsecase{
		proposals:    deps.Proposals,
		reader:       deps.Reader,
		mutator:      deps.Mutator,
		saga:         deps.Saga,
		entityWriter: deps.EntityWriter,
		indexSync:    deps.IndexSync,
		lg:           deps.LG,
	}
}

type CascadeSagaStep struct {
	ID          string `json:"id"`
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
	AffectedEntitiesCount int                   `json:"affected_entities_count"`
	AffectedFactsCount    int                   `json:"affected_facts_count"`
	FactDiffs             []CascadeFactDiff     `json:"fact_diffs"`
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
	SagaStepUpsertEntity  = "upsert_entity"
	SagaStepTouchAffected = "touch_affected"
	SagaStepReplaceFacts  = "replace_facts"
	SagaStepSyncIndex     = "sync_index"
)

type L4CascadeUsecase struct {
	proposals    CascadeProposalStore
	reader       CascadeGraphReader
	mutator      CascadeFactMutator
	saga         CascadeSagaStore
	entityWriter L4EntityWriter
	lg           loggateway.Logger
	indexSync    MemoryFactIndexSyncer
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
		uc.lg.Warn("解析 neighborhood json 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
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
		uc.lg.Warn("解析 cascade proposal row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
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
			uc.lg.Warn("Cascade: failed to unmarshal affected entities", loggateway.StepID("memory.cascade_fail"), loggateway.Str("raw", strutil.TruncateBytes(rawAffected, 80)), loggateway.Err(err))
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
		uc.lg.Warn("解析 cascade proposal row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return nil, err
	}
	status := jsonutil.IfaceStr(row, "status")
	if status == "applied" {
		return raw, nil
	}
	if status == "running" {
		return nil, ErrCascadeSagaInProgress
	}
	if status != "pending" && status != "partial" && status != "failed" {
		return raw, nil
	}

	// Guard against concurrent Approve calls: atomically transition to "running"
	// using Compare-And-Swap. Only proceeds if the current status is one of the
	// expected values (pending/partial/failed). If another process already moved
	// it to "running", the CAS fails and we return ErrCascadeSagaInProgress.
	raw, swapped, err := uc.proposals.CompareAndSwapProposalStatus(ctx, id,
		[]string{"pending", "partial", "failed"}, "running", reviewer, "saga execution started")
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, ErrCascadeSagaInProgress
	}
	row = nil
	if err := json.Unmarshal(raw, &row); err != nil {
		uc.lg.Warn("解析 cascade proposal row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return nil, err
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
		// Save old name into UpsertEntity payload so compensation can restore it.
		// Use a dedicated compensation payload with _old_name as a first-class field
		// rather than injecting it into the entity snapshot — this ensures the
		// compensation data survives payload mutations and is always recoverable.
		entRaw, err := uc.reader.GetEntityRow(ctx, entityID)
		if err != nil {
			return nil, err
		}
		var entForSnapshot map[string]any
		if err := json.Unmarshal(entRaw, &entForSnapshot); err != nil {
			uc.lg.Warn("解析 entity row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
			return nil, err
		}
		// Build compensation payload with explicit _old_name field.
		compPayload, err := mustMarshal(map[string]any{
			"entity_id":   entityID,
			"scope_type":  jsonutil.IfaceStr(entForSnapshot, "scope_type"),
			"scope_id":    jsonutil.IfaceStr(entForSnapshot, "scope_id"),
			"user_id":     jsonutil.IfaceStr(entForSnapshot, "user_id"),
			"entity_type": jsonutil.IfaceStr(entForSnapshot, "entity_type"),
			"old_name":    oldName,
			"description": jsonutil.IfaceStr(entForSnapshot, "description"),
			"metadata":    jsonutil.IfaceStr(entForSnapshot, "metadata_json"),
			"importance":  anyFloatRaw(entForSnapshot, "importance", l4CascadeEntImportance),
			"confidence":  anyFloatRaw(entForSnapshot, "confidence", l4CascadeEntConfidence),
		})
		if err != nil {
			return nil, err
		}
		affectedJSON := jsonutil.IfaceStr(row, "affected_json")
		replacePayload, err := mustMarshal(map[string]string{"agent_id": agentID, "old_name": oldName, "new_name": newName})
		if err != nil {
			return nil, err
		}
		syncPayload, err := mustMarshal(map[string]string{"agent_id": agentID})
		if err != nil {
			return nil, err
		}
		steps := []CascadeSagaStep{
			{StepName: SagaStepUpsertEntity, IsCritical: true, PayloadJSON: string(compPayload)},
			{StepName: SagaStepTouchAffected, IsCritical: false, PayloadJSON: affectedJSON},
			{StepName: SagaStepReplaceFacts, IsCritical: true, PayloadJSON: string(replacePayload)},
			{StepName: SagaStepSyncIndex, IsCritical: false, PayloadJSON: string(syncPayload)},
		}
		if err := uc.saga.InitCascadeSagaSteps(ctx, id, steps); err != nil {
			// Roll back "running" so Approve can be retried after init failure.
			if _, rbErr := uc.proposals.UpdateCascadeProposalStatus(ctx, id, status, reviewer, "saga init failed: "+err.Error()); rbErr != nil {
				uc.lg.Warn("cascade: failed to roll back running after saga init error",
					loggateway.StepID("memory.cascade_init_rollback"),
					loggateway.Str("proposal_id", id),
					loggateway.Err(rbErr))
			}
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
		if step.State == "failed" || step.State == "pending" || step.State == "running" ||
			step.State == "compensated" || step.State == "compensate_failed" {
			if err := uc.executeSagaStep(ctx, step, row); err != nil {
				allSucceeded = false
				anyFailed = true
				if step.IsCritical {
					uc.compensateCompletedSteps(ctx, sagaSteps[:i])
					break
				}
				// Non-critical step failed: log and continue, mark as partial later.
				uc.lg.Warn("saga non-critical step failed (continuing)",
					loggateway.StepID("memory.saga_non_critical_fail"),
					loggateway.Str("step", step.StepName),
					loggateway.Err(err))
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
	// Keep in-memory State in sync with DB so compensateCompletedSteps can see
	// successes from earlier steps in the same Approve call.
	step.State = "running"

	var execErr error
	switch step.StepName {
	case SagaStepUpsertEntity:
		execErr = uc.execUpsertEntity(ctx, row)
	case SagaStepTouchAffected:
		execErr = uc.execTouchAffected(ctx, row)
	case SagaStepReplaceFacts:
		execErr = uc.execReplaceFacts(ctx, step, row)
	case SagaStepSyncIndex:
		execErr = uc.execSyncIndex(ctx, step, row)
	default:
		execErr = apierror.BadRequest("MEMORY", "unknown saga step")
	}

	if execErr != nil {
		if err := uc.saga.UpdateSagaStepState(ctx, step.ID, "failed", execErr.Error()); err != nil {
			uc.lg.Warn("failed to mark saga step as failed",
				loggateway.StepID("memory.saga_step_fail"), loggateway.Str("step_id", fmt.Sprint(step.ID)), loggateway.Err(err))
		}
		step.State = "failed"
		return execErr
	}
	if err := uc.saga.UpdateSagaStepState(ctx, step.ID, "succeeded", ""); err != nil {
		return err
	}
	step.State = "succeeded"
	return nil
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
		uc.lg.Warn("解析 entity row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return err
	}
	// Preserve original importance/confidence from the entity row so the
	// cascade approval only changes the name without corrupting other fields.
	// Use anyFloatRaw (not anyFloatOr) to preserve zero values — an entity
	// with importance=0 or confidence=0 should keep that value, not be
	// overwritten with the fallback.
	importance := anyFloatRaw(ent, "importance", l4CascadeEntImportance)
	confidence := anyFloatRaw(ent, "confidence", l4CascadeEntConfidence)
	return uc.entityWriter.UpsertEntity(ctx, L4EntityWrite{
		ID:             entityID,
		ScopeType:      "agent",
		ScopeID:        agentID,
		UserID:         jsonutil.IfaceStr(ent, "user_id"),
		EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
		Name:           newName,
		NameNormalized: strings.ToLower(newName),
		Description:    jsonutil.IfaceStr(ent, "description"),
		Importance:     importance,
		Confidence:     confidence,
		MetadataJSON:   mergeCascadeAppliedMeta(jsonutil.IfaceStr(ent, "metadata_json"), newName, uc.lg),
	})
}

func (uc *L4CascadeUsecase) execTouchAffected(ctx context.Context, row map[string]any) error {
	entityID := jsonutil.IfaceStr(row, "trigger_entity_id")
	newName := jsonutil.IfaceStr(row, "new_value")
	return uc.touchAffectedEntities(ctx, row, entityID, newName)
}

func (uc *L4CascadeUsecase) execReplaceFacts(ctx context.Context, step *CascadeSagaStep, row map[string]any) error {
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
		return apierror.Internal("MEMORY", "save_cascade_original_statements: %s", err.Error())
	}
	updatedRows, _, err := uc.mutator.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(map[string]any{"updated_count": len(updatedRows)})
	if err != nil {
		uc.lg.Warn("Cascade: failed to marshal replace-facts result", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		resultJSON = []byte("{}")
	}
	if err := uc.saga.UpdateSagaStepResult(ctx, step.ID, string(resultJSON)); err != nil {
		uc.lg.Warn("Cascade: failed to update saga step result", loggateway.StepID("memory.cascade_fail"), loggateway.Str("step_id", fmt.Sprint(step.ID)), loggateway.Err(err))
	}
	return nil
}

func (uc *L4CascadeUsecase) execSyncIndex(ctx context.Context, step *CascadeSagaStep, row map[string]any) error {
	agentID := jsonutil.IfaceStr(row, "agent_id")
	syncer := uc.indexSync
	if agentID == "" || syncer == nil {
		return nil
	}
	marked, err := uc.mutator.MarkFactsIndexStaleByAgent(ctx, agentID)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(map[string]any{"stale_marked": marked})
	if err != nil {
		uc.lg.Warn("Cascade: failed to marshal sync-index result", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		resultJSON = []byte("{}")
	}
	if err := uc.saga.UpdateSagaStepResult(ctx, step.ID, string(resultJSON)); err != nil {
		uc.lg.Warn("Cascade: failed to update saga step result for sync index", loggateway.StepID("memory.cascade_fail"), loggateway.Str("step_id", fmt.Sprint(step.ID)), loggateway.Err(err))
	}
	return nil
}

func (uc *L4CascadeUsecase) compensateCompletedSteps(ctx context.Context, steps []CascadeSagaStep) {
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		if s.State != "succeeded" {
			continue
		}
		compOK := true
		switch s.StepName {
		case SagaStepReplaceFacts:
			compOK = uc.compensateReplaceFacts(ctx, s)
		case SagaStepUpsertEntity:
			compOK = uc.compensateUpsertEntity(ctx, s)
		case SagaStepTouchAffected:
			compOK = uc.compensateTouchAffected(ctx, s)
		case SagaStepSyncIndex:
			compOK = uc.compensateSyncIndex(ctx, s)
		}
		newState := "compensated"
		if !compOK {
			newState = "compensate_failed"
		}
		if err := uc.saga.UpdateSagaStepState(ctx, s.ID, newState, ""); err != nil {
			uc.lg.Warn("Cascade: failed to update saga step state", loggateway.StepID("memory.cascade_fail"), loggateway.Str("step_id", fmt.Sprint(s.ID)), loggateway.Str("new_state", newState), loggateway.Err(err))
		}
	}
}

func (uc *L4CascadeUsecase) compensateReplaceFacts(ctx context.Context, step CascadeSagaStep) bool {
	var payload struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(step.PayloadJSON), &payload); err != nil || payload.AgentID == "" {
		if err != nil {
			uc.lg.Warn("解析 compensate_replace_facts payload 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		}
		return false
	}
	reverted, err := uc.mutator.RevertCascadeFactStatements(ctx, payload.AgentID)
	if err != nil {
		uc.lg.Warn("compensate_replace_facts_failed", loggateway.StepID("memory.l4_fail"), loggateway.Err(err))
		return false
	}
	uc.lg.Warn("compensate_replace_facts_reverted", loggateway.StepID("memory.l4"), loggateway.Int("reverted", reverted))
	return true
}

func (uc *L4CascadeUsecase) compensateUpsertEntity(ctx context.Context, step CascadeSagaStep) bool {
	// Restore the entity name to its pre-saga value.
	// The payload is a dedicated compensation struct with explicit "old_name" field.
	var payload struct {
		EntityID   string   `json:"entity_id"`
		ScopeType  string   `json:"scope_type"`
		ScopeID    string   `json:"scope_id"`
		UserID     string   `json:"user_id"`
		EntityType string   `json:"entity_type"`
		OldName    string   `json:"old_name"`
		Desc       string   `json:"description"`
		Meta       string   `json:"metadata"`
		Importance *float64 `json:"importance"`
		Confidence *float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(step.PayloadJSON), &payload); err != nil {
		uc.lg.Warn("compensate_upsert_entity: failed to parse payload", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return false
	}
	if payload.OldName == "" || payload.EntityID == "" {
		uc.lg.Warn("compensate_upsert_entity: missing old_name or entity_id in payload", loggateway.StepID("memory.cascade_fail"))
		return false
	}
	// Use importance/confidence from the compensation payload (captured at
	// saga init time from the pre-saga entity snapshot). *float64 pointers
	// distinguish "field absent in old payload" (nil → use fallback) from
	// "field present with value 0" (non-nil → preserve zero).
	importance := l4CascadeEntImportance
	if payload.Importance != nil {
		importance = *payload.Importance
	}
	confidence := l4CascadeEntConfidence
	if payload.Confidence != nil {
		confidence = *payload.Confidence
	}
	if err := uc.entityWriter.UpsertEntity(ctx, L4EntityWrite{
		ID:             payload.EntityID,
		ScopeType:      payload.ScopeType,
		ScopeID:        payload.ScopeID,
		UserID:         payload.UserID,
		EntityType:     payload.EntityType,
		Name:           payload.OldName,
		NameNormalized: strings.ToLower(payload.OldName),
		Description:    payload.Desc,
		Importance:     importance,
		Confidence:     confidence,
		MetadataJSON:   payload.Meta,
	}); err != nil {
		uc.lg.Warn("compensate_upsert_entity: failed to restore old name", loggateway.StepID("memory.cascade_fail"), loggateway.Str("entity_id", payload.EntityID), loggateway.Err(err))
		return false
	}
	uc.lg.Warn("compensate_upsert_entity: restored old name", loggateway.StepID("memory.l4"), loggateway.Str("entity_id", payload.EntityID), loggateway.Str("old_name", payload.OldName))
	return true
}

func (uc *L4CascadeUsecase) compensateTouchAffected(ctx context.Context, step CascadeSagaStep) bool {
	// Remove cascade_linked_* metadata from affected entities that were touched.
	var affected []CascadeAffectedEntity
	if err := json.Unmarshal([]byte(step.PayloadJSON), &affected); err != nil {
		uc.lg.Warn("compensate_touch_affected: failed to parse payload", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return false
	}
	if len(affected) == 0 {
		uc.lg.Warn("compensate_touch_affected: no affected entities in payload", loggateway.StepID("memory.cascade_fail"))
		return false
	}
	allOK := true
	for _, aff := range affected {
		entRaw, err := uc.reader.GetEntityRow(ctx, aff.EntityID)
		if err != nil {
			uc.lg.Warn("compensate_touch_affected: failed to get entity", loggateway.StepID("memory.cascade_fail"), loggateway.Str("entity_id", aff.EntityID), loggateway.Err(err))
			allOK = false
			continue
		}
		var ent map[string]any
		if err := json.Unmarshal(entRaw, &ent); err != nil {
			uc.lg.Warn("compensate_touch_affected: failed to parse entity row", loggateway.StepID("memory.cascade_fail"), loggateway.Str("entity_id", aff.EntityID), loggateway.Err(err))
			allOK = false
			continue
		}
		metaStr := jsonutil.IfaceStr(ent, "metadata_json")
		var m map[string]any
		if json.Unmarshal([]byte(metaStr), &m) == nil {
			delete(m, "cascade_linked_trigger_id")
			delete(m, "cascade_linked_name")
			if newMeta, err := json.Marshal(m); err == nil {
				// Preserve original importance/confidence from the entity row
				// so compensation only removes cascade metadata without
				// corrupting the entity's intrinsic quality scores.
				// Use anyFloatRaw to preserve zero values.
				importance := anyFloatRaw(ent, "importance", l4CascadeTouchImportance)
				confidence := anyFloatRaw(ent, "confidence", l4CascadeTouchConfidence)
				if err := uc.entityWriter.UpsertEntity(ctx, L4EntityWrite{
					ID:             aff.EntityID,
					ScopeType:      "agent",
					ScopeID:        jsonutil.IfaceStr(ent, "scope_id"),
					UserID:         jsonutil.IfaceStr(ent, "user_id"),
					EntityType:     jsonutil.IfaceStr(ent, "entity_type"),
					Name:           jsonutil.IfaceStr(ent, "name"),
					NameNormalized: jsonutil.IfaceStr(ent, "name_normalized"),
					Description:    jsonutil.IfaceStr(ent, "description"),
					Importance:     importance,
					Confidence:     confidence,
					MetadataJSON:   string(newMeta),
				}); err != nil {
					uc.lg.Warn("compensate_touch_affected: failed to upsert entity", loggateway.StepID("memory.cascade_fail"), loggateway.Str("entity_id", aff.EntityID), loggateway.Err(err))
					allOK = false
				}
			}
		}
	}
	uc.lg.Warn("compensate_touch_affected: cleaned cascade_linked metadata", loggateway.StepID("memory.l4"), loggateway.Int("affected_count", len(affected)))
	return allOK
}

func (uc *L4CascadeUsecase) compensateSyncIndex(ctx context.Context, step CascadeSagaStep) bool {
	var payload struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(step.PayloadJSON), &payload); err != nil || payload.AgentID == "" {
		if err != nil {
			uc.lg.Warn("解析 compensate_sync_index payload 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		}
		return false
	}
	marked, err := uc.mutator.MarkFactsIndexStaleByAgent(ctx, payload.AgentID)
	if err != nil {
		uc.lg.Warn("compensate_sync_index_failed", loggateway.StepID("memory.l4_fail"), loggateway.Err(err))
		return false
	}
	uc.lg.Warn("compensate_sync_index_marked_stale", loggateway.StepID("memory.l4"), loggateway.Str("marked", fmt.Sprint(marked)))
	return true
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
		uc.lg.Warn("解析 affected entities 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
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
			uc.lg.Warn("解析 touch entity row 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Str("entity_id", id), loggateway.Err(err))
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
			Importance:     anyFloatRaw(ent, "importance", l4CascadeTouchImportance),
			Confidence:     anyFloatRaw(ent, "confidence", l4CascadeTouchConfidence),
			MetadataJSON:   mergeCascadeLinkedMeta(jsonutil.IfaceStr(ent, "metadata_json"), triggerID, newName, uc.lg),
		}); err != nil {
			failedIDs = append(failedIDs, id)
		}
	}
	if len(failedIDs) > 0 {
		uc.lg.Warn("touchAffectedEntities: some entities failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("trigger_id", triggerID), loggateway.Str("failed_ids", fmt.Sprint(failedIDs)))
	}
	return nil
}

func mergeCascadeAppliedMeta(base, newName string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil || m == nil {
		if err != nil {
			lg.Warn("解析 cascade applied meta 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		}
		m = map[string]any{}
	}
	m["source"] = "cascade_approve"
	m["cascade_applied_name"] = newName
	delete(m, "pending_name")
	delete(m, "gate")
	delete(m, "conflict")
	b, err := json.Marshal(m)
	if err != nil {
		lg.Warn("序列化 cascade applied meta 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return base
	}
	return string(b)
}

func mergeCascadeLinkedMeta(base, triggerID, newName string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil || m == nil {
		if err != nil {
			lg.Warn("解析 cascade linked meta 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		}
		m = map[string]any{}
	}
	m["cascade_linked_trigger_id"] = triggerID
	m["cascade_linked_name"] = newName
	b, err := json.Marshal(m)
	if err != nil {
		lg.Warn("序列化 cascade linked meta 失败", loggateway.StepID("memory.cascade_fail"), loggateway.Err(err))
		return base
	}
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
	// CAS: only pending/partial/failed can be rejected. Prevents overwriting
	// applied/running proposals without compensation.
	raw, swapped, err := uc.proposals.CompareAndSwapProposalStatus(ctx, id,
		[]string{"pending", "partial", "failed"}, "rejected", reviewer, note)
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, ErrCascadeRejectNotAllowed
	}
	return raw, nil
}

func cascadeNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// anyFloatRaw extracts a float64 from a map value, returning the fallback
// only if the key is missing or the value is not a numeric type.
// It preserves zero and negative values — used for compensation payloads
// where the original value must be restored exactly.
func anyFloatRaw(m map[string]any, key string, fallback float64) float64 {
	v, ok := m[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return fallback
}

// mustMarshal marshals v to JSON or returns an apierror.Internal error.
// Used for saga payloads where data integrity is critical — silent
// failure would leave the saga unable to compensate.
func mustMarshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, apierror.Internal("MEMORY", "marshal saga payload: %s", err.Error())
	}
	return b, nil
}
