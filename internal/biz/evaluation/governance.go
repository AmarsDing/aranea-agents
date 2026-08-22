package evaluation

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// Governance capabilities (P2-1 publish gate / P2-3 failure grouping /
// P3-3 pairwise preference). Kept out of evaluation.go to respect the
// 500-line file budget (AS-COG-01).

// ── P2-3: failure grouping ──────────────────────────────────────────────────

// FailureGroup aggregates failed case results sharing one error_message.
type FailureGroup struct {
	ErrorMessage string
	Count        int
	RunCount     int
	LatestAt     string
}

// FailureGroupReport is the dataset-level failure grouping result.
type FailureGroupReport struct {
	TotalFailed int
	Groups      []FailureGroup
}

// GetFailureGroups groups failed case results of one dataset by
// error_message, newest activity first ordered by frequency (P2-3).
func (u *Usecase) GetFailureGroups(ctx context.Context, datasetID, agentID string, limit int) (FailureGroupReport, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return FailureGroupReport{}, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	groups, total, err := u.results.ListFailureGroups(ctx, datasetID, agentID, limit)
	if err != nil {
		return FailureGroupReport{}, err
	}
	return FailureGroupReport{TotalFailed: total, Groups: groups}, nil
}

// ── P3-3: pairwise run preference ───────────────────────────────────────────

// RunPreference records a human judgment that one run's output is better
// than another's for the same dataset (simplified T3 pairwise annotation).
type RunPreference struct {
	ID          string
	DatasetID   string
	RunIDA      string
	RunIDB      string
	WinnerRunID string
	Comment     string
	CreatedBy   string
	CreatedAt   string
}

// SubmitRunPreference validates and persists one pairwise judgment.
func (u *Usecase) SubmitRunPreference(ctx context.Context, in RunPreference) (RunPreference, error) {
	in.DatasetID = strings.TrimSpace(in.DatasetID)
	in.RunIDA = strings.TrimSpace(in.RunIDA)
	in.RunIDB = strings.TrimSpace(in.RunIDB)
	in.WinnerRunID = strings.TrimSpace(in.WinnerRunID)
	if in.DatasetID == "" || in.RunIDA == "" || in.RunIDB == "" || in.WinnerRunID == "" {
		return RunPreference{}, apierror.BadRequest("EVAL", "dataset_id, run_id_a, run_id_b and winner_run_id are required")
	}
	if in.RunIDA == in.RunIDB {
		return RunPreference{}, apierror.BadRequest("EVAL", "run_id_a and run_id_b must differ")
	}
	if in.WinnerRunID != in.RunIDA && in.WinnerRunID != in.RunIDB {
		return RunPreference{}, apierror.BadRequest("EVAL", "winner_run_id must be run_id_a or run_id_b")
	}
	// Both runs must exist and belong to the dataset — a preference over
	// foreign runs would corrupt per-dataset preference statistics.
	runs, err := u.runQueries.GetRunsByIDs(ctx, []string{in.RunIDA, in.RunIDB})
	if err != nil {
		return RunPreference{}, err
	}
	if len(runs) != 2 {
		return RunPreference{}, apierror.BadRequest("EVAL", "both runs must exist")
	}
	for _, r := range runs {
		if r.DatasetID != in.DatasetID {
			return RunPreference{}, apierror.BadRequest("EVAL", "runs must belong to the given dataset")
		}
	}
	if in.ID == "" {
		in.ID = newEvalID()
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		in.CreatedBy = "system"
	}
	if err := u.gov.InsertRunPreference(ctx, in); err != nil {
		return RunPreference{}, err
	}
	return in, nil
}

// ListRunPreferences returns recorded pairwise judgments for a dataset.
func (u *Usecase) ListRunPreferences(ctx context.Context, datasetID string, limit int) ([]RunPreference, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	return u.gov.ListRunPreferences(ctx, datasetID, limit)
}

// ── P2-1: publish regression gate config ────────────────────────────────────

// Gate modes: advisory notifies after publish; blocking waits for the
// regression and returns Conflict on threshold breach. Default is advisory.
const (
	GateModeAdvisory = "advisory"
	GateModeBlocking = "blocking"
)

// GateConfig is a publish-gate configuration. AgentID="" is the platform
// default; a non-empty AgentID is a per-agent override.
type GateConfig struct {
	Enabled   bool
	AgentID   string
	DatasetID string
	Metric    string
	MinScore  float32 // absolute floor; 0 disables
	MaxDrop   float32 // allowed drop vs latest completed baseline; 0 disables
	Mode      string  // advisory | blocking; empty = advisory
	UpdatedAt string
}

// GateTriggerSkillPublish / GateTriggerPackInstall identify the gated
// operation in notifications and logs.
const (
	GateTriggerSkillPublish = "skill_publish"
	GateTriggerPackInstall  = "pack_install"
)

// NormalizeGateMode returns advisory for empty/unknown values.
func NormalizeGateMode(mode string) string {
	if strings.TrimSpace(mode) == GateModeBlocking {
		return GateModeBlocking
	}
	return GateModeAdvisory
}

// GetGateConfig returns the gate config for agentID (empty = platform default).
// A missing per-agent row falls back to the default; a missing default is a
// disabled zero config.
func (u *Usecase) GetGateConfig(ctx context.Context, agentID string) (GateConfig, error) {
	cfg, err := u.gov.GetGateConfig(ctx, strings.TrimSpace(agentID))
	if err != nil {
		return GateConfig{}, err
	}
	cfg.Mode = NormalizeGateMode(cfg.Mode)
	return cfg, nil
}

// UpdateGateConfig validates and persists the gate config (per-agent when
// AgentID is set, otherwise the platform default).
func (u *Usecase) UpdateGateConfig(ctx context.Context, cfg GateConfig) (GateConfig, error) {
	cfg.AgentID = strings.TrimSpace(cfg.AgentID)
	cfg.DatasetID = strings.TrimSpace(cfg.DatasetID)
	cfg.Metric = strings.TrimSpace(cfg.Metric)
	cfg.Mode = NormalizeGateMode(cfg.Mode)
	if cfg.Metric == "" {
		cfg.Metric = "exact_match"
	}
	cfg.MinScore = clamp01(cfg.MinScore)
	cfg.MaxDrop = clamp01(cfg.MaxDrop)
	if cfg.Enabled && (cfg.AgentID == "" || cfg.DatasetID == "") {
		return GateConfig{}, apierror.BadRequest("EVAL", "agent_id and dataset_id are required when the gate is enabled")
	}
	if err := u.gov.UpsertGateConfig(ctx, cfg); err != nil {
		return GateConfig{}, err
	}
	return u.GetGateConfig(ctx, cfg.AgentID)
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// RunMetricScore extracts one metric's run-level score. scores_json key
// presence is authoritative (mirrors the judge-detection rule in
// divergence.go); the four legacy columns are the fallback for runs written
// before scores_json existed.
func RunMetricScore(run Run, metric string) (float32, bool) {
	metric = strings.TrimSpace(metric)
	if metric == "" {
		return 0, false
	}
	if v, ok := ParseScores(run.ScoresJSON)[metric]; ok {
		return v, true
	}
	switch metric {
	case "exact_match":
		return run.ExactMatchScore, true
	case "contains_match":
		return run.ContainsMatchScore, true
	case "llm_as_judge":
		return run.LLMJudgeScore, true
	case "tool_call_accuracy", "tool_trajectory":
		return run.ToolCallAccuracy, true
	}
	return 0, false
}
