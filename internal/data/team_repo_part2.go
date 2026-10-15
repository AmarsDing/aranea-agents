package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"strings"
)

// GetTaskDeadLetter fetches a dead letter by id (N5 IDOR: service-layer
// workspace check needs the owning team before mutating).
func (r *TeamRepo) GetTaskDeadLetter(ctx context.Context, id string) (biz.TaskDeadLetter, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.TaskDeadLetter{}, apierror.BadRequest("TASK_DEAD_LETTER", "task dead letter id is required")
	}
	row, err := r.data.RW().Read(ctx).TaskDeadLetter.Get(ctx, id)
	if err != nil {
		return biz.TaskDeadLetter{}, entErrToBizErr(err, "TEAM")
	}
	return entTaskDeadLetterToBiz(row), nil
}

func (r *TeamRepo) ResolveTaskDeadLetter(ctx context.Context, id string) (biz.TaskDeadLetter, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.TaskDeadLetter{}, apierror.BadRequest("TASK_DEAD_LETTER", "task dead letter id is required")
	}
	existing, err := r.data.RW().Read(ctx).TaskDeadLetter.Get(ctx, id)
	if err != nil {
		return biz.TaskDeadLetter{}, entErrToBizErr(err, "TEAM")
	}
	if strings.EqualFold(strings.TrimSpace(existing.Status), biz.TaskDeadLetterStatusResolved) {
		return entTaskDeadLetterToBiz(existing), nil
	}
	if !strings.EqualFold(strings.TrimSpace(existing.Status), biz.TaskDeadLetterStatusPending) {
		return biz.TaskDeadLetter{}, apierror.BadRequest("TASK_DEAD_LETTER", "task dead letter "+id+" is not pending")
	}
	now := nowRFC3339()
	row, err := r.data.RW().Write(ctx).TaskDeadLetter.UpdateOneID(id).
		SetStatus(biz.TaskDeadLetterStatusResolved).
		SetResolvedAt(now).
		Save(ctx)
	if err != nil {
		return biz.TaskDeadLetter{}, entErrToBizErr(err, "TEAM")
	}
	return entTaskDeadLetterToBiz(row), nil
}

func entTaskDeadLetterToBiz(e *ent.TaskDeadLetter) biz.TaskDeadLetter {
	if e == nil {
		return biz.TaskDeadLetter{}
	}
	return biz.TaskDeadLetter{
		ID:               e.ID,
		SourceType:       e.SourceType,
		SourceID:         e.SourceID,
		TeamID:           e.TeamID,
		TeamRunID:        e.TeamRunID,
		SessionID:        e.SessionID,
		GraphExecutionID: e.GraphExecutionID,
		ErrorMessage:     e.ErrorMessage,
		PayloadJSON:      e.PayloadJSON,
		Status:           e.Status,
		CreatedAt:        e.CreatedAt,
		ResolvedAt:       e.ResolvedAt,
	}
}

func entOrchestrationStepToBiz(e *ent.OrchestrationStep) biz.OrchestrationStep {
	if e == nil {
		return biz.OrchestrationStep{}
	}
	return biz.OrchestrationStep{
		ID:                   e.ID,
		TeamRunID:            e.TeamRunID,
		GraphExecutionID:     e.GraphExecutionID,
		NodeID:               e.NodeID,
		ActivitySnapshotJSON: e.ActivitySnapshotJSON,
		Status:               e.Status,
		StartedAt:            e.StartedAt,
		FinishedAt:           e.FinishedAt,
		CreatedAt:            e.CreatedAt,
	}
}

func parseDependsOnJSON(jsonStr string, lg loggateway.Logger) []string {
	if jsonStr == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(jsonStr), &ids); err != nil {
		lg.Warn("team json unmarshal failed", loggateway.StepID("data.team"), loggateway.Err(err))
		return nil
	}
	return ids
}

func formatDependsOnJSON(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return ""
	}
	return string(b)
}
