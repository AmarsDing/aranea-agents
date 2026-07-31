// Package biz — Platform self-improvement triggers (V3, 73-self-iteration-v3).
//
// Four triggers implement EvolutionTrigger for EvolutionTargetPlatform, one
// per signal source (design D2):
//
//	ErrorClusterTrigger   — FailurePattern KB 聚类：同 error_code+相似堆栈窗口内高频出现
//	PerfBottleneckTrigger — monitor 指标：步骤 P95 / token 超基线倍数
//	EvalRegressionTrigger — evaluation 基线：综合分环比退化超阈值
//	TestFailureTrigger    — cron 全量测试：同一测试连续多轮失败
//
// Triggers are stateless sensors: dedup/cooldown is enforced by the
// orchestrator (per-action cooldown + DB pending unique index). Each
// suggestion carries TargetID "<trigger_source>/<signature>" and mirrors the
// signature into metadata.pattern_hash so per-signature pending dedup works
// through the generic unified-evolution path.
package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── Default thresholds (design D2; config block wires overrides in T1.11) ───

const (
	SIDefaultErrorClusterWindowDays  = 7
	SIDefaultErrorClusterMinCount    = 5
	SIDefaultPerfLatencyFactor       = 2.0
	SIDefaultPerfTokenFactor         = 1.5
	SIDefaultEvalRegressionThreshold = 0.10
	SIDefaultTestFailureRounds       = 2
	// SIDefaultTestFailureMaxPerTick caps suggestions per scan (storm guard).
	SIDefaultTestFailureMaxPerTick = 5
)

// ── Signal types (design §4.3) ───────────────────────────────────────────────

// ErrorCluster is one recurring error signature in the observation window.
type ErrorCluster struct {
	ErrorCode     string
	PatternHash   string // KB 聚类哈希（error_code + normalized stack）；可空
	Count         int
	Component     string // 受影响组件/包路径（若可得）
	SampleMessage string
	LastSeen      time.Time
}

// StepLatencyStat compares a step's P95 latency against its baseline.
type StepLatencyStat struct {
	StepID        string
	BaselineP95MS float64
	CurrentP95MS  float64
	SampleCount   int
}

// TokenAnomaly compares token usage of a scope (agent/step) against baseline.
type TokenAnomaly struct {
	Scope          string
	BaselineTokens float64
	CurrentTokens  float64
	SampleCount    int
}

// EvalBaseline is one completed evaluation run treated as a quality point.
type EvalBaseline struct {
	RunID     string
	DatasetID string
	AgentID   string
	Score     float64 // 综合分（0-1）
	CreatedAt time.Time
}

// TestFailure is one test failing across consecutive full-test rounds.
type TestFailure struct {
	Package           string
	TestName          string
	ConsecutiveRounds int
	LastError         string
	LastSeen          time.Time
}

// ── Signal source ports (design §4.3, Stability:evolving) ───────────────────

// ErrorClusterReader reads clustered recurring errors (KB 聚类读).
// Stability:evolving
type ErrorClusterReader interface {
	ListErrorClusters(ctx context.Context, since time.Time, minCount int) ([]ErrorCluster, error)
}

// PerfMetricsReader reads latency/token baselines vs current windows.
// Stability:evolving
type PerfMetricsReader interface {
	GetStepLatencyStats(ctx context.Context, baselineWindow, currentWindow string) ([]StepLatencyStat, error)
	GetTokenUsageStats(ctx context.Context, baselineWindow, currentWindow string) ([]TokenAnomaly, error)
}

// EvalBaselineReader reads the two most recent comparable eval baselines.
// Stability:evolving
type EvalBaselineReader interface {
	GetLatestBaseline(ctx context.Context) (*EvalBaseline, error)
	GetPreviousBaseline(ctx context.Context) (*EvalBaseline, error)
}

// TestRunReader reads tests failing across consecutive full-test rounds.
// Stability:evolving
type TestRunReader interface {
	ListRecentFailures(ctx context.Context, minConsecutiveRounds int) ([]TestFailure, error)
}

// ── shared suggestion builder ────────────────────────────────────────────────

// siSignature returns a deterministic short hash over the identity parts.
func siSignature(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return "si:" + hex.EncodeToString(h.Sum(nil))[:24]
}

// buildPlatformSuggestion assembles one pending platform suggestion. The
// signature keys TargetID and is mirrored into metadata.pattern_hash (the DB
// pending-dedup unique index reads that key).
func buildPlatformSuggestion(triggerSource string, actionType EvolutionActionType, signature string, reason string, priority int, evidence map[string]any) UnifiedEvolutionSuggestion {
	meta := map[string]any{
		EvoMetaTriggerSignature: signature,
		EvoMetaPatternHash:      signature,
	}
	for k, v := range evidence {
		meta[k] = v
	}
	raw, _ := json.Marshal(meta)
	return UnifiedEvolutionSuggestion{
		ID:              newAgentCatalogID(),
		TargetType:      EvolutionTargetPlatform,
		TargetID:        triggerSource + "/" + signature,
		ActionType:      actionType,
		TriggerSource:   triggerSource,
		TriggerReason:   reason,
		Status:          string(UnifiedEvolutionStatePending),
		Priority:        priority,
		LifecycleStatus: "draft",
		Metadata:        raw,
		CreatedAt:       time.Now().UTC(),
	}
}

// ── ErrorClusterTrigger ──────────────────────────────────────────────────────

// ErrorClusterTrigger fires on recurring error clusters (同 error_code+相似
// 堆栈窗口内 ≥minCount 次，design D2).
type ErrorClusterTrigger struct {
	reader     ErrorClusterReader
	windowDays int
	minCount   int
	lg         loggateway.Logger
}

// NewErrorClusterTrigger creates the trigger; windowDays/minCount <= 0 fall
// back to defaults (7d / 5).
func NewErrorClusterTrigger(reader ErrorClusterReader, windowDays, minCount int, lg loggateway.Logger) *ErrorClusterTrigger {
	if windowDays <= 0 {
		windowDays = SIDefaultErrorClusterWindowDays
	}
	if minCount <= 0 {
		minCount = SIDefaultErrorClusterMinCount
	}
	return &ErrorClusterTrigger{reader: reader, windowDays: windowDays, minCount: minCount, lg: lg}
}

// TargetType implements EvolutionTrigger.
func (t *ErrorClusterTrigger) TargetType() EvolutionTargetType { return EvolutionTargetPlatform }

// ActionType implements EvolutionTrigger.
func (t *ErrorClusterTrigger) ActionType() EvolutionActionType { return EvolutionActionPatchCode }

// TriggerSource implements EvolutionTrigger.
func (t *ErrorClusterTrigger) TriggerSource() string { return TriggerSourceErrorCluster }

// Check implements EvolutionTrigger.
func (t *ErrorClusterTrigger) Check(ctx context.Context, _ string) ([]UnifiedEvolutionSuggestion, error) {
	if t == nil || t.reader == nil {
		return nil, nil
	}
	since := time.Now().UTC().Add(-time.Duration(t.windowDays) * 24 * time.Hour)
	clusters, err := t.reader.ListErrorClusters(ctx, since, t.minCount)
	if err != nil {
		return nil, err
	}
	suggestions := make([]UnifiedEvolutionSuggestion, 0, len(clusters))
	for _, c := range clusters {
		sigSource := c.PatternHash
		if sigSource == "" {
			sigSource = c.ErrorCode // reader 未提供聚类哈希时按错误码兜底
		}
		sig := siSignature(TriggerSourceErrorCluster, sigSource)
		priority := 1
		if c.Count >= 2*t.minCount {
			priority = 2
		}
		reason := fmt.Sprintf("错误聚类 %q 在 %dd 内出现 %d 次（阈值 %d）", c.ErrorCode, t.windowDays, c.Count, t.minCount)
		suggestions = append(suggestions, buildPlatformSuggestion(
			TriggerSourceErrorCluster, EvolutionActionPatchCode, sig, reason, priority,
			map[string]any{
				"error_code":      c.ErrorCode,
				"kb_pattern_hash": c.PatternHash, // KB 聚类哈希；不得占用 EvoMetaPatternHash 键（签名镜像）
				"count":           c.Count,
				"window_days":     t.windowDays,
				"component":       c.Component,
				"sample_message":  c.SampleMessage,
				"last_seen":       c.LastSeen.Format(time.RFC3339Nano),
			},
		))
	}
	return suggestions, nil
}

// ── PerfBottleneckTrigger ────────────────────────────────────────────────────

// PerfBottleneckTrigger fires when a step's P95 latency or a scope's token
// usage exceeds its baseline by the configured factor (design D2). Latency
// signals map to patch_code; token anomalies map to tune_config.
type PerfBottleneckTrigger struct {
	reader        PerfMetricsReader
	latencyFactor float64
	tokenFactor   float64
	lg            loggateway.Logger
}

// NewPerfBottleneckTrigger creates the trigger; factors <= 0 fall back to
// defaults (2.0 / 1.5).
func NewPerfBottleneckTrigger(reader PerfMetricsReader, latencyFactor, tokenFactor float64, lg loggateway.Logger) *PerfBottleneckTrigger {
	if latencyFactor <= 0 {
		latencyFactor = SIDefaultPerfLatencyFactor
	}
	if tokenFactor <= 0 {
		tokenFactor = SIDefaultPerfTokenFactor
	}
	return &PerfBottleneckTrigger{reader: reader, latencyFactor: latencyFactor, tokenFactor: tokenFactor, lg: lg}
}

// TargetType implements EvolutionTrigger.
func (t *PerfBottleneckTrigger) TargetType() EvolutionTargetType { return EvolutionTargetPlatform }

// ActionType implements EvolutionTrigger (primary; suggestions may carry
// tune_config for token anomalies).
func (t *PerfBottleneckTrigger) ActionType() EvolutionActionType { return EvolutionActionPatchCode }

// TriggerSource implements EvolutionTrigger.
func (t *PerfBottleneckTrigger) TriggerSource() string { return TriggerSourcePerfBottleneck }

// Check implements EvolutionTrigger.
func (t *PerfBottleneckTrigger) Check(ctx context.Context, _ string) ([]UnifiedEvolutionSuggestion, error) {
	if t == nil || t.reader == nil {
		return nil, nil
	}
	var suggestions []UnifiedEvolutionSuggestion

	latency, err := t.reader.GetStepLatencyStats(ctx, "7d", "24h")
	if err != nil {
		t.lg.Warn("perf trigger: GetStepLatencyStats failed", loggateway.Err(err))
	}
	for _, s := range latency {
		if s.BaselineP95MS <= 0 || s.CurrentP95MS < s.BaselineP95MS*t.latencyFactor {
			continue
		}
		sig := siSignature(TriggerSourcePerfBottleneck, "latency", s.StepID)
		reason := fmt.Sprintf("步骤 %q P95 延迟 %.0fms 超基线 %.0fms 的 %.1f 倍", s.StepID, s.CurrentP95MS, s.BaselineP95MS, t.latencyFactor)
		suggestions = append(suggestions, buildPlatformSuggestion(
			TriggerSourcePerfBottleneck, EvolutionActionPatchCode, sig, reason, 2,
			map[string]any{
				"signal":         "latency",
				"step_id":        s.StepID,
				"baseline_p95ms": s.BaselineP95MS,
				"current_p95ms":  s.CurrentP95MS,
				"factor":         t.latencyFactor,
				"sample_count":   s.SampleCount,
			},
		))
	}

	tokens, err := t.reader.GetTokenUsageStats(ctx, "7d", "24h")
	if err != nil {
		t.lg.Warn("perf trigger: GetTokenUsageStats failed", loggateway.Err(err))
	}
	for _, a := range tokens {
		if a.BaselineTokens <= 0 || a.CurrentTokens < a.BaselineTokens*t.tokenFactor {
			continue
		}
		sig := siSignature(TriggerSourcePerfBottleneck, "token", a.Scope)
		reason := fmt.Sprintf("范围 %q token 用量 %.0f 超基线 %.0f 的 %.1f 倍", a.Scope, a.CurrentTokens, a.BaselineTokens, t.tokenFactor)
		suggestions = append(suggestions, buildPlatformSuggestion(
			TriggerSourcePerfBottleneck, EvolutionActionTuneConfig, sig, reason, 1,
			map[string]any{
				"signal":          "token",
				"scope":           a.Scope,
				"baseline_tokens": a.BaselineTokens,
				"current_tokens":  a.CurrentTokens,
				"factor":          t.tokenFactor,
				"sample_count":    a.SampleCount,
			},
		))
	}
	return suggestions, nil
}

// ── EvalRegressionTrigger ────────────────────────────────────────────────────

// EvalRegressionTrigger fires when the latest eval baseline score regresses
// beyond the threshold vs the previous baseline (design D2: 退化 >10%).
type EvalRegressionTrigger struct {
	reader    EvalBaselineReader
	threshold float64
	lg        loggateway.Logger
}

// NewEvalRegressionTrigger creates the trigger; threshold <= 0 falls back to
// the default (0.10).
func NewEvalRegressionTrigger(reader EvalBaselineReader, threshold float64, lg loggateway.Logger) *EvalRegressionTrigger {
	if threshold <= 0 {
		threshold = SIDefaultEvalRegressionThreshold
	}
	return &EvalRegressionTrigger{reader: reader, threshold: threshold, lg: lg}
}

// TargetType implements EvolutionTrigger.
func (t *EvalRegressionTrigger) TargetType() EvolutionTargetType { return EvolutionTargetPlatform }

// ActionType implements EvolutionTrigger.
func (t *EvalRegressionTrigger) ActionType() EvolutionActionType { return EvolutionActionPatchCode }

// TriggerSource implements EvolutionTrigger.
func (t *EvalRegressionTrigger) TriggerSource() string { return TriggerSourceEvalRegression }

// Check implements EvolutionTrigger.
func (t *EvalRegressionTrigger) Check(ctx context.Context, _ string) ([]UnifiedEvolutionSuggestion, error) {
	if t == nil || t.reader == nil {
		return nil, nil
	}
	latest, err := t.reader.GetLatestBaseline(ctx)
	if err != nil {
		return nil, err
	}
	previous, err := t.reader.GetPreviousBaseline(ctx)
	if err != nil {
		return nil, err
	}
	if latest == nil || previous == nil || previous.Score <= 0 {
		return nil, nil
	}
	// 退化判定：drop > threshold（严格大于，设计 D2）。浮点直接比较
	// latest < previous*(1-threshold) 在边界（如 0.80→0.72）受精度影响，
	// 改用 drop 比值 + epsilon 容差。
	drop := (previous.Score - latest.Score) / previous.Score
	if drop <= t.threshold+1e-9 {
		return nil, nil
	}
	sig := siSignature(TriggerSourceEvalRegression, latest.DatasetID, latest.AgentID)
	reason := fmt.Sprintf("评估基线综合分 %.2f → %.2f，退化 %.1f%%（阈值 %.0f%%）",
		previous.Score, latest.Score, drop*100, t.threshold*100)
	return []UnifiedEvolutionSuggestion{buildPlatformSuggestion(
		TriggerSourceEvalRegression, EvolutionActionPatchCode, sig, reason, 2,
		map[string]any{
			"dataset_id":      latest.DatasetID,
			"agent_id":        latest.AgentID,
			"latest_run_id":   latest.RunID,
			"previous_run_id": previous.RunID,
			"latest_score":    latest.Score,
			"previous_score":  previous.Score,
			"drop_ratio":      drop,
			"threshold":       t.threshold,
		},
	)}, nil
}

// ── TestFailureTrigger ───────────────────────────────────────────────────────

// TestFailureTrigger fires on tests failing across consecutive full-test
// rounds (design D2: 同一测试连续 2 轮失败). Suggestions are capped per scan
// (storm guard), most persistent failures first.
type TestFailureTrigger struct {
	reader     TestRunReader
	rounds     int
	maxPerTick int
	lg         loggateway.Logger
}

// NewTestFailureTrigger creates the trigger; rounds/maxPerTick <= 0 fall back
// to defaults (2 / 5).
func NewTestFailureTrigger(reader TestRunReader, rounds, maxPerTick int, lg loggateway.Logger) *TestFailureTrigger {
	if rounds <= 0 {
		rounds = SIDefaultTestFailureRounds
	}
	if maxPerTick <= 0 {
		maxPerTick = SIDefaultTestFailureMaxPerTick
	}
	return &TestFailureTrigger{reader: reader, rounds: rounds, maxPerTick: maxPerTick, lg: lg}
}

// TargetType implements EvolutionTrigger.
func (t *TestFailureTrigger) TargetType() EvolutionTargetType { return EvolutionTargetPlatform }

// ActionType implements EvolutionTrigger.
func (t *TestFailureTrigger) ActionType() EvolutionActionType { return EvolutionActionPatchCode }

// TriggerSource implements EvolutionTrigger.
func (t *TestFailureTrigger) TriggerSource() string { return TriggerSourceTestFailure }

// Check implements EvolutionTrigger.
func (t *TestFailureTrigger) Check(ctx context.Context, _ string) ([]UnifiedEvolutionSuggestion, error) {
	if t == nil || t.reader == nil {
		return nil, nil
	}
	failures, err := t.reader.ListRecentFailures(ctx, t.rounds)
	if err != nil {
		return nil, err
	}
	if len(failures) == 0 {
		return nil, nil
	}
	// 稳定排序：连续失败轮次降序，保证 cap 截取最持久的失败且多次扫描结果一致。
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].ConsecutiveRounds != failures[j].ConsecutiveRounds {
			return failures[i].ConsecutiveRounds > failures[j].ConsecutiveRounds
		}
		if failures[i].Package != failures[j].Package {
			return failures[i].Package < failures[j].Package
		}
		return failures[i].TestName < failures[j].TestName
	})
	if len(failures) > t.maxPerTick {
		failures = failures[:t.maxPerTick]
	}
	suggestions := make([]UnifiedEvolutionSuggestion, 0, len(failures))
	for _, f := range failures {
		sig := siSignature(TriggerSourceTestFailure, f.Package, f.TestName)
		priority := 1
		if f.ConsecutiveRounds > t.rounds {
			priority = 2
		}
		reason := fmt.Sprintf("测试 %s/%s 连续 %d 轮失败", f.Package, f.TestName, f.ConsecutiveRounds)
		suggestions = append(suggestions, buildPlatformSuggestion(
			TriggerSourceTestFailure, EvolutionActionPatchCode, sig, reason, priority,
			map[string]any{
				"package":            f.Package,
				"test_name":          f.TestName,
				"consecutive_rounds": f.ConsecutiveRounds,
				"last_error":         f.LastError,
				"last_seen":          f.LastSeen.Format(time.RFC3339Nano),
			},
		))
	}
	return suggestions, nil
}
