package biz

import (
	"encoding/json"
	"strings"
)

// AgentEvalAutoConfig is stored under config_json.evaluation for AfterTurn auto-eval (US-5).
type AgentEvalAutoConfig struct {
	Enabled        bool   `json:"auto_after_turn"`
	DatasetID      string `json:"dataset_id"`
	Metrics        string `json:"metrics"`
	NumRuns        int    `json:"num_runs"`
	MinIntervalSec int    `json:"min_interval_sec"`
}

// ParseAgentEvalAutoConfig reads evaluation.auto_after_turn settings from agent config_json.
func ParseAgentEvalAutoConfig(configJSON string) AgentEvalAutoConfig {
	configJSON = strings.TrimSpace(configJSON)
	if configJSON == "" || configJSON == "{}" {
		return AgentEvalAutoConfig{}
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &root); err != nil {
		return AgentEvalAutoConfig{}
	}
	raw, ok := root["evaluation"]
	if !ok {
		return AgentEvalAutoConfig{}
	}
	var cfg AgentEvalAutoConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return AgentEvalAutoConfig{}
	}
	cfg.DatasetID = strings.TrimSpace(cfg.DatasetID)
	if cfg.NumRuns <= 0 {
		cfg.NumRuns = 1
	}
	if cfg.MinIntervalSec <= 0 {
		cfg.MinIntervalSec = 300
	}
	return cfg
}
