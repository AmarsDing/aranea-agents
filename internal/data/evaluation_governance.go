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
// EVAL-03: the join must carry the run workspace filter — shared datasets
// (workspace="") are readable by every tenant, and without it one tenant's
// failed runs (error text included) leak into another tenant's grouping.
func (r *evalRepo) ListFailureGroups(ctx context.Context, datasetID, agentID string, limit int) ([]biz.EvalFailureGroup, int, error) {
	if limit <= 0 {
		limit = 20
	}
	d := r.data.Dialect()
	wsClause, wsArgs := evalRunsWorkspaceFilter(ctx)
	baseWhere := `ru.dataset_id=? AND (?='' OR ru.agent_id=?) AND r.error_message != ''` + wsClause
	baseArgs := append([]any{datasetID, agentID, agentID}, wsArgs...)
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		d.RenumberPlaceholders(`SELECT COUNT(*) FROM eval_case_results r
			JOIN eval_runs ru ON ru.id = r.run_id
			WHERE `+baseWhere),
		baseArgs, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	queryArgs := append(append([]any{}, baseArgs...), limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		d.RenumberPlaceholders(`SELECT r.error_message, COUNT(*) AS cnt, COUNT(DISTINCT r.run_id) AS run_count, MAX(r.created_at) AS latest_at
			FROM eval_case_results r
			JOIN eval_runs ru ON ru.id = r.run_id
			WHERE `+baseWhere+`
			GROUP BY r.error_message ORDER BY cnt DESC, latest_at DESC LIMIT ?`),
		queryArgs...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	out := make([]biz.EvalFailureGroup, 0)
	for rows.Next() {
		var g biz.EvalFailureGroup
		if err := rows.Scan(&g.ErrorMessage, &g.Count, &g.RunCount, &g.LatestAt); err != nil {
			return nil, 0, entErrToBizErr(err, "EVAL")
		}
		out = append(out, g)
	}
	return out, total, entErrToBizErr(rows.Err(), "EVAL")
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
	return entErrToBizErr(err, "EVAL")
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
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	out := make([]biz.EvalRunPreference, 0)
	for rows.Next() {
		var p biz.EvalRunPreference
		if err := rows.Scan(&p.ID, &p.DatasetID, &p.RunIDA, &p.RunIDB, &p.WinnerRunID, &p.Comment, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, entErrToBizErr(err, "EVAL")
		}
		out = append(out, p)
	}
	return out, entErrToBizErr(rows.Err(), "EVAL")
}

// evalGateConfigID is the platform-default row key (legacy singleton).
const evalGateConfigID = "singleton"

func evalGateRowID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return evalGateConfigID
	}
	return "agent:" + agentID
}

func scanGateConfig(enabled int, cfg biz.EvalGateConfig) biz.EvalGateConfig {
	cfg.Enabled = enabled != 0
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "advisory"
	}
	return cfg
}

// GetGateConfig returns the per-agent gate when agentID is set and a row
// exists; otherwise the platform default. A missing default is a disabled zero.
func (r *evalRepo) GetGateConfig(ctx context.Context, agentID string) (biz.EvalGateConfig, error) {
	if cfg, ok, err := r.loadGateRow(ctx, evalGateRowID(agentID)); err != nil {
		return biz.EvalGateConfig{}, err
	} else if ok {
		return cfg, nil
	}
	if strings.TrimSpace(agentID) != "" {
		if cfg, ok, err := r.loadGateRow(ctx, evalGateConfigID); err != nil {
			return biz.EvalGateConfig{}, err
		} else if ok {
			return cfg, nil
		}
	}
	return biz.EvalGateConfig{Metric: "exact_match", Mode: "advisory"}, nil
}

func (r *evalRepo) loadGateRow(ctx context.Context, id string) (biz.EvalGateConfig, bool, error) {
	var cfg biz.EvalGateConfig
	var enabled int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT enabled,agent_id,dataset_id,metric,min_score,max_drop,COALESCE(mode,''),updated_at
			FROM eval_gate_config WHERE id=?`), []any{id},
		&enabled, &cfg.AgentID, &cfg.DatasetID, &cfg.Metric, &cfg.MinScore, &cfg.MaxDrop, &cfg.Mode, &cfg.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows || apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.EvalGateConfig{}, false, nil
		}
		return biz.EvalGateConfig{}, false, entErrToBizErr(err, "EVAL")
	}
	return scanGateConfig(enabled, cfg), true, nil
}

// UpsertGateConfig writes the default row (agent_id empty) or a per-agent row.
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
	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = "advisory"
	}
	id := evalGateRowID(cfg.AgentID)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_gate_config (id,enabled,agent_id,dataset_id,metric,min_score,max_drop,mode,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET enabled=?,agent_id=?,dataset_id=?,metric=?,min_score=?,max_drop=?,mode=?,updated_at=?`),
		id, enabled, cfg.AgentID, cfg.DatasetID, metric, cfg.MinScore, cfg.MaxDrop, mode, t,
		enabled, cfg.AgentID, cfg.DatasetID, metric, cfg.MinScore, cfg.MaxDrop, mode, t)
	return entErrToBizErr(err, "EVAL")
}
