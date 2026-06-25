package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/activity"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect/sql"
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
		Order(ent.Asc(activity.FieldSeq), ent.Asc(activity.FieldTimestamp)).
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
		Order(ent.Asc(activity.FieldSeq), ent.Asc(activity.FieldTimestamp)).
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

// ListBySpiritSession returns all activities under a spirit session tree
// (across team/agent sub-sessions). Uses the spirit_session_id index.
func (r *activityRepo) ListBySpiritSession(ctx context.Context, spiritSessionID string) ([]biz.Activity, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("activity repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).Activity.Query().
		Where(activity.SpiritSessionIDEQ(spiritSessionID)).
		Order(ent.Asc(activity.FieldSeq), ent.Asc(activity.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivitiesToBiz(rows), nil
}

// ListByTeam returns all activities for a given team.
func (r *activityRepo) ListByTeam(ctx context.Context, teamID string) ([]biz.Activity, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("activity repo: database not configured")
	}
	if teamID == "" {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).Activity.Query().
		Where(activity.TeamIDEQ(teamID)).
		Order(ent.Asc(activity.FieldSeq), ent.Asc(activity.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivitiesToBiz(rows), nil
}

// ListByParentSession returns activities whose session_id belongs to direct
// child sessions of parentSessionID. Used for member session activity loading.
// Implementation: first query child session IDs, then query their activities.
func (r *activityRepo) ListByParentSession(ctx context.Context, parentSessionID string) ([]biz.Activity, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("activity repo: database not configured")
	}
	if parentSessionID == "" {
		return nil, nil
	}
	// Step 1: find direct child session IDs
	childSessions, err := r.data.RW().Read(ctx).Session.Query().
		Where(session.ParentSessionIDEQ(parentSessionID)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	if len(childSessions) == 0 {
		return nil, nil
	}
	childIDs := make([]string, 0, len(childSessions))
	for _, s := range childSessions {
		childIDs = append(childIDs, s.ID)
	}
	// Step 2: query activities for those sessions
	rows, err := r.data.RW().Read(ctx).Activity.Query().
		Where(activity.SessionIDIn(childIDs...)).
		Order(ent.Asc(activity.FieldSeq), ent.Asc(activity.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivitiesToBiz(rows), nil
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
		SetSeq(a.Seq).
		SetPromptTokens(a.PromptTokens).
		SetCompletionTokens(a.CompletionTokens).
		SetContent(a.Content).
		SetReasoning(a.Reasoning).
		SetToolName(a.ToolName).
		SetToolCategory(string(a.ToolCategory)).
		SetToolCallID(a.ToolCallID).
		SetToolArguments(a.ToolArguments).
		SetToolResult(a.ToolResult).
		SetToolDurationMs(a.ToolDurationMs).
		SetToolErrorCode(a.ToolErrorCode).
		SetStage(a.Stage).
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
		SetSeq(a.Seq).
		SetPromptTokens(a.PromptTokens).
		SetCompletionTokens(a.CompletionTokens).
		SetContent(a.Content).
		SetReasoning(a.Reasoning).
		SetToolName(a.ToolName).
		SetToolCategory(string(a.ToolCategory)).
		SetToolCallID(a.ToolCallID).
		SetToolArguments(a.ToolArguments).
		SetToolResult(a.ToolResult).
		SetToolDurationMs(a.ToolDurationMs).
		SetToolErrorCode(a.ToolErrorCode).
		SetStage(a.Stage).
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
	// Atomic upsert via ON CONFLICT (id) DO UPDATE. Avoids the "Create → ConstraintError
	// → fallback Update" race that previously produced noise-level
	// `agent.activity_sequencer.persist` warn logs on every ActivityDone (same
	// activity_id is re-inserted to mark the activity complete). Ent's
	// ResolveWithNewValues() updates only the columns actually SET in this call,
	// preserving immutable fields (id, kind, session_id, turn_id, parent_activity_id,
	// timestamp, agent_key, agent_name, child_board_id, spirit_session_id, team_id,
	// dag_node_id) from the initial insert.
	now := a.Timestamp.UTC().Format(time.RFC3339Nano)
	b := r.data.RW().Write(ctx).Activity.Create().
		SetID(a.ID).
		SetKind(string(a.Kind)).
		SetStatus(string(a.Status)).
		SetSessionID(a.SessionID).
		SetTurnID(a.TurnID).
		SetParentActivityID(a.ParentActivityID).
		SetTimestamp(now).
		SetDurationMs(a.DurationMs).
		SetSeq(a.Seq).
		SetPromptTokens(a.PromptTokens).
		SetCompletionTokens(a.CompletionTokens).
		SetContent(a.Content).
		SetReasoning(a.Reasoning).
		SetToolName(a.ToolName).
		SetToolCategory(string(a.ToolCategory)).
		SetToolCallID(a.ToolCallID).
		SetToolArguments(a.ToolArguments).
		SetToolResult(a.ToolResult).
		SetToolDurationMs(a.ToolDurationMs).
		SetToolErrorCode(a.ToolErrorCode).
		SetStage(a.Stage).
		SetChildBoardID(a.ChildBoardID).
		SetSpiritSessionID(a.SpiritSessionID).
		SetTeamID(a.TeamID).
		SetDagNodeID(a.DagNodeID).
		SetAgentKey(a.AgentKey).
		SetAgentName(a.AgentName).
		SetCollapsed(a.Collapsed).
		SetLabel(a.Label)
	if len(a.DependsOn) > 0 {
		b.SetDependsOn(a.DependsOn)
	}
	if err := b.OnConflict(
		sql.ConflictColumns(activity.FieldID),
		sql.ResolveWithNewValues(),
	).Exec(ctx); err != nil {
		return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
	}
	row, err := r.data.RW().Read(ctx).Activity.Get(ctx, a.ID)
	if err != nil {
		return biz.Activity{}, entErrToBizErr(err, "ACTIVITY")
	}
	return entActivityToBiz(row), nil
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
		Seq:              row.Seq,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		Content:          row.Content,
		Reasoning:        row.Reasoning,
		ToolName:         row.ToolName,
		ToolCategory:     biz.ToolCategory(row.ToolCategory),
		ToolCallID:       row.ToolCallID,
		ToolArguments:    row.ToolArguments,
		ToolResult:       row.ToolResult,
		ToolDurationMs:   row.ToolDurationMs,
		ToolErrorCode:    row.ToolErrorCode,
		Stage:            row.Stage,
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
