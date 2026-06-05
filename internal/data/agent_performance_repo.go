package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var _ biz.AgentPerformanceRepository = (*agentPerformanceRepo)(nil)

type agentPerformanceRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewAgentPerformanceRepo implements biz.AgentPerformanceRepository.
func NewAgentPerformanceRepo(d *Data, lg loggateway.Logger) biz.AgentPerformanceRepository {
	return &agentPerformanceRepo{data: d, lg: lg}
}

func (r *agentPerformanceRepo) Get(ctx context.Context, agentKey, taskType string) (*biz.AgentPerformance, error) {
	agentKey = strings.TrimSpace(agentKey)
	taskType = strings.TrimSpace(taskType)
	if agentKey == "" || taskType == "" {
		return nil, kerrors.BadRequest("AGENT_PERF", "agent_key and task_type are required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, agent_key, task_type, total_runs, success_runs, success_rate,
			avg_dq_score, avg_duration_ms, last_executed_at
		 FROM agent_performances WHERE agent_key = ? AND task_type = ?`, agentKey, taskType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	perf, err := scanAgentPerformanceFromRows(rows)
	if err != nil {
		return nil, err
	}
	return perf, nil
}

func (r *agentPerformanceRepo) GetBestForTaskType(ctx context.Context, taskType string, limit int) ([]*biz.AgentPerformance, error) {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, agent_key, task_type, total_runs, success_runs, success_rate,
			avg_dq_score, avg_duration_ms, last_executed_at
		 FROM agent_performances WHERE task_type = ?
		 ORDER BY success_rate DESC, avg_dq_score DESC
		 LIMIT ?`, taskType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perfs []*biz.AgentPerformance
	for rows.Next() {
		perf, err := scanAgentPerformanceFromRows(rows)
		if err != nil {
			return nil, err
		}
		perfs = append(perfs, perf)
	}
	return perfs, nil
}

func (r *agentPerformanceRepo) Upsert(ctx context.Context, perf *biz.AgentPerformance) error {
	if perf == nil {
		return nil
	}
	perf.AgentKey = strings.TrimSpace(perf.AgentKey)
	perf.TaskType = strings.TrimSpace(perf.TaskType)
	if perf.AgentKey == "" || perf.TaskType == "" {
		return kerrors.BadRequest("AGENT_PERF", "agent_key and task_type are required")
	}
	id := perf.AgentKey + "_" + perf.TaskType

	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		`INSERT OR REPLACE INTO agent_performances
			(id, agent_key, task_type, total_runs, success_runs, success_rate,
			 avg_dq_score, avg_duration_ms, last_executed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, perf.AgentKey, perf.TaskType,
		perf.TotalRuns, perf.SuccessRuns, perf.SuccessRate,
		perf.AvgDQScore, perf.AvgDurationMs, perf.LastExecutedAt,
	)
	return err
}

// EnsureAgentPerformanceSchema creates the agent_performances table if it does not exist.
// Called during DDL migration.
func EnsureAgentPerformanceSchema(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS agent_performances (
		id TEXT PRIMARY KEY,
		agent_key TEXT DEFAULT '',
		task_type TEXT DEFAULT '',
		total_runs INTEGER DEFAULT 0,
		success_runs INTEGER DEFAULT 0,
		success_rate REAL DEFAULT 0,
		avg_dq_score REAL DEFAULT 0,
		avg_duration_ms INTEGER DEFAULT 0,
		last_executed_at TEXT DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create agent_performances table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_performances_key_type ON agent_performances(agent_key, task_type)`); err != nil {
		return fmt.Errorf("create agent_performances index: %w", err)
	}
	return nil
}

func scanAgentPerformanceFromRows(rows *sql.Rows) (*biz.AgentPerformance, error) {
	var perf biz.AgentPerformance
	var id string

	err := rows.Scan(
		&id, &perf.AgentKey, &perf.TaskType,
		&perf.TotalRuns, &perf.SuccessRuns, &perf.SuccessRate,
		&perf.AvgDQScore, &perf.AvgDurationMs, &perf.LastExecutedAt,
	)
	if err != nil {
		return nil, err
	}

	return &perf, nil
}
