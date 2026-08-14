// Package monitor implements audit logging, monitor events, and alert evaluation workflows.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz/monitor/alert"
	"aranea-agents/internal/biz/monitor/trace"
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

// DEV-05: trace domain models live in the trace subpackage; these aliases
// keep the historical monitor.* API surface intact.
type (
	TraceWrite       = trace.TraceWrite
	TraceSpanWrite   = trace.TraceSpanWrite
	TraceSpan        = trace.TraceSpan
	TraceCompletion  = trace.TraceCompletion
	UsageAggregate   = trace.UsageAggregate
	TraceUsageRepo   = trace.UsageRepo
	TraceSpanReader  = trace.SpanReader
	TraceProjector   = trace.TraceProjector
	FlowFileAppender = trace.FlowFileAppender
)

var (
	NewTraceProjector   = trace.NewTraceProjector
	NewFlowFileAppender = trace.NewFlowFileAppender
	TraceSpansRaw       = trace.TraceSpansRaw
)

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

// DEV-05: alert domain types live in the alert subpackage; these aliases
// keep the historical monitor.* API surface intact.
type (
	// AlertRule defines a simple threshold alert on monitor_events aggregates.
	AlertRule = alert.AlertRule
	// AlertNotifier delivers fired alerts to external channels.
	AlertNotifier = alert.AlertNotifier
)

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
type AlertRepo = alert.AlertRepo

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

// Usecase orchestrates monitoring sub-domains.
// DEV-05 done: alert/ (engine, state machine, worker), trace/ (projector,
// flow file appender), heal/ (self-heal, observer, predictive heal, RCA
// engine, pattern mining, failure knowledge base, diag bundle).
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
	engine     *alert.Engine    // DEV-05: 告警域实现主体（规则 CRUD/状态机/评估）

	// --- trace / 日志落盘域 ---
	traceProjector   *TraceProjector
	flowFileAppender *FlowFileAppender

	// --- 横切 ---
	lg      loggateway.Logger
	flowLog FlowLogWriter
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

func NewUsecase(audit AuditRepo, event EventRepo, trace TraceRepo, alertRepo AlertRepo, runner RunnerCompletionRepo, notifier AlertNotifier, opts ...UsecaseOption) *Usecase {
	uc := &Usecase{auditRepo: audit, eventRepo: event, traceRepo: trace, alertRepo: alertRepo, runnerCompletion: runner, notifier: notifier}
	for _, opt := range opts {
		opt(uc)
	}
	if uc.lg == nil {
		uc.lg = loggateway.NewNoop()
	}
	// DEV-05: the alert engine owns the alert domain; the Usecase delegates.
	// Constructed after options so registry/flowLog/lg are already wired.
	uc.engine = alert.NewEngine(alertRepo, event, notifier,
		alert.WithRegistry(uc.registry),
		alert.WithEventSink(alertEventSink{uc: uc}),
		alert.WithFlowLogger(alertFlowLogger{w: uc.flowLog}),
		alert.WithLogger(uc.lg),
	)
	return uc
}

// alertEngine exposes the alert engine to in-package adapters (eval worker
// constructor). Nil-safe.
func (u *Usecase) alertEngine() *alert.Engine {
	if u == nil {
		return nil
	}
	return u.engine
}

// alertEventSink adapts the Usecase's RecordMonitorEvent to the alert
// package's narrow EventSink port (alert.fired / alert.recovered rows).
type alertEventSink struct{ uc *Usecase }

func (s alertEventSink) RecordAlertEvent(ctx context.Context, key, name, description, status, metadataJSON string) error {
	return s.uc.RecordMonitorEvent(ctx, EventWrite{
		EventKey:     key,
		Name:         name,
		Description:  description,
		Status:       status,
		MetadataJSON: metadataJSON,
	})
}

// alertFlowLogger adapts the monitor FlowLogWriter port to the alert
// package's narrow FlowLogger port (LogPair type translation).
type alertFlowLogger struct{ w FlowLogWriter }

func (a alertFlowLogger) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...alert.LogPair) {
	if a.w == nil {
		return
	}
	ps := make([]LogPair, 0, len(pairs))
	for _, p := range pairs {
		ps = append(ps, LogPair{Key: p.Key, Value: p.Value})
	}
	a.w.LogFlowDone(ctx, sessionID, stepID, message, ps...)
}

// SetEvalWorker is the ONLY remaining setter: the eval worker is constructed
// from the usecase's alert engine (see NewAlertEvalWorker), so it cannot be a
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
// DEV-05: delegates to the alert engine.
func (u *Usecase) ListAlertRulesWithDefaults(ctx context.Context) ([]AlertRule, error) {
	if u == nil || u.engine == nil {
		return nil, nil
	}
	return u.engine.ListAlertRulesWithDefaults(ctx)
}

// DefaultAlertRules returns the built-in default alert rules.
var DefaultAlertRules = alert.DefaultAlertRules

// ListAlertRules returns all alert rules.
func (u *Usecase) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	if u == nil || u.engine == nil {
		return nil, nil
	}
	return u.engine.ListAlertRules(ctx)
}

// ReplaceAlertRules replaces all alert rules.
func (u *Usecase) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	if u == nil || u.engine == nil {
		return nil
	}
	return u.engine.ReplaceAlertRules(ctx, rules)
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (u *Usecase) EvaluateAlerts(ctx context.Context) {
	if u == nil || u.engine == nil {
		return
	}
	u.engine.EvaluateAlerts(ctx)
}

// ShouldFireAlert checks whether an alert rule should fire now (MON-OPT-02:
// cooldown against DB-persisted LastFiredAt). DEV-05: delegates to the engine.
func (u *Usecase) ShouldFireAlert(rule AlertRule, now time.Time) bool {
	if u == nil || u.engine == nil {
		return false
	}
	return u.engine.ShouldFireAlert(rule, now)
}

// MarkAlertFiredPersistent is the DB-backed mark called after the alert.fired
// event is published (MON-OPT-02). DEV-05: delegates to the engine.
func (u *Usecase) MarkAlertFiredPersistent(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if u == nil || u.engine == nil {
		return
	}
	u.engine.MarkAlertFiredPersistent(ctx, rule, now, metricValue)
}

// MarkAlertRecovered transitions a firing alert to recovered and persists it (MON-OPT-02).
func (u *Usecase) MarkAlertRecovered(ctx context.Context, rule AlertRule, now time.Time) {
	if u == nil || u.engine == nil {
		return
	}
	u.engine.MarkAlertRecovered(ctx, rule, now)
}

// MarkAlertReset transitions a recovered alert back to idle after cooldown expires.
func (u *Usecase) MarkAlertReset(ctx context.Context, rule AlertRule) {
	if u == nil || u.engine == nil {
		return
	}
	u.engine.MarkAlertReset(ctx, rule)
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

// RebuildRingBuffer replays persisted runner.completion counts into the ring
// buffer so alert windows survive process restarts. DEV-05: delegates to the engine.
func (u *Usecase) RebuildRingBuffer(ctx context.Context, rb *MetricRingBuffer) int {
	if u == nil || u.engine == nil {
		return 0
	}
	return u.engine.RebuildRingBuffer(ctx, rb)
}
