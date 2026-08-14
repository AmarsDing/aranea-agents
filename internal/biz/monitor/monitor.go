// Package monitor implements audit logging, monitor events, and alert evaluation workflows.
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// AuditLog is one admin audit row.
type AuditLog struct {
	ID           string
	Action       string
	Resource     string
	ResourceID   string
	RequestID    string
	Detail       string
	CreatedAt    string
	Actor        string
	IP           string
	UserAgent    string
	Severity     string
	MetadataJSON string
}

// AuditQuery filters audit log list.
type AuditQuery struct {
	Limit    int32
	Offset   int32
	Action   string
	Resource string
	Actor    string
	Keyword  string
	// ExcludeSystem hides system-generated noise (currently sync.* actions such
	// as skill filesystem sync) so user operations stay visible. Ignored when
	// Action is set explicitly (an explicit sync.* filter must still match).
	ExcludeSystem bool
}

// AuditListResult is a paginated audit log list.
type AuditListResult struct {
	Items []AuditLog
	Total int32
}

// PlatformRow is a generic monitor platform row.
type PlatformRow struct {
	Resource     string
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ParentID     string
	Level        string
	AgentID      string
	Provider     string
	Model        string
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
	// AgentName / TeamName are resolved display names joined from agents/teams
	// at the query layer (traces only); empty when the reference is dangling.
	AgentName string
	TeamName  string
	// SessionID / RunID are correlation keys for trace rows (traces only);
	// empty for event rows. Used by the detail dialog to query flow history.
	SessionID string
	RunID     string
}

// EventsQuery filters monitor events list.
type EventsQuery struct {
	Limit     int32
	Offset    int32
	EventType string
	AgentID   string
	Status    string
	SessionID string // filter by session_id in metadata
	TraceID   string // filter by trace_id in metadata
	// EventTypes: prefix match ANY (union with EventType).
	EventTypes []string
	// ExcludeEventTypes: prefix exclusion applied after the include set
	// (e.g. ["skill.filesystem."] to hide governance noise from the Events tab).
	ExcludeEventTypes []string
	// HideLinkedCompletions: exclude runner.completion rows already materialized
	// as Runs (usage_event_id set, or trace_id matching a monitor_traces row), so
	// server-side pagination total matches what the Events table renders.
	HideLinkedCompletions bool
}

// TracesQuery filters monitor traces list.
type TracesQuery struct {
	Limit    int32
	Offset   int32
	AgentID  string
	Provider string
	Model    string
	Status   string
	// Keyword: case-insensitive substring over name/trace_key/agent_id/provider/model.
	Keyword string
	// ExcludeInternal hides internal domains (system/skill: cron jobs, skill
	// watch sync, MCP health …) which fire constantly and drown user runs.
	ExcludeInternal bool
	// Domain filters one run domain exactly (chat/team/graph/system/skill);
	// empty = no domain filter (ExcludeInternal still applies).
	Domain string
}

type TraceWrite struct {
	TraceID       string
	SessionID     string
	RunID         string
	InvocationID  string
	AgentID       string
	Provider      string
	Model         string
	TeamID        string
	ParentTraceID string
	Name          string
	Status        string
	DurationMs    int64
	SpanCount     int
	ErrorCount    int
	TotalTokens   int64
	TotalCostUsd  float64
	MetadataJSON  string
}

type TraceSpanWrite struct {
	TraceID        string
	SpanID         string
	ParentSpanID   string
	Kind           string
	Name           string
	StartedAt      int64
	EndedAt        int64
	Status         string
	AttributesJSON string
	ErrorJSON      string
}

// TraceSpan is the read model for one persisted span row (monitor_trace_spans).
// Timestamps are Unix milliseconds; EndedAt may be 0 for a still-open span.
type TraceSpan struct {
	SpanID         string
	ParentSpanID   string
	Kind           string
	Name           string
	StartedAt      int64
	EndedAt        int64
	Status         string
	AttributesJSON string
	ErrorJSON      string
}

// ListResult is a paginated list of platform rows.
type ListResult struct {
	Items []PlatformRow
	Total int32
	// StatusCounts aggregates rows per status under the current filters
	// (keyword/domain applied, status condition excluded) — feeds filter chips.
	StatusCounts map[string]int32
	// DomainCounts aggregates rows per run domain under the current filters
	// (keyword/status applied, domain condition and ExcludeInternal excluded)
	// so chips can reveal how many internal rows are hidden by default.
	DomainCounts map[string]int32
}

// EventWrite is the insert payload for a monitor event.
type EventWrite struct {
	EventKey     string
	Name         string
	Description  string
	Status       string
	MetadataJSON string
}

// AlertFiringState is the alert state machine value (MON-OPT-02).
type AlertFiringState string

const (
	AlertFiringStateIdle      AlertFiringState = "idle"
	AlertFiringStateFiring    AlertFiringState = "firing"
	AlertFiringStateRecovered AlertFiringState = "recovered"
)

// defaultRecoveryFactor is the fraction of the threshold below which a firing alert is considered recovered.
const defaultRecoveryFactor = 0.9

// AlertRule defines a simple threshold alert on monitor_events aggregates.
type AlertRule struct {
	ID               string
	Name             string
	MetricKey        string
	Threshold        float64
	WindowMinutes    int
	Enabled          bool
	Severity         string
	NotifyWebhookURL string
	NotifyChannelID  string
	CooldownMinutes  int
	ReminderMinutes  int // while firing: re-notify interval; default 30
	CreatedAt        string
	UpdatedAt        string

	// MON-OPT-02: persistent firing state machine.
	FiringState          AlertFiringState // idle | firing | recovered
	LastFiredAt          *time.Time       // unix ms persisted in DB
	LastFiredValue       float64          // metric value at last fire
	LastFiredWindowStart *time.Time
	RecoveredAt          *time.Time
	RecoveryFactor       float64 // 0.9 default: metric must drop below Threshold×RecoveryFactor to recover
}

// AlertNotifier delivers fired alerts to external channels.
type AlertNotifier interface {
	Notify(ctx context.Context, rule AlertRule, payload map[string]any)
}

// RunnerCompletionRow represents a runner completion record.
type RunnerCompletionRow struct {
	TraceID    string
	SessionID  string
	RunID      string
	AgentID    string
	Status     string
	DurationMs int64
	CreatedAt  string
}

// AuditRepo handles audit log persistence.
type AuditRepo interface {
	ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error)
	InsertAuditLog(ctx context.Context, entry AuditLog) error
	// DeleteAuditLogs hard-deletes all audit log rows and returns the deleted count.
	DeleteAuditLogs(ctx context.Context) (int, error)
}

// EventRepo handles monitor event persistence and queries.
type EventRepo interface {
	InsertMonitorEvent(ctx context.Context, ev EventWrite) error
	ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error)
	GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error)
	CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error)
	// DeleteMonitorEventsOlderThan hard-deletes rows created before olderThan.
	// Safe for retention: alert windows / runner metrics aggregate over
	// minutes-hours, and Runs use monitor_traces as truth source (OPT-05).
	DeleteMonitorEventsOlderThan(ctx context.Context, olderThan time.Time) (int, error)
}

// UsageAggregate holds token/cost aggregates for one trace, computed from
// model_token_usage_events (the authoritative cost source).
type UsageAggregate struct {
	TotalTokens  int64
	TotalCostUsd float64
	Provider     string
	Model        string
	CallCount    int
}

// TraceCompletion carries the terminal-state fields written when a run
// completes (or is backfilled). Provider/Model are backfilled only when the
// stored column is still empty.
type TraceCompletion struct {
	Status       string
	DurationMs   int64
	SpanCount    int
	ErrorCount   int
	TotalTokens  int64
	TotalCostUsd float64
	Provider     string
	Model        string
}

// TraceUsageRepo aggregates token usage events for a single trace.
// Stability:evolving
type TraceUsageRepo interface {
	AggregateUsageByTrace(ctx context.Context, traceID string) (UsageAggregate, error)
}

// TraceSpanReader reads persisted spans for a single trace, ordered by start time.
// Stability:evolving
type TraceSpanReader interface {
	ListMonitorTraceSpans(ctx context.Context, traceID string) ([]TraceSpan, error)
}

// TraceRepo handles monitor trace persistence and queries.
type TraceRepo interface {
	ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error)
	GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error)
	InsertMonitorTrace(ctx context.Context, tw TraceWrite) error
	UpsertMonitorTraceSpan(ctx context.Context, sw TraceSpanWrite) error
	UpdateMonitorTraceCompletion(ctx context.Context, traceID string, c TraceCompletion) error
	// InterruptStaleTraces sweeps traces stuck in "running" (process crashed
	// before completion) to "interrupted". Traces with span activity inside
	// the TTL window are kept (still alive). Returns affected row count.
	InterruptStaleTraces(ctx context.Context, olderThan time.Time) (int64, error)
	EnsureTraceSchema(ctx context.Context) error
}

// AlertRepo handles alert rule persistence and state.
type AlertRepo interface {
	ListAlertRules(ctx context.Context) ([]AlertRule, error)
	ReplaceAlertRules(ctx context.Context, rules []AlertRule) error
	UpdateAlertFiringState(ctx context.Context, id string, state AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error
}

// RunnerCompletionRepo handles runner completion persistence and queries.
type RunnerCompletionRepo interface {
	ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error)
	PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error)
	AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error)
	LatencyPercentilesSince(ctx context.Context, sinceRFC3339 string) (p50, p95, p99 float64, err error)
	ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]RunnerCompletionRow, error)
}

// RunnerMetricsSummary aggregates runner.completion monitor events.
type RunnerMetricsSummary struct {
	WindowMinutes int
	TotalRuns     int32
	ErrorRuns     int32
	ErrorRate     float64
	SuccessRate   float64
	AvgDurationMs float64
	P50DurationMs float64
	P95DurationMs float64
	P99DurationMs float64
}

// FilesystemHealthReader supplies live skill filesystem health for alerts.
type FilesystemHealthReader interface {
	FilesystemHealthStats(ctx context.Context) (missingCount int, pendingCount int, err error)
}

// Usecase orchestrates monitoring sub-domains.
// TODO(debt): DEV-05 — Split into sub-packages by domain:
//   - audit/   (AuditLog, AuditRecord)
//   - trace/   (TraceProjector, FlowLogUtils)
//   - alert/   (AlertEvalWorker, MetricRingBuffer, AlertMetricRegistry)
//   - heal/    (SelfHealUsecase, SelfHealObserver, PredictiveHealUsecase)
//   - rca/     (RootCauseEngine, RootCauseAnalyzer)
//   - pattern/ (PatternMiningUsecase, FailurePatternRepo)
type Usecase struct {
	// --- 数据访问端口 ---
	auditRepo        AuditRepo
	eventRepo        EventRepo
	traceRepo        TraceRepo
	alertRepo        AlertRepo
	runnerCompletion RunnerCompletionRepo
	traceSpanReader  TraceSpanReader

	// --- 告警域（alert） ---
	notifier   AlertNotifier
	registry   *AlertMetricRegistry
	evalWorker *AlertEvalWorker // 循环依赖（worker 持有 uc）：唯一保留的 setter 注入
	rulesCache alertRulesCache

	// --- trace / 日志落盘域 ---
	traceProjector   *TraceProjector
	flowFileAppender *FlowFileAppender

	// --- 横切 ---
	lg      loggateway.Logger
	flowLog FlowLogWriter
}

const rulesCacheTTL = 5 * time.Minute

// alertRulesCache bundles the in-memory alert-rule cache state (singleflight
// against the alert repo; invalidated on every rule/state mutation).
type alertRulesCache struct {
	mu     sync.RWMutex
	rules  []AlertRule
	expire time.Time
}

func (c *alertRulesCache) get() []AlertRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.rules != nil && time.Now().Before(c.expire) {
		return c.rules
	}
	return nil
}

func (c *alertRulesCache) set(rules []AlertRule) {
	c.mu.Lock()
	c.rules = rules
	c.expire = time.Now().Add(rulesCacheTTL)
	c.mu.Unlock()
}

func (c *alertRulesCache) invalidate() {
	c.mu.Lock()
	c.rules = nil
	c.expire = time.Time{}
	c.mu.Unlock()
}

type UsecaseOption func(*Usecase)

// WithTraceSpanReader wires the span read path used by GetMonitorTrace details.
func WithTraceSpanReader(r TraceSpanReader) UsecaseOption {
	return func(u *Usecase) { u.traceSpanReader = r }
}

func WithEvalWorker(w *AlertEvalWorker) UsecaseOption {
	return func(u *Usecase) { u.evalWorker = w }
}

func WithRegistry(r *AlertMetricRegistry) UsecaseOption {
	return func(u *Usecase) { u.registry = r }
}

func WithLogger(lg loggateway.Logger) UsecaseOption {
	return func(u *Usecase) { u.lg = lg }
}

// WithFlowLogWriter wires the user-visible flow log (流程日志) port. Nil-safe:
// when unset, flow log emission is skipped.
func WithFlowLogWriter(fl FlowLogWriter) UsecaseOption {
	return func(u *Usecase) { u.flowLog = fl }
}

// WithTraceProjector wires the completion → trace close side-effect.
func WithTraceProjector(p *TraceProjector) UsecaseOption {
	return func(u *Usecase) { u.traceProjector = p }
}

// WithFlowFileAppender wires the TRACE-01 trace file sink.
func WithFlowFileAppender(a *FlowFileAppender) UsecaseOption {
	return func(u *Usecase) { u.flowFileAppender = a }
}

func NewUsecase(audit AuditRepo, event EventRepo, trace TraceRepo, alert AlertRepo, runner RunnerCompletionRepo, notifier AlertNotifier, opts ...UsecaseOption) *Usecase {
	uc := &Usecase{auditRepo: audit, eventRepo: event, traceRepo: trace, alertRepo: alert, runnerCompletion: runner, notifier: notifier}
	for _, opt := range opts {
		opt(uc)
	}
	if uc.lg == nil {
		uc.lg = loggateway.NewNoop()
	}
	return uc
}

// SetEvalWorker is the ONLY remaining setter: AlertEvalWorker holds a back
// reference to the Usecase (circular dependency), so it cannot be a
// construction option. All other dependencies use UsecaseOption.
func (u *Usecase) SetEvalWorker(w *AlertEvalWorker) {
	if u != nil {
		u.evalWorker = w
	}
}

func (u *Usecase) EvalWorker() *AlertEvalWorker {
	if u == nil {
		return nil
	}
	return u.evalWorker
}

func (u *Usecase) Registry() *AlertMetricRegistry {
	if u == nil {
		return nil
	}
	return u.registry
}

// RecordAuditLog persists an admin audit row (best-effort, logs on failure).
func (u *Usecase) RecordAuditLog(ctx context.Context, entry AuditLog) error {
	if u == nil || u.auditRepo == nil {
		return nil
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = uuid.NewString()
	}
	if err := u.auditRepo.InsertAuditLog(ctx, entry); err != nil {
		u.lg.Warn("RecordAuditLog failed", loggateway.StepID("monitor.audit_log_fail"), loggateway.Str("action", entry.Action), loggateway.Str("resource_id", entry.ResourceID), loggateway.Err(err))
		return err
	}
	return nil
}

// RecordMonitorEvent persists a monitor_events row (best-effort, logs on failure).
func (u *Usecase) RecordMonitorEvent(ctx context.Context, ev EventWrite) error {
	if u == nil || u.eventRepo == nil {
		return nil
	}
	if err := u.eventRepo.InsertMonitorEvent(ctx, ev); err != nil {
		u.lg.Warn("RecordMonitorEvent failed", loggateway.StepID("monitor.event_persist_fail"), loggateway.Str("event_key", ev.EventKey), loggateway.Err(err))
		return err
	}
	return nil
}

// ListAuditLogs returns paginated audit logs.
func (u *Usecase) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	return u.auditRepo.ListAuditLogs(ctx, query)
}

// DeleteAuditLogs hard-deletes all audit logs and returns the deleted count.
// DeleteAuditLogs clears all audit logs, then writes a self-audit entry so
// the destructive operation itself is traceable (who/when/how many).
func (u *Usecase) DeleteAuditLogs(ctx context.Context) (int, error) {
	deleted, err := u.auditRepo.DeleteAuditLogs(ctx)
	if err != nil {
		return deleted, err
	}
	// Self-audit: record the clear operation itself as a fresh audit entry.
	u.RecordAuditLog(ctx, AuditLog{
		Action:   "delete.audit_logs",
		Resource: "audit",
		Detail:   fmt.Sprintf(`{"deleted":%d}`, deleted),
		Severity: "warning",
	})
	return deleted, nil
}

// ListMonitorEvents returns paginated monitor events.
func (u *Usecase) ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.eventRepo.ListMonitorEvents(ctx, query)
}

// ListAlertRulesWithDefaults returns alert rules, creating defaults if none exist.
func (u *Usecase) ListAlertRulesWithDefaults(ctx context.Context) ([]AlertRule, error) {
	rules, err := u.ListAlertRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		defaults := DefaultAlertRules()
		if err := u.ReplaceAlertRules(ctx, defaults); err != nil {
			u.lg.Warn("ListAlertRulesWithDefaults: ReplaceAlertRules failed",
				loggateway.StepID("monitor.alert_rules_replace_fail"),
				loggateway.Err(err),
			)
		}
		rules = defaults
	}
	return rules, nil
}

// DefaultAlertRules returns the built-in default alert rules.
func DefaultAlertRules() []AlertRule {
	return []AlertRule{
		{
			ID: "default-runner-errors", Name: "Runner error rate",
			MetricKey: "runner.error_rate", Threshold: 0.25, WindowMinutes: 60, Enabled: true, Severity: "warning",
		},
		{
			// P0-R2a: any dead-lettered event means durable persist loss.
			ID: "default-sequencer-dead-letter", Name: "Sequencer dead-letter backlog",
			MetricKey: "sequencer.dead_letter_count", Threshold: 1, WindowMinutes: 5, Enabled: true, Severity: "critical",
		},
		{
			// 29-token §9.4 (G1-B): low prompt-cache hit ratio means prefix bust.
			ID: "default-llm-cache-hit-ratio-low", Name: "LLM cache hit ratio low",
			MetricKey: "llm.cache_hit_ratio_low", Threshold: 1, WindowMinutes: 60, Enabled: true, Severity: "warning",
		},
	}
}

// ListAlertRules returns all alert rules.
func (u *Usecase) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	if u == nil || u.alertRepo == nil {
		return nil, nil
	}
	return u.alertRepo.ListAlertRules(ctx)
}

// validateAlertRule 边界校验：名称/指标必填、阈值与统计窗口为正（前端另有 saveDisabled 门禁，双保险）。
func validateAlertRule(r AlertRule) error {
	if strings.TrimSpace(r.Name) == "" {
		return apierror.BadRequest(apierror.DomainMonitor, "alert rule name is required")
	}
	if strings.TrimSpace(r.MetricKey) == "" {
		return apierror.BadRequest(apierror.DomainMonitor, "alert rule metric_key is required")
	}
	if r.Threshold <= 0 {
		return apierror.BadRequest(apierror.DomainMonitor, "alert rule threshold must be greater than 0")
	}
	if r.WindowMinutes <= 0 {
		return apierror.BadRequest(apierror.DomainMonitor, "alert rule window_minutes must be greater than 0")
	}
	return nil
}

// ReplaceAlertRules replaces all alert rules.
func (u *Usecase) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	if u == nil || u.alertRepo == nil {
		return nil
	}
	for _, r := range rules {
		if err := validateAlertRule(r); err != nil {
			return err
		}
	}

	if err := u.alertRepo.ReplaceAlertRules(ctx, rules); err != nil {
		return err
	}

	// Invalidate rules cache
	u.rulesCache.invalidate()
	return nil
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (u *Usecase) EvaluateAlerts(ctx context.Context) {
	if u == nil || u.alertRepo == nil {
		return
	}
	rules := u.cachedAlertRules(ctx)
	ruleCount := 0
	triggeredCount := 0
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		ruleCount++
		metricKey := strings.TrimSpace(rule.MetricKey)
		if u.registry == nil {
			// Legacy dual-track switch removed (S1): evaluation only runs via
			// the metric registry. Nil registry means no metrics can be
			// evaluated — wire always provides one in production.
			u.lg.Debug("EvaluateAlerts: no metric registry, skipping rule",
				loggateway.StepID("monitor.alert_eval_no_registry"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey))
			continue
		}
		m, ok := u.registry.Get(metricKey)
		if !ok {
			continue
		}
		window := time.Duration(rule.WindowMinutes) * time.Minute
		if window <= 0 {
			window = 60 * time.Minute
		}
		value, err := m.Evaluate(ctx, window)
		if errors.Is(err, ErrAlertMetricNoData) {
			// Empty window: no evidence for any state transition.
			// Skip silently so a firing alert is not falsely recovered.
			u.lg.Debug("EvaluateAlerts: no metric data in window, skipping",
				loggateway.StepID("monitor.alert_eval_no_data"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey))
			continue
		}
		if err != nil {
			u.lg.Warn("EvaluateAlerts: metric evaluation failed",
				loggateway.StepID("monitor.alert_eval_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey), loggateway.Err(err))
			continue
		}
		if u.evaluateMetricValue(ctx, rule, value, m) {
			triggeredCount++
		}
	}
	if u.flowLog != nil {
		u.flowLog.LogFlowDone(ctx, "", "monitor.alert.evaluate", "告警评估完成",
			LogPair{Key: "rule_count", Value: ruleCount},
			LogPair{Key: "triggered_count", Value: triggeredCount})
	}
}

// evaluateMetricValue applies the alert state machine to one metric sample.
// metric may be nil; when it implements AlertBreachDetailer the breach
// summary/details of the most recent Evaluate call are merged into
// alert.fired event metadata and notifier payloads.
// Returns true when the rule newly fired (idle/recovered → firing) this call.
func (u *Usecase) evaluateMetricValue(ctx context.Context, rule AlertRule, value float64, metric AlertMetric) bool {
	now := time.Now().UTC()
	breachSummary, breachPayload := breachDetailsOf(metric)

	// Auto-transition recovered → idle after cooldown expires and metric stays below threshold
	if rule.FiringState == AlertFiringStateRecovered && value < rule.Threshold {
		if rule.RecoveredAt != nil {
			cooldown := rule.CooldownMinutes
			if cooldown <= 0 {
				cooldown = 60
			}
			if now.Sub(*rule.RecoveredAt) >= time.Duration(cooldown)*time.Minute {
				u.MarkAlertReset(ctx, rule)
			}
		}
	}

	if rule.FiringState == AlertFiringStateFiring && value < recoveryThreshold(rule) {
		u.MarkAlertRecovered(ctx, rule, now)
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "recovery_threshold": recoveryThreshold(rule),
		})
		if err := u.RecordMonitorEvent(ctx, EventWrite{
			EventKey: "alert.recovered", Name: rule.Name,
			Description: fmt.Sprintf("%s %.2f recovered below %.2f", rule.MetricKey, value, recoveryThreshold(rule)),
			Status:      "recovered", MetadataJSON: string(meta),
		}); err != nil {
			u.lg.Warn("RecordMonitorEvent for alert.recovered failed",
				loggateway.StepID("monitor.alert_recovered_persist_fail"),
				loggateway.Str("rule_id", rule.ID),
				loggateway.Err(err))
		}
		if u.notifier != nil {
			u.notifier.Notify(ctx, rule, map[string]any{
				"rule_id": rule.ID, "name": rule.Name, "metric_key": rule.MetricKey,
				"value": value, "recovered_at": now.Format(time.RFC3339),
			})
		}
		return false
	}
	if value < rule.Threshold {
		return false
	}
	// Already firing: only send periodic reminders; never re-enter threshold_exceeded transition.
	if rule.FiringState == AlertFiringStateFiring {
		if !u.shouldRemindAlert(rule, now) {
			return false
		}
		u.touchAlertReminder(ctx, rule, now, value)
		metaMap := map[string]any{
			"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "threshold": rule.Threshold, "reminder": true,
		}
		for k, v := range breachPayload {
			metaMap[k] = v
		}
		meta, _ := json.Marshal(metaMap)
		if err := u.RecordMonitorEvent(ctx, EventWrite{
			EventKey: "alert.fired", Name: rule.Name,
			Description: appendBreachSummary(fmt.Sprintf("%s %.2f >= %.2f (reminder)", rule.MetricKey, value, rule.Threshold), breachSummary),
			Status:      strings.TrimSpace(rule.Severity), MetadataJSON: string(meta),
		}); err != nil {
			u.lg.Warn("RecordMonitorEvent for alert.fired reminder failed",
				loggateway.StepID("monitor.alert_fired_persist_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
		}
		if u.notifier != nil {
			u.notifier.Notify(ctx, rule, map[string]any{
				"rule_id": rule.ID, "name": rule.Name, "metric_key": rule.MetricKey,
				"value": value, "threshold": rule.Threshold, "reminder": true,
				"severity": strings.TrimSpace(rule.Severity), "fired_at": now.Format(time.RFC3339),
			})
		}
		return false
	}
	if !u.ShouldFireAlert(rule, now) {
		return false
	}
	u.MarkAlertFiredPersistent(ctx, rule, now, value)
	metaMap := map[string]any{
		"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "threshold": rule.Threshold,
	}
	for k, v := range breachPayload {
		metaMap[k] = v
	}
	meta, _ := json.Marshal(metaMap)
	if err := u.RecordMonitorEvent(ctx, EventWrite{
		EventKey: "alert.fired", Name: rule.Name,
		Description: appendBreachSummary(fmt.Sprintf("%s %.2f >= %.2f", rule.MetricKey, value, rule.Threshold), breachSummary),
		Status:      strings.TrimSpace(rule.Severity), MetadataJSON: string(meta),
	}); err != nil {
		u.lg.Warn("RecordMonitorEvent for alert.fired failed",
			loggateway.StepID("monitor.alert_fired_persist_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	payload := map[string]any{
		"rule_id": rule.ID, "name": rule.Name, "metric_key": rule.MetricKey,
		"value": value, "threshold": rule.Threshold,
		"severity": strings.TrimSpace(rule.Severity), "fired_at": now.Format(time.RFC3339),
	}
	for k, v := range breachPayload {
		payload[k] = v
	}
	if breachSummary != "" {
		payload["breach_summary"] = breachSummary
	}
	if u.notifier != nil {
		u.notifier.Notify(ctx, rule, payload)
	}
	return true
}

func (u *Usecase) cachedAlertRules(ctx context.Context) []AlertRule {
	if rules := u.rulesCache.get(); rules != nil {
		return rules
	}

	rules, err := u.alertRepo.ListAlertRules(ctx)
	if err != nil {
		u.lg.Warn("cachedAlertRules: ListAlertRules failed", loggateway.StepID("monitor.alert_rules_load_fail"), loggateway.Err(err))
		return nil
	}

	u.rulesCache.set(rules)
	return rules
}

// ShouldFireAlert checks whether an alert rule should fire now.
//
// MON-OPT-02: Cooldown is evaluated against DB-persisted LastFiredAt (loaded via
// ListAlertRules cache). This survives process restarts and prevents duplicate fires
// across replicas when SQLite is used (single-writer).
func (u *Usecase) ShouldFireAlert(rule AlertRule, now time.Time) bool {
	if u == nil {
		return false
	}
	cooldown := rule.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 60
	}
	cooldownDur := time.Duration(cooldown) * time.Minute

	// DB-persisted path (MON-OPT-02): use rule.LastFiredAt if available.
	if rule.LastFiredAt != nil && now.Sub(*rule.LastFiredAt) < cooldownDur {
		return false
	}
	return true
}

const defaultReminderMinutes = 30

func reminderInterval(rule AlertRule) time.Duration {
	mins := rule.ReminderMinutes
	if mins <= 0 {
		mins = defaultReminderMinutes
	}
	return time.Duration(mins) * time.Minute
}

func (u *Usecase) shouldRemindAlert(rule AlertRule, now time.Time) bool {
	if rule.LastFiredAt == nil {
		return true
	}
	return now.Sub(*rule.LastFiredAt) >= reminderInterval(rule)
}

// touchAlertReminder refreshes LastFiredAt while staying in firing state (reminder path).
func (u *Usecase) touchAlertReminder(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if u == nil || u.alertRepo == nil {
		return
	}
	if err := u.alertRepo.UpdateAlertFiringState(ctx, rule.ID, AlertFiringStateFiring, &now, metricValue, nil); err != nil {
		u.lg.Warn("touchAlertReminder: DB update failed",
			loggateway.StepID("monitor.alert_reminder_db_fail"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Err(err))
	}
	u.rulesCache.invalidate()
}

// MarkAlertFiredPersistent is the DB-backed mark called after the alert.fired
// event is published (MON-OPT-02). It also advances the state machine from
// idle/recovered → firing.
func (u *Usecase) MarkAlertFiredPersistent(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if u == nil || u.alertRepo == nil {
		return
	}
	// Validate state machine transition: current → firing
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventThresholdExceeded)
	if err != nil {
		u.lg.Warn("MarkAlertFiredPersistent: invalid state transition",
			loggateway.StepID("monitor.mark_fired_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	u.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := u.alertRepo.UpdateAlertFiringState(ctx, rule.ID, next, &now, metricValue, nil); err != nil {
		u.lg.Warn("MarkAlertFiredPersistent: DB update failed", loggateway.StepID("monitor.mark_fired_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	// Invalidate rules cache so next evaluation round reads fresh DB state.
	u.rulesCache.invalidate()
}

// MarkAlertRecovered transitions a firing alert to recovered and persists it (MON-OPT-02).
func (u *Usecase) MarkAlertRecovered(ctx context.Context, rule AlertRule, now time.Time) {
	if u == nil || u.alertRepo == nil {
		return
	}
	// Validate state machine transition: current → recovered
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventRecovered)
	if err != nil {
		u.lg.Warn("MarkAlertRecovered: invalid state transition",
			loggateway.StepID("monitor.mark_recovered_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	u.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := u.alertRepo.UpdateAlertFiringState(ctx, rule.ID, next, rule.LastFiredAt, rule.LastFiredValue, &now); err != nil {
		u.lg.Warn("MarkAlertRecovered: DB update failed", loggateway.StepID("monitor.mark_recovered_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	u.rulesCache.invalidate()
}

// MarkAlertReset transitions a recovered alert back to idle after cooldown expires.
func (u *Usecase) MarkAlertReset(ctx context.Context, rule AlertRule) {
	if u == nil || u.alertRepo == nil {
		return
	}
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventReset)
	if err != nil {
		u.lg.Warn("MarkAlertReset: invalid state transition",
			loggateway.StepID("monitor.mark_reset_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	u.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := u.alertRepo.UpdateAlertFiringState(ctx, rule.ID, next, nil, 0, nil); err != nil {
		u.lg.Warn("MarkAlertReset: DB update failed", loggateway.StepID("monitor.mark_reset_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	u.rulesCache.invalidate()
}

// recoveryThreshold returns the value below which a firing alert is considered recovered.
func recoveryThreshold(rule AlertRule) float64 {
	f := rule.RecoveryFactor
	if f <= 0 || f > 1 {
		f = defaultRecoveryFactor
	}
	return rule.Threshold * f
}

// GetMonitorEvent returns one monitor event by ID.
func (u *Usecase) GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error) {
	return u.eventRepo.GetMonitorEvent(ctx, id)
}

// ListMonitorTraces returns paginated monitor traces.
func (u *Usecase) ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.traceRepo.ListMonitorTraces(ctx, query)
}

// GetMonitorTrace returns one monitor trace by ID.
func (u *Usecase) GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error) {
	return u.traceRepo.GetMonitorTrace(ctx, id)
}

// ListTraceSpans returns persisted spans for a trace, ordered by start time.
// Nil-safe: an unwired reader yields an empty slice so callers can fall back
// to legacy config_json spans.
func (u *Usecase) ListTraceSpans(ctx context.Context, traceID string) ([]TraceSpan, error) {
	if u == nil || u.traceSpanReader == nil || strings.TrimSpace(traceID) == "" {
		return nil, nil
	}
	return u.traceSpanReader.ListMonitorTraceSpans(ctx, traceID)
}

// GetRunnerMetrics aggregates runner.completion monitor events.
func (u *Usecase) GetRunnerMetrics(ctx context.Context, windowMinutes int) (RunnerMetricsSummary, error) {
	out := RunnerMetricsSummary{WindowMinutes: windowMinutes}
	if u == nil || u.eventRepo == nil {
		return out, nil
	}
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	out.WindowMinutes = windowMinutes
	since := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute).Format(time.RFC3339)
	total, err := u.eventRepo.CountMonitorEventsSince(ctx, "runner.completion", "", since, "")
	if err != nil {
		return out, err
	}
	errors, err := u.eventRepo.CountMonitorEventsSince(ctx, "runner.completion", "error", since, "")
	if err != nil {
		return out, err
	}
	out.TotalRuns = total
	out.ErrorRuns = errors
	if total > 0 {
		out.ErrorRate = float64(errors) / float64(total)
		out.SuccessRate = 1 - out.ErrorRate
	}
	if avg, err := u.runnerCompletion.AvgRunnerCompletionDurationMsSince(ctx, since); err == nil {
		out.AvgDurationMs = avg
	} else {
		u.lg.Warn("runner_metrics.avg_duration_failed", loggateway.Err(err))
	}
	if p50, p95, p99, err := u.runnerCompletion.LatencyPercentilesSince(ctx, since); err == nil {
		out.P50DurationMs = p50
		out.P95DurationMs = p95
		out.P99DurationMs = p99
	} else {
		u.lg.Warn("runner_metrics.percentiles_failed", loggateway.Err(err))
	}
	return out, nil
}

// RunnerCompletionLinkParams encapsulates the parameters for linking runner
// completion rows with usage rows. Introduced to keep method signatures under
// the 5-parameter limit (S4/S5 fix). Bridge may be nil for LinkRunnerCompletionUsage
// (the method returns early in that case).
type RunnerCompletionLinkParams struct {
	SessionID    string
	RunID        string
	InvocationID string
	UsageEventID string
	TraceID      string
	Status       string
	DurationMs   int64
	Bridge       RunnerCompletionBridge
}

// RecordRunnerCompletion persists a runner.completion event and patches metadata.
func (u *Usecase) RecordRunnerCompletion(ctx context.Context, write EventWrite, p RunnerCompletionLinkParams) error {
	if u == nil || u.eventRepo == nil {
		return nil
	}
	status := strings.TrimSpace(p.Status)
	if status == "" {
		status = strings.TrimSpace(write.Status)
	}
	if status == "" {
		status = "ok"
	}
	if p.SessionID != "" && p.InvocationID != "" {
		exists, err := u.runnerCompletion.ExistsRunnerCompletion(ctx, p.SessionID, p.InvocationID)
		if err != nil {
			return err
		}
		if exists {
			patched, err := u.PatchRunnerCompletionLink(ctx, p)
			if err != nil {
				return err
			}
			if patched || strings.TrimSpace(p.UsageEventID) != "" {
				p.Bridge.ClearTurn(p.SessionID, p.RunID)
			}
			u.notifyCompletionSideEffects(ctx, status, p.DurationMs, p.TraceID)
			return nil
		}
	}
	if err := u.eventRepo.InsertMonitorEvent(ctx, write); err != nil {
		return err
	}
	patched, err := u.PatchRunnerCompletionLink(ctx, p)
	if err != nil {
		return err
	}
	if patched || strings.TrimSpace(p.UsageEventID) != "" {
		p.Bridge.ClearTurn(p.SessionID, p.RunID)
	}
	u.notifyCompletionSideEffects(ctx, status, p.DurationMs, p.TraceID)
	return nil
}

// notifyCompletionSideEffects feeds live alert metrics, closes traces, and writes TRACE-01 files.
func (u *Usecase) notifyCompletionSideEffects(ctx context.Context, status string, durationMs int64, traceID string) {
	if u == nil {
		return
	}
	if u.evalWorker != nil {
		u.evalWorker.OnCompletion(status, durationMs)
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return
	}
	if u.traceProjector != nil {
		u.traceProjector.OnRunnerCompletion(ctx, traceID, status, durationMs)
	}
	if u.flowFileAppender != nil {
		u.flowFileAppender.WriteTraceComplete(map[string]any{
			"schema_version": "trace_complete/v1",
			"trace_id":       traceID,
			"status":         status,
			"duration_ms":    durationMs,
		})
	}
}

// LinkRunnerCompletionUsage patches the latest completion row with usage_event_id.
func (u *Usecase) LinkRunnerCompletionUsage(ctx context.Context, p RunnerCompletionLinkParams) error {
	if u == nil || u.runnerCompletion == nil {
		return nil
	}
	p.SessionID = strings.TrimSpace(p.SessionID)
	p.RunID = strings.TrimSpace(p.RunID)
	p.UsageEventID = strings.TrimSpace(p.UsageEventID)
	if p.SessionID == "" || p.RunID == "" || p.UsageEventID == "" {
		return nil
	}
	p.Bridge.RegisterTurnUsage(p.SessionID, p.RunID, p.UsageEventID, p.TraceID, "", "")
	p.InvocationID = p.RunID
	patched, err := u.PatchRunnerCompletionLink(ctx, p)
	if err != nil {
		return err
	}
	if patched {
		p.Bridge.ClearTurn(p.SessionID, p.RunID)
	}
	return nil
}

// PatchRunnerCompletionLink patches runner completion metadata with usage correlation.
func (u *Usecase) PatchRunnerCompletionLink(ctx context.Context, p RunnerCompletionLinkParams) (bool, error) {
	p.UsageEventID = strings.TrimSpace(p.UsageEventID)
	p.TraceID = strings.TrimSpace(p.TraceID)
	if p.UsageEventID == "" {
		if u2, t2, ok := p.Bridge.PendingUsage(p.SessionID, p.RunID); ok {
			p.UsageEventID = u2
			if p.TraceID == "" {
				p.TraceID = t2
			}
		}
	}
	if p.UsageEventID == "" {
		return false, nil
	}
	patch := MergeRunnerCompletionUsagePatch(p.UsageEventID, p.TraceID)
	return u.runnerCompletion.PatchRunnerCompletionMetadata(ctx, p.SessionID, p.RunID, p.InvocationID, patch)
}

// RunnerCompletionBridge links runner.completion rows with usage rows.
type RunnerCompletionBridge interface {
	RegisterTurnUsage(sessionID, runID, usageEventID, traceID, agentID, agentKey string)
	PendingUsage(sessionID, runID string) (usageEventID, traceID string, ok bool)
	ClearTurn(sessionID, runID string)
}

// MergeRunnerCompletionUsagePatch builds metadata patch after usage is recorded.
func MergeRunnerCompletionUsagePatch(usageEventID, traceID string) string {
	patch := map[string]any{"schema_version": "runner.completion/v1"}
	if v := strings.TrimSpace(usageEventID); v != "" {
		patch["usage_event_id"] = v
	}
	if v := strings.TrimSpace(traceID); v != "" {
		patch["trace_id"] = v
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (u *Usecase) RebuildRingBuffer(ctx context.Context, rb *MetricRingBuffer) int {
	if u == nil || u.eventRepo == nil || rb == nil {
		return 0
	}
	now := time.Now().UTC()
	windowMinutes := defaultBucketCapacity
	rebuilt := 0
	for i := windowMinutes - 1; i >= 0; i-- {
		bucketStart := now.Add(-time.Duration(i) * time.Minute).Truncate(rb.bucketSize)
		since := bucketStart.Format(time.RFC3339)
		until := bucketStart.Add(rb.bucketSize).Format(time.RFC3339)
		total, errT := u.eventRepo.CountMonitorEventsSince(ctx, "runner.completion", "", since, until)
		if errT != nil {
			continue
		}
		errors, errE := u.eventRepo.CountMonitorEventsSince(ctx, "runner.completion", "error", since, until)
		if errE != nil {
			continue
		}
		rb.mu.Lock()
		b := rb.ensureBucketAt(bucketStart.Unix())
		b.totals["runner.completion"] = int64(total)
		b.errors["runner.completion"] = int64(errors)
		rb.mu.Unlock()
		rebuilt++
	}
	return rebuilt
}
