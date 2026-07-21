package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/orchestrationstep"
	"aranea-agents/internal/data/ent/taskdeadletter"
	"aranea-agents/internal/data/ent/team"
	"aranea-agents/internal/data/ent/teamrun"
	"aranea-agents/internal/data/ent/teamrunstep"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type TeamRepo struct {
	data *Data
}

var (
	_ biz.TeamReader            = (*TeamRepo)(nil)
	_ biz.TeamWriter            = (*TeamRepo)(nil)
	_ biz.TeamRunReader         = (*TeamRepo)(nil)
	_ biz.TeamRunWriter         = (*TeamRepo)(nil)
	_ biz.OrchestrationStepRepo = (*TeamRepo)(nil)
	_ biz.TaskDeadLetterRepo    = (*TeamRepo)(nil)
)

// NewTeamRepo creates a TeamRepo that satisfies all team-related narrow interfaces.
func NewTeamRepo(d *Data) *TeamRepo {
	return &TeamRepo{data: d}
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
		DepartmentID:       e.DepartmentID,
		DeptLeadAgentID:    e.DeptLeadAgentID,
		Deliverables:       e.Deliverables,
		InputContract:      e.InputContract,
		DeliverablesOutput: e.DeliverablesOutputJSON,
		CrossDeptMemberIDs: e.CrossDeptMemberIds,
		LinkedGraphID:      e.LinkedGraphID,
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
		InterruptReason:    e.InterruptReason,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		DeletedAt:          e.DeletedAt,
	}
}

func entTeamRunToBiz(e *ent.TeamRun) biz.TeamRunRecord {
	if e == nil {
		return biz.TeamRunRecord{}
	}
	return biz.TeamRunRecord{
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

func (r *TeamRepo) ListTeamsByStatus(ctx context.Context, status string) ([]biz.Team, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.Team.Query().Where(team.StatusEQ(status), team.DeletedAtEQ("")).
		Order(team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *TeamRepo) ListTeams(ctx context.Context) ([]biz.Team, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.Team.Query().Where(team.DeletedAtEQ("")).
		Order(team.ByIsDefault(entsql.OrderDesc()), team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *TeamRepo) GetTeamByID(ctx context.Context, id string) (biz.Team, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.Team.Query().Where(team.IDEQ(id), team.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, apierror.NotFound(apierror.DomainTeam, "not found")
		}
		return biz.Team{}, entErrToBizErr(err, "TEAM")
	}
	return entTeamToBiz(row, r.data.lg), nil
}

func (r *TeamRepo) GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.Team.Query().Where(team.TeamKeyEQ(teamKey), team.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, apierror.NotFound(apierror.DomainTeam, "not found")
		}
		return biz.Team{}, entErrToBizErr(err, "TEAM")
	}
	return entTeamToBiz(row, r.data.lg), nil
}

func (r *TeamRepo) CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	if t.TeamKey == "" || t.DisplayName == "" {
		return biz.Team{}, apierror.BadRequest("TEAM", "missing required fields")
	}
	if t.ID == "" {
		t.ID = generateCatalogID()
	}
	now := nowRFC3339()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = biz.TeamStatusPending
	}
	// Kind and Source are required enum fields. Ent's Default() only applies
	// when the field is not set at all; an explicit SetKind("") fails validation.
	// Apply defaults here so callers (e.g. AssembleTeam) don't need to set them.
	if t.Kind == "" {
		t.Kind = "user"
	}
	if t.Source == "" {
		t.Source = "user"
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
		SetDepartmentID(t.DepartmentID).
		SetDeptLeadAgentID(t.DeptLeadAgentID).
		SetDeliverables(t.Deliverables).
		SetInputContract(t.InputContract).
		SetDeliverablesOutputJSON(t.DeliverablesOutput).
		SetCrossDeptMemberIds(t.CrossDeptMemberIDs).
		SetLinkedGraphID(t.LinkedGraphID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetTaskDescription(t.TaskDescription).
		SetAutoCreated(t.AutoCreated).
		SetDagNodeID(t.DagNodeID).
		SetDependsOnJSON(formatDependsOnJSON(t.DependsOn)).
		SetParallelConfigJSON(t.ParallelConfigJSON).
		SetTopology(t.Topology).
		SetInterruptReason(t.InterruptReason).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt).
		SetDeletedAt(t.DeletedAt).
		Save(ctx)
	if err != nil {
		return biz.Team{}, entErrToBizErr(err, "TEAM")
	}
	return r.GetTeamByID(ctx, t.ID)
}

func (r *TeamRepo) UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return biz.Team{}, apierror.BadRequest("TEAM", "missing required fields")
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
		SetDepartmentID(t.DepartmentID).
		SetDeptLeadAgentID(t.DeptLeadAgentID).
		SetDeliverables(t.Deliverables).
		SetInputContract(t.InputContract).
		SetDeliverablesOutputJSON(t.DeliverablesOutput).
		SetCrossDeptMemberIds(t.CrossDeptMemberIDs).
		SetLinkedGraphID(t.LinkedGraphID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetTaskDescription(t.TaskDescription).
		SetAutoCreated(t.AutoCreated).
		SetDagNodeID(t.DagNodeID).
		SetDependsOnJSON(formatDependsOnJSON(t.DependsOn)).
		SetParallelConfigJSON(t.ParallelConfigJSON).
		SetTopology(t.Topology).
		SetInterruptReason(t.InterruptReason).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, apierror.NotFound(apierror.DomainTeam, "not found")
		}
		return biz.Team{}, entErrToBizErr(err, "TEAM")
	}
	return r.GetTeamByID(ctx, t.ID)
}

// UpdateTeamWhereStatus performs a Compare-And-Swap update on the team status
// field. The row is updated only if its current status equals
// expectedCurrentStatus. Returns true if the row was updated, false if the
// current status did not match (concurrent modification).
func (r *TeamRepo) UpdateTeamWhereStatus(ctx context.Context, id, newStatus, expectedCurrentStatus string) (bool, error) {
	if id == "" {
		return false, apierror.BadRequest("TEAM", "id is required")
	}
	now := nowRFC3339()
	count, err := r.data.RW().Write(ctx).Team.Update().
		Where(team.IDEQ(id), team.StatusEQ(expectedCurrentStatus)).
		SetStatus(newStatus).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return false, entErrToBizErr(err, "TEAM")
	}
	return count > 0, nil
}

func (r *TeamRepo) DeleteTeam(ctx context.Context, id string) error {
	if id == "" {
		return apierror.BadRequest("TEAM", "id is required")
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := nowRFC3339()
		if _, err := r.data.RW().Write(txCtx).Team.UpdateOneID(id).
			SetDeletedAt(now).
			SetStatus(biz.TeamStatusDeleted).
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return entErrToBizErr(err, "TEAM")
		}
		return cascadeDeleteByTeam(txCtx, r.data, id)
	})
}

func (r *TeamRepo) BatchArchiveTeams(ctx context.Context, ids []string) (int, error) {
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
		return 0, entErrToBizErr(err, "TEAM")
	}
	return n, nil
}

// ListTeamsByDepartmentID lists all teams belonging to a specific department.
func (r *TeamRepo) ListTeamsByDepartmentID(ctx context.Context, deptID string) ([]biz.Team, error) {
	if deptID == "" {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).Team.Query().
		Where(team.DepartmentIDEQ(deptID), team.DeletedAtEQ("")).
		Order(team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *TeamRepo) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]biz.Team, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("TEAM", "spirit_session_id is required")
	}
	rows, err := r.data.RW().Read(ctx).Team.Query().
		Where(team.SpiritSessionIDEQ(spiritSessionID), team.DeletedAtEQ("")).
		Order(team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row, r.data.lg))
	}
	return out, nil
}

func (r *TeamRepo) ListTeamRuns(ctx context.Context, teamID string, limit int) ([]biz.TeamRunRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := r.data.RW().Read(ctx).TeamRun.Query().Order(teamrun.ByCreatedAt(entsql.OrderDesc()))
	if teamID != "" {
		q = q.Where(teamrun.TeamIDEQ(teamID))
	}
	rows, err := q.Limit(limit).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.TeamRunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamRunToBiz(row))
	}
	return out, nil
}

func (r *TeamRepo) ListTeamRunsByTeamIDs(ctx context.Context, teamIDs []string, limit int) (map[string][]biz.TeamRunRecord, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).TeamRun.Query().
		Where(teamrun.TeamIDIn(teamIDs...)).
		Order(teamrun.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit * len(teamIDs)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	result := make(map[string][]biz.TeamRunRecord, len(teamIDs))
	for _, row := range rows {
		bizRun := entTeamRunToBiz(row)
		result[bizRun.TeamID] = append(result[bizRun.TeamID], bizRun)
	}
	return result, nil
}

func (r *TeamRepo) HasActiveTeamRun(ctx context.Context, teamID string) (bool, error) {
	// Active statuses must match the DB partial unique index (migration 20260724):
	//   WHERE status NOT IN ('success', 'failed', 'cancelled')
	// i.e., active = pending, running, waiting_human.
	count, err := r.data.RW().Read(ctx).TeamRun.Query().
		Where(
			teamrun.TeamIDEQ(teamID),
			teamrun.StatusIn(biz.TeamRunStatusRunning, biz.TeamRunStatusPending, biz.TeamRunStatusWaitingHuman),
		).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, entErrToBizErr(err, "TEAM")
	}
	return count > 0, nil
}

func (r *TeamRepo) GetTeamRunByID(ctx context.Context, id string) (biz.TeamRunRecord, error) {
	row, err := r.data.RW().Read(ctx).TeamRun.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.TeamRunRecord{}, apierror.NotFound(apierror.DomainTeam, "not found")
		}
		return biz.TeamRunRecord{}, entErrToBizErr(err, "TEAM")
	}
	return entTeamRunToBiz(row), nil
}

func (r *TeamRepo) ListTeamRunSteps(ctx context.Context, runID string) ([]biz.TeamRunStep, error) {
	rows, err := r.data.RW().Read(ctx).TeamRunStep.Query().
		Where(teamrunstep.RunIDEQ(runID)).
		Order(teamrunstep.BySortOrder(entsql.OrderAsc()), teamrunstep.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.TeamRunStep, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamRunStepToBiz(row))
	}
	return out, nil
}

func (r *TeamRepo) CreateTeamRun(ctx context.Context, run biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.TeamID) == "" {
		return biz.TeamRunRecord{}, apierror.BadRequest("TEAM_RUN", "team run id and team_id are required")
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
		return biz.TeamRunRecord{}, entErrToBizErr(err, "TEAM")
	}
	row, err := r.data.RW().Read(ctx).TeamRun.Get(ctx, run.ID)
	if err != nil {
		return biz.TeamRunRecord{}, entErrToBizErr(err, "TEAM")
	}
	return entTeamRunToBiz(row), nil
}

func (r *TeamRepo) UpdateTeamRun(ctx context.Context, run biz.TeamRunRecord) error {
	if strings.TrimSpace(run.ID) == "" {
		return apierror.BadRequest("TEAM_RUN", "team run id is required")
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
	return entErrToBizErr(err, "TEAM")
}

// UpdateTeamRunWhereStatus performs a Compare-And-Swap update on the team run
// status field. The row is updated only if its current status equals
// expectedCurrentStatus. Returns true if the row was updated, false if the
// current status did not match (concurrent modification). Terminal statuses
// also set finished_at.
func (r *TeamRepo) UpdateTeamRunWhereStatus(ctx context.Context, runID, newStatus, expectedCurrentStatus string) (bool, error) {
	if strings.TrimSpace(runID) == "" {
		return false, apierror.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	builder := r.data.RW().Write(ctx).TeamRun.Update().
		Where(teamrun.IDEQ(runID), teamrun.StatusEQ(expectedCurrentStatus)).
		SetStatus(newStatus).
		SetUpdatedAt(now)
	// Set finished_at for terminal statuses.
	if biz.IsTeamRunTerminalStatus(newStatus) {
		builder = builder.SetFinishedAt(now)
	}
	count, err := builder.Save(ctx)
	if err != nil {
		return false, entErrToBizErr(err, "TEAM")
	}
	return count > 0, nil
}

func (r *TeamRepo) CreateTeamRunStep(ctx context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.RunID) == "" {
		return biz.TeamRunStep{}, apierror.BadRequest("TEAM_RUN_STEP", "step id and run_id are required")
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
		return biz.TeamRunStep{}, entErrToBizErr(err, "TEAM")
	}
	row, err := r.data.RW().Read(ctx).TeamRunStep.Get(ctx, step.ID)
	if err != nil {
		return biz.TeamRunStep{}, entErrToBizErr(err, "TEAM")
	}
	return entTeamRunStepToBiz(row), nil
}

func (r *TeamRepo) UpdateTeamRunSummaryJSON(ctx context.Context, runID, summaryJSON string) error {
	if strings.TrimSpace(runID) == "" {
		return apierror.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE team_runs SET summary_json=?, updated_at=? WHERE id=?`),
		summaryJSON, now, runID)
	return entErrToBizErr(err, "TEAM")
}

func (r *TeamRepo) UpdateTeamRunGraphExecutionID(ctx context.Context, runID, graphExecutionID string) error {
	if strings.TrimSpace(runID) == "" {
		return apierror.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE team_runs SET graph_execution_id=?, updated_at=? WHERE id=?`),
		graphExecutionID, now, runID)
	return entErrToBizErr(err, "TEAM")
}

func (r *TeamRepo) UpdateTeamRunTraceID(ctx context.Context, runID, traceID string) error {
	if strings.TrimSpace(runID) == "" {
		return apierror.BadRequest("TEAM_RUN", "team run id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE team_runs SET trace_id=?, updated_at=? WHERE id=?`),
		traceID, now, runID)
	return entErrToBizErr(err, "TEAM")
}

func (r *TeamRepo) BatchCreateOrchestrationSteps(ctx context.Context, steps []biz.OrchestrationStep) error {
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
	return entErrToBizErr(err, "TEAM")
}

func (r *TeamRepo) ListOrchestrationSteps(ctx context.Context, teamRunID, nodeID string, limit int) ([]biz.OrchestrationStep, error) {
	teamRunID = strings.TrimSpace(teamRunID)
	if teamRunID == "" {
		return nil, apierror.BadRequest("ORCHESTRATION_STEP", "team_run_id is required")
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
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.OrchestrationStep, 0, len(rows))
	for _, row := range rows {
		out = append(out, entOrchestrationStepToBiz(row))
	}
	return out, nil
}

func (r *TeamRepo) CreateTaskDeadLetter(ctx context.Context, dl biz.TaskDeadLetter) error {
	if strings.TrimSpace(dl.ID) == "" {
		return apierror.BadRequest("TASK_DEAD_LETTER", "task dead letter id is required")
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
	return entErrToBizErr(err, "TEAM")
}

func (r *TeamRepo) ListTaskDeadLetters(ctx context.Context, filter biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
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
		return nil, entErrToBizErr(err, "TEAM")
	}
	out := make([]biz.TaskDeadLetter, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTaskDeadLetterToBiz(row))
	}
	return out, nil
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
