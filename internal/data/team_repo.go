package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/team"
	"aranea-agents/internal/data/ent/teamrun"
	"aranea-agents/internal/data/ent/teamrunstep"

	entsql "entgo.io/ent/dialect/sql"
)

type teamRepo struct {
	data *Data
}

// NewTeamRepo implements biz.TeamRepository.
func NewTeamRepo(d *Data) biz.TeamRepository {
	return &teamRepo{data: d}
}

func entTeamToBiz(e *ent.Team) biz.Team {
	if e == nil {
		return biz.Team{}
	}
	return biz.Team{
		ID:             e.ID,
		TeamKey:        e.TeamKey,
		DisplayName:    e.DisplayName,
		Status:         e.Status,
		IsDefault:      e.IsDefault,
		DefinitionJSON: e.DefinitionJSON,
		ADKAppName:     e.AdkAppName,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		DeletedAt:      e.DeletedAt,
	}
}

func entTeamRunToBiz(e *ent.TeamRun) biz.TeamRun {
	if e == nil {
		return biz.TeamRun{}
	}
	return biz.TeamRun{
		ID:            e.ID,
		TeamID:        e.TeamID,
		SessionID:     e.SessionID,
		MessageID:     e.MessageID,
		Mode:          e.Mode,
		Status:        e.Status,
		InputPreview:  e.InputPreview,
		OutputPreview: e.OutputPreview,
		TokenIn:       e.TokenIn,
		TokenOut:      e.TokenOut,
		CostMicroUSD:  e.CostMicroUsd,
		DurationMS:    e.DurationMs,
		ErrorMessage:  e.ErrorMessage,
		TopologyJSON:  e.TopologyJSON,
		StartedAt:     e.StartedAt,
		FinishedAt:    e.FinishedAt,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
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
	}
}

func (r *teamRepo) ListTeams(ctx context.Context) ([]biz.Team, error) {
	c := r.data.entClient
	rows, err := c.Team.Query().Where(team.DeletedAtEQ("")).
		Order(team.ByIsDefault(entsql.OrderDesc()), team.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, entTeamToBiz(row))
	}
	return out, nil
}

func (r *teamRepo) GetTeamByID(ctx context.Context, id string) (biz.Team, error) {
	c := r.data.entClient
	row, err := c.Team.Query().Where(team.IDEQ(id), team.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Team{}, sql.ErrNoRows
		}
		return biz.Team{}, err
	}
	return entTeamToBiz(row), nil
}

func (r *teamRepo) CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return biz.Team{}, fmt.Errorf("missing required fields")
	}
	now := nowRFC3339()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.data.entClient.Team.Create().
		SetID(t.ID).
		SetTeamKey(t.TeamKey).
		SetDisplayName(t.DisplayName).
		SetStatus(t.Status).
		SetIsDefault(t.IsDefault).
		SetDefinitionJSON(t.DefinitionJSON).
		SetAdkAppName(t.ADKAppName).
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
		return biz.Team{}, fmt.Errorf("missing required fields")
	}
	now := nowRFC3339()
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.data.entClient.Team.UpdateOneID(t.ID).
		SetTeamKey(t.TeamKey).
		SetDisplayName(t.DisplayName).
		SetStatus(t.Status).
		SetIsDefault(t.IsDefault).
		SetDefinitionJSON(t.DefinitionJSON).
		SetAdkAppName(t.ADKAppName).
		SetUpdatedAt(t.UpdatedAt).
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
		return fmt.Errorf("id is required")
	}
	now := nowRFC3339()
	_, err := r.data.entClient.Team.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *teamRepo) ListTeamRuns(ctx context.Context, teamID string, limit int) ([]biz.TeamRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := r.data.entClient.TeamRun.Query().Order(teamrun.ByCreatedAt(entsql.OrderDesc()))
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

func (r *teamRepo) ListTeamRunSteps(ctx context.Context, runID string) ([]biz.TeamRunStep, error) {
	rows, err := r.data.entClient.TeamRunStep.Query().
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
