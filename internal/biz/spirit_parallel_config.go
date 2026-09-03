package biz

import (
	"encoding/json"
	"time"

	"aranea-agents/pkg/loggateway"
)

type ParallelConfig struct {
	MaxConcurrentTeams int `json:"max_concurrent_teams"`
	MaxTeamConcurrency int `json:"max_team_concurrency"`
	// TeamTimeoutSeconds is the IDLE window for a running team (2026-09-03 F1,
	// lbg-verify-planner 复盘): the timeout fires only when no member activity
	// (steps_v2 started_at across the team session tree) has been observed for
	// this long. A team whose members are still progressing is re-armed for the
	// remaining window instead of being killed mid-work — the runner-level
	// guards (first-byte/stall budgets, no-progress auditor, token budget) own
	// member-level liveness; this timer is a backstop for a fully wedged run.
	// The AfterFunc is registered at activation, not AssembleTeam, so pending
	// DAG dependents do not time out while waiting.
	TeamTimeoutSeconds int `json:"team_timeout_seconds"`
	// TeamMaxLifetimeSeconds is the absolute ceiling for one team execution
	// attempt, measured from the latest run's started_at. It bounds the
	// fail-open extension path (probe errors / continuous activity) so a team
	// can never run forever. Default 14400 (4h).
	TeamMaxLifetimeSeconds int `json:"team_max_lifetime_seconds"`
	AutoArchiveSeconds     int `json:"auto_archive_seconds"`
	MaxSessionDepth        int `json:"max_session_depth"`
	// TimeoutHandlerDBTimeoutSec is the maximum duration for DB operations
	// inside the timeout callback goroutine (default 30s).
	TimeoutHandlerDBTimeoutSec int `json:"timeout_handler_db_timeout_sec"`
}

func DefaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		MaxConcurrentTeams:         3,
		MaxTeamConcurrency:         2,
		TeamTimeoutSeconds:         600,
		TeamMaxLifetimeSeconds:     14400,
		AutoArchiveSeconds:         3600,
		MaxSessionDepth:            2,
		TimeoutHandlerDBTimeoutSec: 30,
	}
}

func ParseParallelConfig(extraJSON string, lg loggateway.Logger) ParallelConfig {
	cfg := DefaultParallelConfig()
	if extraJSON == "" {
		return cfg
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(extraJSON), &raw); err != nil {
		lg.Warn("解析 parallel config json 失败", loggateway.StepID("spirit.parallel_config"), loggateway.Err(err))
		return cfg
	}
	if v, ok := raw["parallel_config"]; ok {
		var pc ParallelConfig
		if err := json.Unmarshal(v, &pc); err == nil {
			cfg = pc
		}
	}
	if cfg.MaxConcurrentTeams <= 0 {
		cfg.MaxConcurrentTeams = 3
	}
	if cfg.MaxTeamConcurrency <= 0 {
		cfg.MaxTeamConcurrency = 2
	}
	if cfg.TeamTimeoutSeconds <= 0 {
		cfg.TeamTimeoutSeconds = 600
	}
	if cfg.MaxSessionDepth <= 0 {
		cfg.MaxSessionDepth = 2
	}
	if cfg.TimeoutHandlerDBTimeoutSec <= 0 {
		cfg.TimeoutHandlerDBTimeoutSec = 30
	}
	return cfg
}

func (c ParallelConfig) TeamTimeout() time.Duration {
	return time.Duration(c.TeamTimeoutSeconds) * time.Second
}

// TeamMaxLifetime returns the absolute ceiling for one team execution attempt.
func (c ParallelConfig) TeamMaxLifetime() time.Duration {
	return time.Duration(c.TeamMaxLifetimeSeconds) * time.Second
}

func (c ParallelConfig) AutoArchiveAfter() time.Duration {
	return time.Duration(c.AutoArchiveSeconds) * time.Second
}

func (c ParallelConfig) TimeoutHandlerDBTimeout() time.Duration {
	return time.Duration(c.TimeoutHandlerDBTimeoutSec) * time.Second
}
