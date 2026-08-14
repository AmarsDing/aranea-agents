package data

import (
	"context"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/computeruseaudit"
	"aranea-agents/pkg/loggateway"
)

// ComputerUseAuditRepo 实现 bizcu.AuditStore（75-computer-use 审计落库）。
type ComputerUseAuditRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewComputerUseAuditRepo 构造（独立持有 Logger 模式）。
func NewComputerUseAuditRepo(d *Data, lg loggateway.Logger) *ComputerUseAuditRepo {
	return &ComputerUseAuditRepo{data: d, lg: lg.With(loggateway.Domain("computeruse_audit"))}
}

var _ bizcu.AuditStore = (*ComputerUseAuditRepo)(nil)

// RecordStep 落库一步审计记录；ID 由 DB 自增（entry.ID 忽略）。
func (r *ComputerUseAuditRepo) RecordStep(ctx context.Context, entry bizcu.AuditEntry) error {
	params := entry.Params
	if params == nil {
		params = map[string]any{}
	}
	err := r.data.RW().Write(ctx).ComputerUseAudit.Create().
		SetSessionID(entry.SessionID).
		SetAgentKey(entry.AgentKey).
		SetStepIndex(entry.Index).
		SetTarget(entry.Target).
		SetPath(string(entry.Path)).
		SetAction(string(entry.Action)).
		SetParams(params).
		SetResult(string(entry.Result)).
		SetError(entry.Error).
		SetDurationMs(entry.DurationMs).
		SetConfirmedBy(entry.ConfirmedBy).
		SetDanger(entry.Danger).
		SetScreenshotRef(entry.ScreenshotRef).
		SetCreatedAt(entry.CreatedAt).
		Exec(ctx)
	return entErrToBizErr(err, "computeruse_audit")
}

// ListSteps 按 step_index 升序返回会话的全部审计步骤。
func (r *ComputerUseAuditRepo) ListSteps(ctx context.Context, sessionID string) ([]bizcu.AuditEntry, error) {
	rows, err := r.data.RW().Read(ctx).ComputerUseAudit.Query().
		Where(computeruseaudit.SessionID(sessionID)).
		Order(ent.Asc(computeruseaudit.FieldStepIndex)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "computeruse_audit")
	}
	out := make([]bizcu.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, bizcu.AuditEntry{
			ID:            int64(row.ID),
			SessionID:     row.SessionID,
			AgentKey:      row.AgentKey,
			Index:         row.StepIndex,
			Target:        row.Target,
			Path:          bizcu.GroundingPath(row.Path),
			Action:        bizcu.ActionType(row.Action),
			Params:        row.Params,
			Result:        bizcu.StepResult(row.Result),
			Error:         row.Error,
			DurationMs:    row.DurationMs,
			ConfirmedBy:   row.ConfirmedBy,
			Danger:        row.Danger,
			Degraded:      bizcu.GroundingPath(row.Path) == bizcu.PathVLMDirect,
			ScreenshotRef: row.ScreenshotRef,
			CreatedAt:     row.CreatedAt,
		})
	}
	return out, nil
}
