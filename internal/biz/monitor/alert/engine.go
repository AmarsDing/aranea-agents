package alert

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
)

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

// AlertRepo handles alert rule persistence and state.
type AlertRepo interface {
	ListAlertRules(ctx context.Context) ([]AlertRule, error)
	ReplaceAlertRules(ctx context.Context, rules []AlertRule) error
	UpdateAlertFiringState(ctx context.Context, id string, state AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error
}

// EventSink records an alert lifecycle event (alert.fired / alert.recovered)
// to the monitor event store. Implemented by the root monitor.Usecase; kept
// narrow so the alert package does not depend on its parent.
type EventSink interface {
	RecordAlertEvent(ctx context.Context, key, name, description, status, metadataJSON string) error
}

// LogPair is a key-value pair for structured flow log extras.
type LogPair struct {
	Key   string
	Value any
}

// FlowLogWriter is the narrow user-visible flow-log port the engine needs.
// This is not the deleted event.WithFlowLogger / NewFlowLogger aliases
// (those were ctx helpers around TraceEmitter). The root monitor package
// adapts its FlowLogWriter to this interface.
type FlowLogWriter interface {
	LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair)
}

// defaultRecoveryFactor is the fraction of the threshold below which a firing alert is considered recovered.
const defaultRecoveryFactor = 0.9

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

// Engine owns the alert evaluation domain: rule CRUD, metric evaluation,
// the firing state machine, and event/notification emission.
type Engine struct {
	repo     AlertRepo
	events   EventCounter
	notifier AlertNotifier
	registry *AlertMetricRegistry
	sink     EventSink
	flowLog  FlowLogWriter
	lg       loggateway.Logger

	rulesCache alertRulesCache
}

// EngineOption customizes optional Engine dependencies.
type EngineOption func(*Engine)

// WithRegistry wires the shared metric registry.
func WithRegistry(r *AlertMetricRegistry) EngineOption {
	return func(e *Engine) { e.registry = r }
}

// WithEventSink wires the alert lifecycle event sink.
func WithEventSink(s EventSink) EngineOption {
	return func(e *Engine) { e.sink = s }
}

// WithFlowLogWriter wires the user-visible flow log port (nil-safe).
func WithFlowLogWriter(fl FlowLogWriter) EngineOption {
	return func(e *Engine) { e.flowLog = fl }
}

// WithLogger wires the process logger.
func WithLogger(lg loggateway.Logger) EngineOption {
	return func(e *Engine) { e.lg = lg }
}

// NewEngine builds the alert engine. repo/events/notifier may be nil in
// tests; every method nil-checks its collaborators.
func NewEngine(repo AlertRepo, events EventCounter, notifier AlertNotifier, opts ...EngineOption) *Engine {
	e := &Engine{repo: repo, events: events, notifier: notifier}
	for _, opt := range opts {
		opt(e)
	}
	if e.lg == nil {
		e.lg = loggateway.NewNoop()
	}
	return e
}

// recordEvent persists an alert lifecycle event via the sink (best-effort:
// callers log failures and continue).
func (e *Engine) recordEvent(ctx context.Context, key, name, description, status, metadataJSON, stepID, ruleID string) {
	if e.sink == nil {
		return
	}
	if err := e.sink.RecordAlertEvent(ctx, key, name, description, status, metadataJSON); err != nil {
		e.lg.Warn("RecordAlertEvent failed",
			loggateway.StepID(stepID),
			loggateway.Str("rule_id", ruleID),
			loggateway.Err(err))
	}
}

// ListAlertRules returns all alert rules.
func (e *Engine) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	if e == nil || e.repo == nil {
		return nil, nil
	}
	return e.repo.ListAlertRules(ctx)
}

// ListAlertRulesWithDefaults returns alert rules, creating defaults if none exist.
func (e *Engine) ListAlertRulesWithDefaults(ctx context.Context) ([]AlertRule, error) {
	rules, err := e.ListAlertRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		defaults := DefaultAlertRules()
		if err := e.ReplaceAlertRules(ctx, defaults); err != nil {
			e.lg.Warn("ListAlertRulesWithDefaults: ReplaceAlertRules failed",
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
func (e *Engine) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	if e == nil || e.repo == nil {
		return nil
	}
	for _, r := range rules {
		if err := validateAlertRule(r); err != nil {
			return err
		}
	}

	if err := e.repo.ReplaceAlertRules(ctx, rules); err != nil {
		return err
	}

	// Invalidate rules cache
	e.rulesCache.invalidate()
	return nil
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (e *Engine) EvaluateAlerts(ctx context.Context) {
	if e == nil || e.repo == nil {
		return
	}
	rules := e.cachedAlertRules(ctx)
	ruleCount := 0
	triggeredCount := 0
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		ruleCount++
		metricKey := strings.TrimSpace(rule.MetricKey)
		if e.registry == nil {
			// Legacy dual-track switch removed (S1): evaluation only runs via
			// the metric registry. Nil registry means no metrics can be
			// evaluated — wire always provides one in production.
			e.lg.Debug("EvaluateAlerts: no metric registry, skipping rule",
				loggateway.StepID("monitor.alert_eval_no_registry"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey))
			continue
		}
		m, ok := e.registry.Get(metricKey)
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
			e.lg.Debug("EvaluateAlerts: no metric data in window, skipping",
				loggateway.StepID("monitor.alert_eval_no_data"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey))
			continue
		}
		if err != nil {
			e.lg.Warn("EvaluateAlerts: metric evaluation failed",
				loggateway.StepID("monitor.alert_eval_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Str("metric_key", metricKey), loggateway.Err(err))
			continue
		}
		if e.evaluateMetricValue(ctx, rule, value, m) {
			triggeredCount++
		}
	}
	if e.flowLog != nil {
		e.flowLog.LogFlowDone(ctx, "", "monitor.alert.evaluate", "告警评估完成",
			LogPair{Key: "rule_count", Value: ruleCount},
			LogPair{Key: "triggered_count", Value: triggeredCount})
	}
}

// evaluateMetricValue applies the alert state machine to one metric sample.
// metric may be nil; when it implements AlertBreachDetailer the breach
// summary/details of the most recent Evaluate call are merged into
// alert.fired event metadata and notifier payloads.
// Returns true when the rule newly fired (idle/recovered → firing) this call.
func (e *Engine) evaluateMetricValue(ctx context.Context, rule AlertRule, value float64, metric AlertMetric) bool {
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
				e.MarkAlertReset(ctx, rule)
			}
		}
	}

	if rule.FiringState == AlertFiringStateFiring && value < RecoveryThreshold(rule) {
		e.MarkAlertRecovered(ctx, rule, now)
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "recovery_threshold": RecoveryThreshold(rule),
		})
		e.recordEvent(ctx, "alert.recovered", rule.Name,
			fmt.Sprintf("%s %.2f recovered below %.2f", rule.MetricKey, value, RecoveryThreshold(rule)),
			"recovered", string(meta), "monitor.alert_recovered_persist_fail", rule.ID)
		if e.notifier != nil {
			e.notifier.Notify(ctx, rule, map[string]any{
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
		if !e.shouldRemindAlert(rule, now) {
			return false
		}
		e.touchAlertReminder(ctx, rule, now, value)
		metaMap := map[string]any{
			"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "threshold": rule.Threshold, "reminder": true,
		}
		for k, v := range breachPayload {
			metaMap[k] = v
		}
		meta, _ := json.Marshal(metaMap)
		e.recordEvent(ctx, "alert.fired", rule.Name,
			appendBreachSummary(fmt.Sprintf("%s %.2f >= %.2f (reminder)", rule.MetricKey, value, rule.Threshold), breachSummary),
			strings.TrimSpace(rule.Severity), string(meta), "monitor.alert_fired_persist_fail", rule.ID)
		if e.notifier != nil {
			e.notifier.Notify(ctx, rule, map[string]any{
				"rule_id": rule.ID, "name": rule.Name, "metric_key": rule.MetricKey,
				"value": value, "threshold": rule.Threshold, "reminder": true,
				"severity": strings.TrimSpace(rule.Severity), "fired_at": now.Format(time.RFC3339),
			})
		}
		return false
	}
	if !e.ShouldFireAlert(rule, now) {
		return false
	}
	e.MarkAlertFiredPersistent(ctx, rule, now, value)
	metaMap := map[string]any{
		"rule_id": rule.ID, "metric_key": rule.MetricKey, "value": value, "threshold": rule.Threshold,
	}
	for k, v := range breachPayload {
		metaMap[k] = v
	}
	meta, _ := json.Marshal(metaMap)
	e.recordEvent(ctx, "alert.fired", rule.Name,
		appendBreachSummary(fmt.Sprintf("%s %.2f >= %.2f", rule.MetricKey, value, rule.Threshold), breachSummary),
		strings.TrimSpace(rule.Severity), string(meta), "monitor.alert_fired_persist_fail", rule.ID)
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
	if e.notifier != nil {
		e.notifier.Notify(ctx, rule, payload)
	}
	return true
}

func (e *Engine) cachedAlertRules(ctx context.Context) []AlertRule {
	if rules := e.rulesCache.get(); rules != nil {
		return rules
	}

	rules, err := e.repo.ListAlertRules(ctx)
	if err != nil {
		e.lg.Warn("cachedAlertRules: ListAlertRules failed", loggateway.StepID("monitor.alert_rules_load_fail"), loggateway.Err(err))
		return nil
	}

	e.rulesCache.set(rules)
	return rules
}

// ShouldFireAlert checks whether an alert rule should fire now.
//
// MON-OPT-02: Cooldown is evaluated against DB-persisted LastFiredAt (loaded via
// ListAlertRules cache). This survives process restarts and prevents duplicate fires
// across replicas when SQLite is used (single-writer).
func (e *Engine) ShouldFireAlert(rule AlertRule, now time.Time) bool {
	if e == nil {
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

func (e *Engine) shouldRemindAlert(rule AlertRule, now time.Time) bool {
	if rule.LastFiredAt == nil {
		return true
	}
	return now.Sub(*rule.LastFiredAt) >= reminderInterval(rule)
}

// touchAlertReminder refreshes LastFiredAt while staying in firing state (reminder path).
func (e *Engine) touchAlertReminder(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if e == nil || e.repo == nil {
		return
	}
	if err := e.repo.UpdateAlertFiringState(ctx, rule.ID, AlertFiringStateFiring, &now, metricValue, nil); err != nil {
		e.lg.Warn("touchAlertReminder: DB update failed",
			loggateway.StepID("monitor.alert_reminder_db_fail"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Err(err))
	}
	e.rulesCache.invalidate()
}

// MarkAlertFiredPersistent is the DB-backed mark called after the alert.fired
// event is published (MON-OPT-02). It also advances the state machine from
// idle/recovered → firing.
func (e *Engine) MarkAlertFiredPersistent(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if e == nil || e.repo == nil {
		return
	}
	// Validate state machine transition: current → firing
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventThresholdExceeded)
	if err != nil {
		e.lg.Warn("MarkAlertFiredPersistent: invalid state transition",
			loggateway.StepID("monitor.mark_fired_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	e.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := e.repo.UpdateAlertFiringState(ctx, rule.ID, next, &now, metricValue, nil); err != nil {
		e.lg.Warn("MarkAlertFiredPersistent: DB update failed", loggateway.StepID("monitor.mark_fired_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	// Invalidate rules cache so next evaluation round reads fresh DB state.
	e.rulesCache.invalidate()
}

// MarkAlertRecovered transitions a firing alert to recovered and persists it (MON-OPT-02).
func (e *Engine) MarkAlertRecovered(ctx context.Context, rule AlertRule, now time.Time) {
	if e == nil || e.repo == nil {
		return
	}
	// Validate state machine transition: current → recovered
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventRecovered)
	if err != nil {
		e.lg.Warn("MarkAlertRecovered: invalid state transition",
			loggateway.StepID("monitor.mark_recovered_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	e.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := e.repo.UpdateAlertFiringState(ctx, rule.ID, next, rule.LastFiredAt, rule.LastFiredValue, &now); err != nil {
		e.lg.Warn("MarkAlertRecovered: DB update failed", loggateway.StepID("monitor.mark_recovered_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	e.rulesCache.invalidate()
}

// MarkAlertReset transitions a recovered alert back to idle after cooldown expires.
func (e *Engine) MarkAlertReset(ctx context.Context, rule AlertRule) {
	if e == nil || e.repo == nil {
		return
	}
	next, err := TransitionAlertFiringState(rule.FiringState, AlertEventReset)
	if err != nil {
		e.lg.Warn("MarkAlertReset: invalid state transition",
			loggateway.StepID("monitor.mark_reset_invalid_transition"),
			loggateway.Str("rule_id", rule.ID),
			loggateway.Str("from_state", string(rule.FiringState)),
			loggateway.Err(err))
		return
	}
	e.lg.Info("alert state transition",
		loggateway.StepID("monitor.alert_state"),
		loggateway.Str("rule_id", rule.ID),
		loggateway.Str("severity", strings.TrimSpace(rule.Severity)),
		loggateway.Str("from", string(rule.FiringState)),
		loggateway.Str("to", string(next)))
	if err := e.repo.UpdateAlertFiringState(ctx, rule.ID, next, nil, 0, nil); err != nil {
		e.lg.Warn("MarkAlertReset: DB update failed", loggateway.StepID("monitor.mark_reset_db_fail"), loggateway.Str("rule_id", rule.ID), loggateway.Err(err))
	}
	e.rulesCache.invalidate()
}

// RecoveryThreshold returns the value below which a firing alert is considered recovered.
func RecoveryThreshold(rule AlertRule) float64 {
	f := rule.RecoveryFactor
	if f <= 0 || f > 1 {
		f = defaultRecoveryFactor
	}
	return rule.Threshold * f
}

// RebuildRingBuffer replays persisted runner.completion counts into the ring
// buffer so alert windows survive process restarts. Returns rebuilt buckets.
func (e *Engine) RebuildRingBuffer(ctx context.Context, rb *MetricRingBuffer) int {
	if e == nil || e.events == nil || rb == nil {
		return 0
	}
	now := time.Now().UTC()
	rebuilt := 0
	for i := defaultBucketCapacity - 1; i >= 0; i-- {
		bucketStart := now.Add(-time.Duration(i) * time.Minute).Truncate(rb.BucketSize())
		since := bucketStart.Format(time.RFC3339)
		until := bucketStart.Add(rb.BucketSize()).Format(time.RFC3339)
		total, errT := e.events.CountMonitorEventsSince(ctx, "runner.completion", "", since, until)
		if errT != nil {
			continue
		}
		errs, errE := e.events.CountMonitorEventsSince(ctx, "runner.completion", "error", since, until)
		if errE != nil {
			continue
		}
		rb.seedBucket(bucketStart.Unix(), int64(total), int64(errs))
		rebuilt++
	}
	return rebuilt
}

// AlertMetricCatalogEntry pairs catalog metadata with the metric's current
// value, evaluated at request time over its default window. CurrentValue is
// best-effort: evaluation errors are logged and leave the value at zero so a
// single broken metric does not fail the whole directory listing.
type AlertMetricCatalogEntry struct {
	AlertMetricInfo
	CurrentValue float64
	EvaluatedAt  time.Time
}

// ListAlertMetricCatalog returns the alert metric directory for the Alerts
// page: every registered metric with human-readable metadata plus its
// current value.
func (e *Engine) ListAlertMetricCatalog(ctx context.Context) []AlertMetricCatalogEntry {
	if e == nil || e.registry == nil {
		return nil
	}
	infos := e.registry.ListCatalog()
	out := make([]AlertMetricCatalogEntry, 0, len(infos))
	now := time.Now().UTC()
	for _, info := range infos {
		entry := AlertMetricCatalogEntry{AlertMetricInfo: info, EvaluatedAt: now}
		m, ok := e.registry.Get(info.Key)
		if !ok {
			out = append(out, entry)
			continue
		}
		window := time.Duration(info.DefaultWindowMinutes) * time.Minute
		if window <= 0 {
			window = time.Hour
		}
		if v, err := m.Evaluate(ctx, window); err != nil {
			// NoData is an expected idle-system state, not a failure — stay silent.
			if !errors.Is(err, ErrAlertMetricNoData) {
				e.lg.Warn("ListAlertMetricCatalog: evaluate failed",
					loggateway.StepID("monitor.alert_metric_eval_fail"),
					loggateway.Str("metric_key", info.Key),
					loggateway.Err(err),
				)
			}
		} else {
			entry.CurrentValue = v
		}
		out = append(out, entry)
	}
	return out
}
