package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/orchestrationstep"
	"aranea-agents/internal/data/ent/taskdeadletter"
	"aranea-agents/internal/data/ent/team"
	"aranea-agents/internal/data/ent/teamrun"
	"aranea-agents/internal/data/ent/teamrunstep"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type teamRepo struct {
	data *Data
}

var _ biz.TeamRepository = (*teamRepo)(nil)

// NewTeamRepo implements biz.TeamRepository.
func NewTeamRepo(d *Data) biz.TeamRepository {
	return &teamRepo{data: d}
}

func entTeamToBiz(e *ent.Team, lg loggateway.Logger) biz.Team {
	if e == nil {
		return biz.Team{}
	}
	return biz.Team{
		ID:                 e.ID,
		TeamKey:            e.TeamKey,
		DisplayName:        e.DisplayName,
		Status:             e.Status,
		IsDefault:          e.IsDefault,
		DefinitionJSON:     e.DefinitionJSON,
		ADKAppName:         e.AdkAppName,
		CategoryIndustryID: e.CategoryIndustryID,
		SpiritSessionID:    e.SpiritSessionID,
		TaskDescription:    e.TaskDescription,
		AutoCreated:        e.AutoCreated,
		DagNodeID:          e.DagNodeID,
		DependsOn:          parseDependsOnJSON(e.DependsOnJSON, lg),
		ParallelConfigJSON: e.ParallelConfigJSON,
		Topology:           e.Topology,
		Readonly:           e.Readonly,
		Kind:               string(e.Kind),
		Source:             string(e.Source),
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		DeletedAt:          e.DeletedAt,
	}
}

func entTeamRunToBiz(e *ent.TeamRun) biz.TeamRun {
	if e == nil {
		return biz.TeamRun{}
	}
	return biz.TeamRun{
		ID:                     e.ID,
		TeamID:                 e.TeamID,
		SessionID:              e.SessionID,
		MessageID:              e.MessageID,
		Mode:                   e.Mode,
		Status:                 e.Status,
		InputPreview:           e.InputPreview,
		OutputPreview:          e.OutputPreview,
		TokenIn:                e.TokenIn,
		TokenOut:               e.TokenOut,
		CostMicroUSD:           e.CostMicroUsd,
		DurationMS:             e.DurationMs,
		ErrorMessage:           e.ErrorMessage,
		TopologyJSON:           e.TopologyJSON,
		GraphExecutionID:       e.GraphExecutionID,
		DefinitionSnapshotJSON: e.DefinitionSnapshotJSON,
		TraceID:                e.TraceID,
		StartedAt:              e.StartedAt,
		FinishedAt:             e.FinishedAt,
		CreatedAt:              e.CreatedAt,
		UpdatedAt:              e.UpdatedAt,
	}
}

func entTeamRunStepToBiz(e *ent.TeamRunStep) biz.TeamRunStep {
	if e == nil {
		return biz.TeamRunStep{}
	}
	return biz.TeamRunStep{
		ID:            e.ID,
		RunID:         e.RunID,
		TeamID:        e.TeamID,
		AgentID:       e.AgentID,
		AgentKey:      e.AgentKey,
		AgentName:     e.AgentName,
		Role:          e.Role,
		SortOrder:     e.SortOrder,
		Status:        e.Status,
		InputPreview:  e.InputPreview,
		OutputPreview: e.OutputPreview,
		TokenIn:       e.TokenIn,
		TokenOut:      e.TokenOut,
		CostMicroUSD:  e.CostMicroUsd,
		DurationMS:    e.DurationMs,
		ErrorMessage:  e.ErrorMessage,
		StartedAt:     e.StartedAt,
		FinishedAt:    e.FinishedAt,
		CreatedAt:     e.CreatedAt,
		ToolCallCount: e.ToolCallCount,
	}
}

func (r *teamRepo) ListTeamsByStatus(ctx context.Context, status string) ([]biz.Team, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.Team.Query().Where(team.StatusEQ(status), team.DeletedAtEQ("")).
		Order(team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *teamRepo) ListTeams(ctx context.Context) ([]biz.Team, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.Team.Query().Where(team.DeletedAtEQ("")).
		Order(team.ByIsDefault(entsql.OrderDesc()), team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *teamRepo) GetTeamByID(ctx context.Context, id string) (biz.Team, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.Team.Query().Where(team.IDEQ(id), team.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, sql.ErrNoRows
		}
		return biz.Team{}, err
	}
	return entTeamToBiz(row, r.data.lg), nil
}

func (r *teamRepo) GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.Team.Query().Where(team.TeamKeyEQ(teamKey), team.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, sql.ErrNoRows
		}
		return biz.Team{}, err
	}
	return entTeamToBiz(row, r.data.lg), nil
}

func (r *teamRepo) CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return biz.Team{}, kerrors.BadRequest("TEAM", "missing required fields")
	}
	now := nowRFC3339()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = biz.TeamStatusPending
	}
	_, err := r.data.RW().Write(ctx).Team.Create().
		SetID(t.ID).
		SetTeamKey(t.TeamKey).
		SetDisplayName(t.DisplayName).
		SetStatus(t.Status).
		SetKind(team.Kind(t.Kind)).
		SetSource(team.Source(t.Source)).
		SetReadonly(t.Readonly).
		SetIsDefault(t.IsDefault).
		SetDefinitionJSON(t.DefinitionJSON).
		SetAdkAppName(t.ADKAppName).
		SetCategoryIndustryID(t.CategoryIndustryID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetTaskDescription(t.TaskDescription).
		SetAutoCreated(t.AutoCreated).
		SetDagNodeID(t.DagNodeID).
		SetDependsOnJSON(formatDependsOnJSON(t.DependsOn)).
		SetParallelConfigJSON(t.ParallelConfigJSON).
		SetTopology(t.Topology).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt).
		SetDeletedAt(t.DeletedAt).
		Save(ctx)
	if err != nil {
		return biz.Team{}, err
	}
	return r.GetTeamByID(ctx, t.ID)
}

func (r *teamRepo) UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return biz.Team{}, kerrors.BadRequest("TEAM", "missing required fields")
	}
	now := nowRFC3339()
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = biz.TeamStatusPending
	}
	_, err := r.data.RW().Write(ctx).Team.UpdateOneID(t.ID).
		SetTeamKey(t.TeamKey).
		SetDisplayName(t.DisplayName).
		SetStatus(t.Status).
		SetKind(team.Kind(t.Kind)).
		SetSource(team.Source(t.Source)).
		SetReadonly(t.Readonly).
		SetIsDefault(t.IsDefault).
		SetDefinitionJSON(t.DefinitionJSON).
		SetAdkAppName(t.ADKAppName).
		SetCategoryIndustryID(t.CategoryIndustryID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetTaskDescription(t.TaskDescription).
		SetAutoCreated(t.AutoCreated).
		SetDagNodeID(t.DagNodeID).
		SetDependsOnJSON(formatDependsOnJSON(t.DependsOn)).
		SetParallelConfigJSON(t.ParallelConfigJSON).
		SetTopology(t.Topology).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, sql.ErrNoRows
		}
		return biz.Team{}, err
	}
	return r.GetTeamByID(ctx, t.ID)
}

func (r *teamRepo) DeleteTeam(ctx context.Context, id string) error {
	if id == "" {
		return kerrors.BadRequest("TEAM", "id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).Team.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	// Cascade: clean up related records after successful soft-delete
	cascadeDeleteByTeam(ctx, r.data, id)
	return nil
}

func (r *teamRepo) BatchArchiveTeams(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	now := nowRFC3339()
	n, err := r.data.RW().Write(ctx).Team.Update().
		Where(
			team.IDIn(ids...),
			team.StatusIn(biz.TeamStatusCompleted, biz.TeamStatusFailed, biz.TeamStatusCancelled),
			team.DeletedAtEQ(""),
		).
		SetStatus(biz.TeamStatusArchived).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *teamRepo) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]biz.Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).Team.Query().
		Where(team.SpiritSessionIDEQ(spiritSessionID), team.DeletedAtEQ("")).
		Order(team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *teamRepo) ListTeamRuns(ctx context.Context, teamID string, limit int) ([]biz.TeamRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := r.data.RW().Read(ctx).TeamRun.Query().Order(teamrun.ByCreatedAt(entsql.OrderDesc()))
	if teamID != "" {
		q = q.Where(teamrun.TeamIDEQ(teamID))
	}
	rows, err := q.Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TeamRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamRunToBiz(row))
	}
	return out, nil
}

func (r *teamRepo) HasActiveTeamRun(ctx context.Context, teamID string) (bool, error) {
	count, err := r.data.RW().Read(ctx).TeamRun.Query().
		Where(
			teamrun.TeamIDEQ(teamID),
			teamrun.StatusIn(biz.TeamRunStatusRunning, biz.TeamRunStatusPending),
		).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *teamRepo) GetTeamRunByID(ctx context.Context, id string) (biz.TeamRun, error) {
	row, err := r.data.RW().Read(ctx).TeamRun.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.TeamRun{}, sql.ErrNoRows
		}
		return biz.TeamRun{}, err
	}
	return entTeamRunToBiz(row), nil
}

func (r *teamRepo) ListTeamRunSteps(ctx context.Context, runID string) ([]biz.TeamRunStep, error) {
	rows, err := r.data.RW().Read(ctx).TeamRunStep.Query().
		Where(teamrunstep.RunIDEQ(runID)).
		Order(teamrunstep.BySortOrder(entsql.OrderAsc()), teamrunstep.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TeamRunStep, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamRunStepToBiz(row))
	}
	return out, nil
}

func (r *teamRepo) CreateTeamRun(ctx context.Context, run biz.TeamRun) (biz.TeamRun, error) {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.TeamID) == "" {
		return biz.TeamRun{}, kerrors.BadRequest("TEAM_RUN", "team run id and team_id are required")
	}
	now := nowRFC3339()
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.UpdatedAt == "" {
		run.UpdatedAt = now
	}
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	if run.Status == "" {
		run.Status = biz.TeamRunStatusRunning
	}
	if run.TopologyJSON == "" {
		run.TopologyJSON = "{}"
	}
	_, err := r.data.RW().Write(ctx).TeamRun.Create().
		SetID(run.ID).
		SetTeamID(run.TeamID).
		SetSessionID(run.SessionID).
		SetMessageID(run.MessageID).
		SetMode(run.Mode).
		SetStatus(run.Status).
		SetInputPreview(run.InputPreview).
		SetOutputPreview(run.OutputPreview).
		SetTokenIn(run.TokenIn).
		SetTokenOut(run.TokenOut).
		SetCostMicroUsd(run.CostMicroUSD).
		SetDurationMs(run.DurationMS).
		SetErrorMessage(run.ErrorMessage).
		SetTopologyJSON(run.TopologyJSON).
		SetGraphExecutionID(run.GraphExecutionID).
		SetDefinitionSnapshotJSON(run.DefinitionSnapshotJSON).
		SetTraceID(run.TraceID).
		SetStartedAt(run.StartedAt).
		SetFinishedAt(run.FinishedAt).
		SetCreatedAt(run.CreatedAt).
		SetUpdatedAt(run.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.TeamRun{}, err
	}
	row, err := r.data.RW().Read(ctx).TeamRun.Get(ctx, run.ID)
	if err != nil {
		return biz.TeamRun{}, err
	}
	return entTeamRunToBiz(row), nil
}

func (r *teamRepo) UpdateTeamRun(ctx context.Context, run biz.TeamRun) error {
	if strings.TrimSpace(run.ID) == "" {
		return kerrors.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).TeamRun.UpdateOneID(run.ID).
		SetStatus(run.Status).
		SetOutputPreview(run.OutputPreview).
		SetTokenIn(run.TokenIn).
		SetTokenOut(run.TokenOut).
		SetCostMicroUsd(run.CostMicroUSD).
		SetDurationMs(run.DurationMS).
		SetErrorMessage(run.ErrorMessage).
		SetTopologyJSON(run.TopologyJSON).
		SetFinishedAt(run.FinishedAt).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *teamRepo) CreateTeamRunStep(ctx context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.RunID) == "" {
		return biz.TeamRunStep{}, kerrors.BadRequest("TEAM_RUN_STEP", "step id and run_id are required")
	}
	now := nowRFC3339()
	if step.CreatedAt == "" {
		step.CreatedAt = now
	}
	if step.Status == "" {
		step.Status = biz.TeamMemberStepStatusOK
	}
	_, err := r.data.RW().Write(ctx).TeamRunStep.Create().
		SetID(step.ID).
		SetRunID(step.RunID).
		SetTeamID(step.TeamID).
		SetAgentID(step.AgentID).
		SetAgentKey(step.AgentKey).
		SetAgentName(step.AgentName).
		SetRole(step.Role).
		SetSortOrder(step.SortOrder).
		SetStatus(step.Status).
		SetInputPreview(step.InputPreview).
		SetOutputPreview(step.OutputPreview).
		SetTokenIn(step.TokenIn).
		SetTokenOut(step.TokenOut).
		SetCostMicroUsd(step.CostMicroUSD).
		SetDurationMs(step.DurationMS).
		SetErrorMessage(step.ErrorMessage).
		SetStartedAt(step.StartedAt).
		SetFinishedAt(step.FinishedAt).
		SetCreatedAt(step.CreatedAt).
		SetToolCallCount(step.ToolCallCount).
		Save(ctx)
	if err != nil {
		return biz.TeamRunStep{}, err
	}
	row, err := r.data.RW().Read(ctx).TeamRunStep.Get(ctx, step.ID)
	if err != nil {
		return biz.TeamRunStep{}, err
	}
	return entTeamRunStepToBiz(row), nil
}

func (r *teamRepo) UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error {
	if strings.TrimSpace(runID) == "" {
		return kerrors.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE team_runs SET summary_json=?, updated_at=? WHERE id=?`,
		summaryJSON, now, runID)
	return err
}

func (r *teamRepo) UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error {
	if strings.TrimSpace(runID) == "" {
		return kerrors.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE team_runs SET graph_execution_id=?, updated_at=? WHERE id=?`,
		graphExecutionID, now, runID)
	return err
}

func (r *teamRepo) UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error {
	if strings.TrimSpace(runID) == "" {
		return kerrors.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE team_runs SET trace_id=?, updated_at=? WHERE id=?`,
		traceID, now, runID)
	return err
}

func (r *teamRepo) BatchCreateOrchestrationSteps(ctx context.Context, steps []biz.OrchestrationStep) error {
	if len(steps) == 0 {
		return nil
	}
	builders := make([]*ent.OrchestrationStepCreate, 0, len(steps))
	for _, step := range steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.TeamRunID) == "" {
			continue
		}
		createdAt := step.CreatedAt
		if createdAt == "" {
			createdAt = nowRFC3339()
		}
		builders = append(builders, r.data.RW().Write(ctx).OrchestrationStep.Create().
			SetID(step.ID).
			SetTeamRunID(step.TeamRunID).
			SetGraphExecutionID(step.GraphExecutionID).
			SetNodeID(step.NodeID).
			SetActivitySnapshotJSON(step.ActivitySnapshotJSON).
			SetStatus(step.Status).
			SetStartedAt(step.StartedAt).
			SetFinishedAt(step.FinishedAt).
			SetCreatedAt(createdAt))
	}
	if len(builders) == 0 {
		return nil
	}
	_, err := r.data.RW().Write(ctx).OrchestrationStep.CreateBulk(builders...).Save(ctx)
	return err
}

func (r *teamRepo) ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]biz.OrchestrationStep, error) {
	teamRunID = strings.TrimSpace(teamRunID)
	if teamRunID == "" {
		return nil, kerrors.BadRequest("ORCHESTRATION_STEP", "team_run_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	q := r.data.RW().Read(ctx).OrchestrationStep.Query().
		Where(orchestrationstep.TeamRunIDEQ(teamRunID)).
		Order(orchestrationstep.ByCreatedAt(entsql.OrderAsc())).
		Limit(limit)
	if nodeID = strings.TrimSpace(nodeID); nodeID != "" {
		q = q.Where(orchestrationstep.NodeIDEQ(nodeID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.OrchestrationStep, 0, len(rows))
	for _, row := range rows {
		out = append(out, entOrchestrationStepToBiz(row))
	}
	return out, nil
}

func (r *teamRepo) CreateTaskDeadLetter(ctx context.Context, dl biz.TaskDeadLetter) error {
	if strings.TrimSpace(dl.ID) == "" {
		return kerrors.BadRequest("TASK_DEAD_LETTER", "task dead letter id is required")
	}
	payload := strings.TrimSpace(dl.PayloadJSON)
	if payload == "" {
		payload = "{}"
	}
	_, err := r.data.RW().Write(ctx).TaskDeadLetter.Create().
		SetID(dl.ID).
		SetSourceType(strings.TrimSpace(dl.SourceType)).
		SetSourceID(strings.TrimSpace(dl.SourceID)).
		SetTeamID(strings.TrimSpace(dl.TeamID)).
		SetTeamRunID(strings.TrimSpace(dl.TeamRunID)).
		SetSessionID(strings.TrimSpace(dl.SessionID)).
		SetGraphExecutionID(strings.TrimSpace(dl.GraphExecutionID)).
		SetErrorMessage(strings.TrimSpace(dl.ErrorMessage)).
		SetPayloadJSON(payload).
		SetStatus(strings.TrimSpace(dl.Status)).
		SetCreatedAt(strings.TrimSpace(dl.CreatedAt)).
		SetResolvedAt(strings.TrimSpace(dl.ResolvedAt)).
		Save(ctx)
	return err
}

func (r *teamRepo) ListTaskDeadLetters(ctx context.Context, filter biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	q := r.data.RW().Read(ctx).TaskDeadLetter.Query()
	if sid := strings.TrimSpace(filter.SessionID); sid != "" {
		q = q.Where(taskdeadletter.SessionIDEQ(sid))
	}
	if tid := strings.TrimSpace(filter.TeamID); tid != "" {
		q = q.Where(taskdeadletter.TeamIDEQ(tid))
	}
	if st := strings.TrimSpace(filter.Status); st != "" {
		q = q.Where(taskdeadletter.StatusEQ(st))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := q.Order(taskdeadletter.ByCreatedAt(entsql.OrderDesc())).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TaskDeadLetter, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTaskDeadLetterToBiz(row))
	}
	return out, nil
}

func (r *teamRepo) ResolveTaskDeadLetter(ctx context.Context, id string) (biz.TaskDeadLetter, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.TaskDeadLetter{}, kerrors.BadRequest("TASK_DEAD_LETTER", "task dead letter id is required")
	}
	existing, err := r.data.RW().Read(ctx).TaskDeadLetter.Get(ctx, id)
	if err != nil {
		return biz.TaskDeadLetter{}, err
	}
	if strings.EqualFold(strings.TrimSpace(existing.Status), biz.TaskDeadLetterStatusResolved) {
		return entTaskDeadLetterToBiz(existing), nil
	}
	if !strings.EqualFold(strings.TrimSpace(existing.Status), biz.TaskDeadLetterStatusPending) {
		return biz.TaskDeadLetter{}, kerrors.BadRequest("TASK_DEAD_LETTER", "task dead letter "+id+" is not pending")
	}
	now := nowRFC3339()
	row, err := r.data.RW().Write(ctx).TaskDeadLetter.UpdateOneID(id).
		SetStatus(biz.TaskDeadLetterStatusResolved).
		SetResolvedAt(now).
		Save(ctx)
	if err != nil {
		return biz.TaskDeadLetter{}, err
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
