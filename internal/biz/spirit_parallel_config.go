package biz

import (
	"encoding/json"
	"time"

	"aranea-agents/pkg/loggateway"
)

type ParallelConfig struct {
	MaxConcurrentTeams int `json:"max_concurrent_teams"`
	MaxTeamConcurrency int `json:"max_team_concurrency"`
	// TeamTimeoutSeconds is the maximum duration a team is allowed to run
	// after StartTeamTurn. The AfterFunc is registered at activation, not
	// AssembleTeam, so pending DAG dependents do not time out while waiting.
	TeamTimeoutSeconds int `json:"team_timeout_seconds"`
	AutoArchiveSeconds int `json:"auto_archive_seconds"`
	MaxSessionDepth    int `json:"max_session_depth"`
	// TimeoutHandlerDBTimeoutSec is the maximum duration for DB operations
	// inside the timeout callback goroutine (default 30s).
	TimeoutHandlerDBTimeoutSec int `json:"timeout_handler_db_timeout_sec"`
}

func DefaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		MaxConcurrentTeams:         3,
		MaxTeamConcurrency:         2,
		TeamTimeoutSeconds:         600,
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

func (c ParallelConfig) AutoArchiveAfter() time.Duration {
	return time.Duration(c.AutoArchiveSeconds) * time.Second
}

func (c ParallelConfig) TimeoutHandlerDBTimeout() time.Duration {
	return time.Duration(c.TimeoutHandlerDBTimeoutSec) * time.Second
}
