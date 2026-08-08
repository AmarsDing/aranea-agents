package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// Governance persistence for evaluation (P2-1 gate config / P2-3 failure
// grouping / P3-3 run preferences). Kept out of evaluation.go for the
// 500-line file budget (AS-COG-01).

// ListFailureGroups groups failed case results of one dataset by
// error_message (P2-3). Returns the groups (count desc, capped by limit)
// plus the total number of failed results regardless of the cap.
func (r *evalRepo) ListFailureGroups(ctx context.Context, datasetID, agentID string, limit int) ([]biz.EvalFailureGroup, int, error) {
	if limit <= 0 {
		limit = 20
	}
	d := r.data.Dialect()
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		d.RenumberPlaceholders(`SELECT COUNT(*) FROM eval_case_results r
			JOIN eval_runs ru ON ru.id = r.run_id
			WHERE ru.dataset_id=? AND (?='' OR ru.agent_id=?) AND r.error_message != ''`),
		[]any{datasetID, agentID, agentID}, &total); err != nil {
		return nil, 0, err
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		d.RenumberPlaceholders(`SELECT r.error_message, COUNT(*) AS cnt, COUNT(DISTINCT r.run_id) AS run_count, MAX(r.created_at) AS latest_at
			FROM eval_case_results r
			JOIN eval_runs ru ON ru.id = r.run_id
			WHERE ru.dataset_id=? AND (?='' OR ru.agent_id=?) AND r.error_message != ''
			GROUP BY r.error_message ORDER BY cnt DESC, latest_at DESC LIMIT ?`),
		datasetID, agentID, agentID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]biz.EvalFailureGroup, 0)
	for rows.Next() {
		var g biz.EvalFailureGroup
		if err := rows.Scan(&g.ErrorMessage, &g.Count, &g.RunCount, &g.LatestAt); err != nil {
			return nil, 0, err
		}
		out = append(out, g)
	}
	return out, total, rows.Err()
}

// InsertRunPreference persists one pairwise judgment (P3-3).
func (r *evalRepo) InsertRunPreference(ctx context.Context, p biz.EvalRunPreference) error {
	if p.CreatedAt == "" {
		p.CreatedAt = now()
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_run_preferences
		 (id,dataset_id,run_id_a,run_id_b,winner_run_id,comment,created_by,created_at)
		 VALUES (?,?,?,?,?,?,?,?)`),
		p.ID, p.DatasetID, p.RunIDA, p.RunIDB, p.WinnerRunID, p.Comment, p.CreatedBy, p.CreatedAt)
	return err
}

// ListRunPreferences returns pairwise judgments for a dataset, newest first.
func (r *evalRepo) ListRunPreferences(ctx context.Context, datasetID string, limit int) ([]biz.EvalRunPreference, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id,dataset_id,run_id_a,run_id_b,winner_run_id,comment,created_by,created_at
			FROM eval_run_preferences WHERE dataset_id=? ORDER BY created_at DESC LIMIT ?`),
		datasetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]biz.EvalRunPreference, 0)
	for rows.Next() {
		var p biz.EvalRunPreference
		if err := rows.Scan(&p.ID, &p.DatasetID, &p.RunIDA, &p.RunIDB, &p.WinnerRunID, &p.Comment, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// evalGateConfigID is the singleton row key for the publish-gate config.
const evalGateConfigID = "singleton"

// GetGateConfig returns the publish-gate singleton (P2-1); a missing row
// yields a disabled zero config.
func (r *evalRepo) GetGateConfig(ctx context.Context) (biz.EvalGateConfig, error) {
	var cfg biz.EvalGateConfig
	var enabled int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT enabled,agent_id,dataset_id,metric,min_score,max_drop,updated_at
			FROM eval_gate_config WHERE id=?`), []any{evalGateConfigID},
		&enabled, &cfg.AgentID, &cfg.DatasetID, &cfg.Metric, &cfg.MinScore, &cfg.MaxDrop, &cfg.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows || apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.EvalGateConfig{Metric: "exact_match"}, nil
		}
		return biz.EvalGateConfig{}, err
	}
	cfg.Enabled = enabled != 0
	return cfg, nil
}

// UpsertGateConfig writes the publish-gate singleton (P2-1).
func (r *evalRepo) UpsertGateConfig(ctx context.Context, cfg biz.EvalGateConfig) error {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	t := now()
	metric := strings.TrimSpace(cfg.Metric)
	if metric == "" {
		metric = "exact_match"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_gate_config (id,enabled,agent_id,dataset_id,metric,min_score,max_drop,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET enabled=?,agent_id=?,dataset_id=?,metric=?,min_score=?,max_drop=?,updated_at=?`),
		evalGateConfigID, enabled, cfg.AgentID, cfg.DatasetID, metric, cfg.MinScore, cfg.MaxDrop, t,
		enabled, cfg.AgentID, cfg.DatasetID, metric, cfg.MinScore, cfg.MaxDrop, t)
	return err
}
