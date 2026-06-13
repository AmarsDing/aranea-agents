package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/activity"
	"aranea-agents/pkg/loggateway"
)

type activityRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.ActivityRepo = (*activityRepo)(nil)

func NewActivityRepo(d *Data, lg loggateway.Logger) biz.ActivityRepo {
	return &activityRepo{data: d, lg: lg.With(loggateway.Domain("ACTIVITY"))}
}

func (r *activityRepo) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]biz.Activity, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("activity repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).Activity.Query().
		Where(
			activity.SessionIDEQ(sessionID),
			activity.TurnIDEQ(turnID),
		).
		Order(ent.Asc(activity.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivitiesToBiz(rows), nil
}

func (r *activityRepo) ListBySession(ctx context.Context, sessionID string) ([]biz.Activity, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("activity repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).Activity.Query().
		Where(activity.SessionIDEQ(sessionID)).
		Order(ent.Asc(activity.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivitiesToBiz(rows), nil
}

func (r *activityRepo) GetActivity(ctx context.Context, id string) (biz.Activity, error) {
	if r == nil || r.data == nil {
		return biz.Activity{}, fmt.Errorf("activity repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).Activity.Get(ctx, id)
	if err != nil {
		return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivityToBiz(row), nil
}

func (r *activityRepo) CreateActivity(ctx context.Context, a biz.Activity) (biz.Activity, error) {
	if r == nil || r.data == nil {
		return biz.Activity{}, fmt.Errorf("activity repo: database not configured")
	}
	builder := r.data.RW().Write(ctx).Activity.Create().
		SetID(a.ID).
		SetKind(string(a.Kind)).
		SetStatus(string(a.Status)).
		SetSessionID(a.SessionID).
		SetTurnID(a.TurnID).
		SetParentActivityID(a.ParentActivityID).
		SetTimestamp(a.Timestamp.UTC().Format(time.RFC3339Nano)).
		SetDurationMs(a.DurationMs).
		SetContent(a.Content).
		SetReasoning(a.Reasoning).
		SetToolName(a.ToolName).
		SetToolCallID(a.ToolCallID).
		SetToolArguments(a.ToolArguments).
		SetToolResult(a.ToolResult).
		SetToolDurationMs(a.ToolDurationMs).
		SetToolErrorCode(a.ToolErrorCode).
		SetChildBoardID(a.ChildBoardID).
		SetSpiritSessionID(a.SpiritSessionID).
		SetTeamID(a.TeamID).
		SetDagNodeID(a.DagNodeID).
		SetAgentKey(a.AgentKey).
		SetAgentName(a.AgentName).
		SetCollapsed(a.Collapsed).
		SetLabel(a.Label)
	if len(a.DependsOn) > 0 {
		builder.SetDependsOn(a.DependsOn)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivityToBiz(row), nil
}

func (r *activityRepo) UpdateActivity(ctx context.Context, a biz.Activity) (biz.Activity, error) {
	if r == nil || r.data == nil {
		return biz.Activity{}, fmt.Errorf("activity repo: database not configured")
	}
	builder := r.data.RW().Write(ctx).Activity.UpdateOneID(a.ID).
		SetKind(string(a.Kind)).
		SetStatus(string(a.Status)).
		SetDurationMs(a.DurationMs).
		SetContent(a.Content).
		SetReasoning(a.Reasoning).
		SetToolResult(a.ToolResult).
		SetToolDurationMs(a.ToolDurationMs).
		SetToolErrorCode(a.ToolErrorCode).
		SetChildBoardID(a.ChildBoardID).
		SetCollapsed(a.Collapsed).
		SetLabel(a.Label)
	if len(a.DependsOn) > 0 {
		builder.SetDependsOn(a.DependsOn)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivityToBiz(row), nil
}

func (r *activityRepo) UpsertActivity(ctx context.Context, a biz.Activity) (biz.Activity, error) {
	if r == nil || r.data == nil {
		return biz.Activity{}, fmt.Errorf("activity repo: database not configured")
	}
	// Try create first; on constraint error (duplicate ID), fall back to update.
	created, err := r.CreateActivity(ctx, a)
	if err == nil {
		return created, nil
	}
	if ent.IsConstraintError(err) {
		return r.UpdateActivity(ctx, a)
	}
	return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
}

func entActivityToBiz(row *ent.Activity) biz.Activity {
	var ts time.Time
	if row.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, row.Timestamp); err == nil {
			ts = parsed
		}
	}
	return biz.Activity{
		ID:               row.ID,
		Kind:             biz.ActivityKind(row.Kind),
		Status:           biz.ActivityStatus(row.Status),
		SessionID:        row.SessionID,
		TurnID:           row.TurnID,
		ParentActivityID: row.ParentActivityID,
		Timestamp:        ts,
		DurationMs:       row.DurationMs,
		Content:          row.Content,
		Reasoning:        row.Reasoning,
		ToolName:         row.ToolName,
		ToolCallID:       row.ToolCallID,
		ToolArguments:    row.ToolArguments,
		ToolResult:       row.ToolResult,
		ToolDurationMs:   row.ToolDurationMs,
		ToolErrorCode:    row.ToolErrorCode,
		ChildBoardID:     row.ChildBoardID,
		SpiritSessionID:  row.SpiritSessionID,
		TeamID:           row.TeamID,
		DagNodeID:        row.DagNodeID,
		DependsOn:        row.DependsOn,
		AgentKey:         row.AgentKey,
		AgentName:        row.AgentName,
		Collapsed:        row.Collapsed,
		Label:            row.Label,
	}
}

func entActivitiesToBiz(rows []*ent.Activity) []biz.Activity {
	items := make([]biz.Activity, 0, len(rows))
	for _, row := range rows {
		items = append(items, entActivityToBiz(row))
	}
	return items
}
