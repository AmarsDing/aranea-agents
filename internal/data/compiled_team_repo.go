package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type compiledTeamRepo struct {
	data          *Data
	runtimeReader biz.SessionRuntimeReader
}

var _ biz.CompiledTeamRepo = (*compiledTeamRepo)(nil)

func NewCompiledTeamRepo(d *Data, runtimeReader biz.SessionRuntimeReader) biz.CompiledTeamRepo {
	return &compiledTeamRepo{data: d, runtimeReader: runtimeReader}
}

func (r *compiledTeamRepo) readDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().ReadDB(ctx)
}

func (r *compiledTeamRepo) writeDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().WriteDB(ctx)
}

func compiledTeamRowID(teamID, graphID string) string {
	return teamID + ":" + graphID
}

func (r *compiledTeamRepo) Save(ctx context.Context, teamID, graphID, sessionID string, ct *biz.CompiledTeam) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	if ct == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	graphID = strings.TrimSpace(graphID)
	sessionID = strings.TrimSpace(sessionID)
	if teamID == "" || graphID == "" {
		return fmt.Errorf("compiled_team repo save: team_id and graph_id required")
	}
	configJSON, err := json.Marshal(ct)
	if err != nil {
		return fmt.Errorf("compiled_team repo save: marshal: %w", err)
	}
	id := compiledTeamRowID(teamID, graphID)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
INSERT OR REPLACE INTO compiled_teams
  (id, team_id, graph_id, session_id, config_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, teamID, graphID, sessionID, string(configJSON), now, now,
	)
	if err != nil {
		return fmt.Errorf("compiled_team repo save: %w", err)
	}
	return nil
}

func (r *compiledTeamRepo) Load(ctx context.Context, teamID, graphID string) (*biz.CompiledTeam, error) {
	db := r.readDB(ctx)
	if db == nil {
		return nil, sql.ErrNoRows
	}
	teamID = strings.TrimSpace(teamID)
	graphID = strings.TrimSpace(graphID)
	id := compiledTeamRowID(teamID, graphID)
	var configJSON string
	err := queryRowScan(ctx, db, `SELECT config_json FROM compiled_teams WHERE id = ? LIMIT 1`, []any{id}, &configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("compiled_team repo load: not found: %s", id)
		}
		return nil, fmt.Errorf("compiled_team repo load: %w", err)
	}
	var ct biz.CompiledTeam
	if err := json.Unmarshal([]byte(configJSON), &ct); err != nil {
		return nil, fmt.Errorf("compiled_team repo load: unmarshal: %w", err)
	}
	return &ct, nil
}

// LoadForSession loads a compiled team and verifies the session is still active
// via SessionRuntimeRepo. Returns the compiled team only if the session exists.
func (r *compiledTeamRepo) LoadForSession(ctx context.Context, teamID, graphID, sessionID string) (*biz.CompiledTeam, error) {
	ct, err := r.Load(ctx, teamID, graphID)
	if err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" && r.runtimeReader != nil {
		rt, err := r.runtimeReader.GetSessionRuntime(ctx, sessionID)
		if err != nil || rt == nil {
			return nil, fmt.Errorf("compiled_team repo load_for_session: session %s not active", sessionID)
		}
	}
	return ct, nil
}

func (r *compiledTeamRepo) Delete(ctx context.Context, teamID, graphID string) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	id := compiledTeamRowID(strings.TrimSpace(teamID), strings.TrimSpace(graphID))
	_, err := db.ExecContext(ctx, `DELETE FROM compiled_teams WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("compiled_team repo delete: %w", err)
	}
	return nil
}
