package repository

import (
	"errors"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) ListTeams() ([]domain.Team, error) {
	rows, err := r.db.Query(`SELECT id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at FROM teams WHERE deleted_at = '' ORDER BY is_default DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Team
	for rows.Next() {
		var v domain.Team
		if err = rows.Scan(&v.ID, &v.TeamKey, &v.DisplayName, &v.Status, &v.IsDefault, &v.DefinitionJSON, &v.ADKAppName, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) GetTeamByID(id string) (domain.Team, error) {
	row := r.db.QueryRow(`SELECT id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at FROM teams WHERE id = ? AND deleted_at = ''`, id)
	var v domain.Team
	if err := row.Scan(&v.ID, &v.TeamKey, &v.DisplayName, &v.Status, &v.IsDefault, &v.DefinitionJSON, &v.ADKAppName, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt); err != nil {
		return domain.Team{}, err
	}
	return v, nil
}

func (r *SQLiteRepository) CreateTeam(t domain.Team) (domain.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return domain.Team{}, errors.New("missing required fields")
	}
	now := nowISO()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.db.Exec(`INSERT INTO teams(id, team_key, display_name, status, is_default, definition_json, adk_app_name, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TeamKey, t.DisplayName, t.Status, t.IsDefault, t.DefinitionJSON, t.ADKAppName, t.CreatedAt, t.UpdatedAt, t.DeletedAt)
	return t, err
}

func (r *SQLiteRepository) UpdateTeam(t domain.Team) (domain.Team, error) {
	if t.ID == "" || t.TeamKey == "" || t.DisplayName == "" {
		return domain.Team{}, errors.New("missing required fields")
	}
	t.UpdatedAt = nowISO()
	if t.Status == "" {
		t.Status = "active"
	}
	_, err := r.db.Exec(`UPDATE teams SET team_key = ?, display_name = ?, status = ?, is_default = ?, definition_json = ?, adk_app_name = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		t.TeamKey, t.DisplayName, t.Status, t.IsDefault, t.DefinitionJSON, t.ADKAppName, t.UpdatedAt, t.ID)
	if err != nil {
		return domain.Team{}, err
	}
	return r.GetTeamByID(t.ID)
}

func (r *SQLiteRepository) DeleteTeam(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	_, err := r.db.Exec(`UPDATE teams SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = '' AND is_default = 0`, nowISO(), nowISO(), id)
	return err
}

func (r *SQLiteRepository) AddTeamRun(run domain.TeamRun) (domain.TeamRun, error) {
	now := nowISO()
	if run.ID == "" || run.TeamID == "" {
		return domain.TeamRun{}, errors.New("team run id and team_id are required")
	}
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
		run.Status = "running"
	}
	if run.TopologyJSON == "" {
		run.TopologyJSON = "{}"
	}
	_, err := r.db.Exec(`INSERT INTO team_runs(id, team_id, session_id, message_id, mode, status, input_preview, output_preview, token_in, token_out, cost_micro_usd, duration_ms, error_message, topology_json, started_at, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TeamID, run.SessionID, run.MessageID, run.Mode, run.Status, run.InputPreview, run.OutputPreview, run.TokenIn, run.TokenOut, run.CostMicroUSD, run.DurationMS, run.ErrorMessage, run.TopologyJSON, run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt)
	return run, err
}

func (r *SQLiteRepository) UpdateTeamRun(run domain.TeamRun) (domain.TeamRun, error) {
	if run.ID == "" {
		return domain.TeamRun{}, errors.New("team run id is required")
	}
	run.UpdatedAt = nowISO()
	_, err := r.db.Exec(`UPDATE team_runs SET message_id = ?, status = ?, output_preview = ?, token_in = ?, token_out = ?, cost_micro_usd = ?, duration_ms = ?, error_message = ?, topology_json = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		run.MessageID, run.Status, run.OutputPreview, run.TokenIn, run.TokenOut, run.CostMicroUSD, run.DurationMS, run.ErrorMessage, run.TopologyJSON, run.FinishedAt, run.UpdatedAt, run.ID)
	if err != nil {
		return domain.TeamRun{}, err
	}
	items, err := r.ListTeamRuns(run.TeamID, 100)
	if err != nil {
		return domain.TeamRun{}, err
	}
	for _, item := range items {
		if item.ID == run.ID {
			return item, nil
		}
	}
	return run, nil
}

func (r *SQLiteRepository) AddTeamRunStep(step domain.TeamRunStep) (domain.TeamRunStep, error) {
	now := nowISO()
	if step.ID == "" || step.RunID == "" || step.TeamID == "" {
		return domain.TeamRunStep{}, errors.New("team run step id, run_id and team_id are required")
	}
	if step.CreatedAt == "" {
		step.CreatedAt = now
	}
	if step.StartedAt == "" {
		step.StartedAt = now
	}
	if step.FinishedAt == "" {
		step.FinishedAt = now
	}
	if step.Status == "" {
		step.Status = "success"
	}
	_, err := r.db.Exec(`INSERT INTO team_run_steps(id, run_id, team_id, agent_id, agent_key, agent_name, role, sort_order, status, input_preview, output_preview, token_in, token_out, cost_micro_usd, duration_ms, error_message, started_at, finished_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.RunID, step.TeamID, step.AgentID, step.AgentKey, step.AgentName, step.Role, step.SortOrder, step.Status, step.InputPreview, step.OutputPreview, step.TokenIn, step.TokenOut, step.CostMicroUSD, step.DurationMS, step.ErrorMessage, step.StartedAt, step.FinishedAt, step.CreatedAt)
	return step, err
}

func (r *SQLiteRepository) ListTeamRuns(teamID string, limit int) ([]domain.TeamRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	where := "1=1"
	args := []any{}
	if teamID != "" {
		where = "team_id = ?"
		args = append(args, teamID)
	}
	args = append(args, limit)
	rows, err := r.db.Query(`SELECT id, team_id, session_id, message_id, mode, status, input_preview, output_preview, token_in, token_out, cost_micro_usd, duration_ms, error_message, topology_json, started_at, finished_at, created_at, updated_at FROM team_runs WHERE `+where+` ORDER BY created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.TeamRun{}
	for rows.Next() {
		var item domain.TeamRun
		if err = rows.Scan(&item.ID, &item.TeamID, &item.SessionID, &item.MessageID, &item.Mode, &item.Status, &item.InputPreview, &item.OutputPreview, &item.TokenIn, &item.TokenOut, &item.CostMicroUSD, &item.DurationMS, &item.ErrorMessage, &item.TopologyJSON, &item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) ListTeamRunSteps(runID string) ([]domain.TeamRunStep, error) {
	rows, err := r.db.Query(`SELECT id, run_id, team_id, agent_id, agent_key, agent_name, role, sort_order, status, input_preview, output_preview, token_in, token_out, cost_micro_usd, duration_ms, error_message, started_at, finished_at, created_at FROM team_run_steps WHERE run_id = ? ORDER BY sort_order ASC, created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.TeamRunStep{}
	for rows.Next() {
		var item domain.TeamRunStep
		if err = rows.Scan(&item.ID, &item.RunID, &item.TeamID, &item.AgentID, &item.AgentKey, &item.AgentName, &item.Role, &item.SortOrder, &item.Status, &item.InputPreview, &item.OutputPreview, &item.TokenIn, &item.TokenOut, &item.CostMicroUSD, &item.DurationMS, &item.ErrorMessage, &item.StartedAt, &item.FinishedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
