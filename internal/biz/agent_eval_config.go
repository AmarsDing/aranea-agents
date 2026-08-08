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
	// SampleRate gates what fraction of turns trigger an online eval run
	// (P2-2). (0,1] enables sampling; <=0 or >1 is normalized to 1.0 (every
	// eligible turn evaluates) for backward compatibility with configs that
	// predate the field.
	SampleRate float64 `json:"sample_rate"`
	// AlertConsecutiveDrops enables the online score-drop alert: when the
	// latest N after_turn runs each score strictly lower than the previous
	// one, a SystemNoticeEvent is published (P2-2). 0 disables alerting.
	AlertConsecutiveDrops int `json:"alert_consecutive_drops"`
	// AlertMetric selects which metric the drop detection watches
	// (exact_match | contains_match | llm_as_judge | tool_call_accuracy).
	// Empty defaults to llm_as_judge. The metric should be part of Metrics,
	// otherwise its score column stays 0 and would look like a permanent drop.
	AlertMetric string `json:"alert_metric"`
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
	if cfg.SampleRate <= 0 || cfg.SampleRate > 1 {
		cfg.SampleRate = 1
	}
	if cfg.AlertConsecutiveDrops < 0 {
		cfg.AlertConsecutiveDrops = 0
	}
	cfg.AlertMetric = strings.TrimSpace(cfg.AlertMetric)
	if cfg.AlertMetric == "" {
		cfg.AlertMetric = "llm_as_judge"
	}
	return cfg
}

// EvalAutoConfig returns the evaluation auto-config from the runtime settings.
// This is a placeholder — evaluation config is still stored in config_json
// (not yet migrated to agent_runtime_settings columns).
// TODO(D-03-phase2): migrate evaluation fields to agent_runtime_settings and
// implement this method. After migration, ParseAgentEvalAutoConfig can be removed.
func (s *AgentRuntimeSettings) EvalAutoConfig() AgentEvalAutoConfig {
	return AgentEvalAutoConfig{}
}
