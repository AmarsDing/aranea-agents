// Package monitor implements audit logging, monitor events, and alert evaluation workflows.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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
}

// EventsQuery filters monitor events list.
type EventsQuery struct {
	Limit     int32
	Offset    int32
	EventType string
	AgentID   string
	Status    string
}

// TracesQuery filters monitor traces list.
type TracesQuery struct {
	Limit    int32
	Offset   int32
	AgentID  string
	Provider string
	Model    string
	Status   string
}

// ListResult is a paginated list of platform rows.
type ListResult struct {
	Items []PlatformRow
	Total int32
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
	CreatedAt        string
	UpdatedAt        string

	// MON-OPT-02: persistent firing state machine.
	FiringState            AlertFiringState // idle | firing | recovered
	LastFiredAt            *time.Time       // unix ms persisted in DB
	LastFiredValue         float64          // metric value at last fire
	LastFiredWindowStart   *time.Time
	RecoveredAt            *time.Time
	RecoveryFactor         float64 // 0.9 default: metric must drop below Threshold×RecoveryFactor to recover
}

// AlertNotifier delivers fired alerts to external channels.
type AlertNotifier interface {
	Notify(ctx context.Context, rule AlertRule, payload map[string]any)
}

// Repo abstracts monitor persistence.
type Repo interface {
	ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error)
	InsertAuditLog(ctx context.Context, entry AuditLog) error
	InsertMonitorEvent(ctx context.Context, ev EventWrite) error
	ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error)
	GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error)
	ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error)
	GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error)
	ListAlertRules(ctx context.Context) ([]AlertRule, error)
	ReplaceAlertRules(ctx context.Context, rules []AlertRule) error
	// UpdateAlertFiringState persists the firing state machine columns (MON-OPT-02).
	// SQLite: runs inside BEGIN IMMEDIATE to prevent duplicate fires on restart.
	UpdateAlertFiringState(ctx context.Context, id string, state AlertFiringState, lastFiredAt *time.Time, lastFiredValue float64, recoveredAt *time.Time) error
	CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339 string) (int32, error)
	AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error)
	ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error)
	PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error)
}

// RunnerMetricsSummary aggregates runner.completion monitor events.
type RunnerMetricsSummary struct {
	WindowMinutes int
	TotalRuns     int32
	ErrorRuns     int32
	ErrorRate     float64
	SuccessRate   float64
	AvgDurationMs float64
}

// FilesystemHealthReader supplies live skill filesystem health for alerts.
type FilesystemHealthReader interface {
	FilesystemHealthStats(ctx context.Context) (missingCount int, pendingCount int, err error)
}

// Usecase implements monitor workflows.
type Usecase struct {
	repo        Repo
	notifier    AlertNotifier
	fsHealth    FilesystemHealthReader
	lastFired   sync.Map
	rulesCache  []AlertRule
	rulesExpire time.Time
	rulesMu     sync.RWMutex
}

const rulesCacheTTL = 5 * time.Minute

// SetFilesystemHealthReader configures skill filesystem metrics for alert rules.
func (u *Usecase) SetFilesystemHealthReader(r FilesystemHealthReader) {
	if u == nil {
		return
	}
	u.fsHealth = r
}

// NewUsecase constructs a MonitorUsecase.
func NewUsecase(repo Repo, notifier AlertNotifier) *Usecase {
	return &Usecase{repo: repo, notifier: notifier}
}

// RecordAuditLog persists an admin audit row (best-effort, logs on failure).
func (u *Usecase) RecordAuditLog(ctx context.Context, entry AuditLog) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = uuid.NewString()
	}
	if err := u.repo.InsertAuditLog(ctx, entry); err != nil {
		slog.Warn("RecordAuditLog failed", "action", entry.Action, "resource_id", entry.ResourceID, "error", err)
		return err
	}
	return nil
}

// RecordMonitorEvent persists a monitor_events row (best-effort, logs on failure).
func (u *Usecase) RecordMonitorEvent(ctx context.Context, ev EventWrite) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if err := u.repo.InsertMonitorEvent(ctx, ev); err != nil {
		slog.Warn("RecordMonitorEvent failed", "event_key", ev.EventKey, "error", err)
		return err
	}
	return nil
}

// ListAuditLogs returns paginated audit logs.
func (u *Usecase) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	return u.repo.ListAuditLogs(ctx, query)
}

// ListMonitorEvents returns paginated monitor events.
func (u *Usecase) ListMonitorEvents(ctx context.Context, query EventsQuery) (ListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorEvents(ctx, query)
}

// ListAlertRules returns all alert rules.
func (u *Usecase) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListAlertRules(ctx)
}

// ReplaceAlertRules replaces all alert rules and cleans up lastFired entries for deleted rules.
func (u *Usecase) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	if u == nil || u.repo == nil {
		return nil
	}

	oldRules, _ := u.repo.ListAlertRules(ctx)
	oldIDs := make(map[string]struct{}, len(oldRules))
	for _, r := range oldRules {
		oldIDs[r.ID] = struct{}{}
	}

	if err := u.repo.ReplaceAlertRules(ctx, rules); err != nil {
		return err
	}

	// Invalidate rules cache
	u.rulesMu.Lock()
	u.rulesCache = nil
	u.rulesExpire = time.Time{}
	u.rulesMu.Unlock()

	newIDs := make(map[string]struct{}, len(rules))
	for _, r := range rules {
		newIDs[r.ID] = struct{}{}
	}
	for id := range oldIDs {
		if _, exists := newIDs[id]; !exists {
			u.lastFired.Delete(id)
		}
	}
	return nil
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (u *Usecase) EvaluateAlerts(ctx context.Context) {
	if u == nil || u.repo == nil {
		return
	}
	rules := u.cachedAlertRules(ctx)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		switch strings.TrimSpace(rule.MetricKey) {
		case "runner.error_rate":
			u.evaluateRunnerErrorRate(ctx, rule)
		case "skill.filesystem_missing_count":
			u.evaluateSkillFilesystemMissingCount(ctx, rule)
		}
	}
}

func (u *Usecase) cachedAlertRules(ctx context.Context) []AlertRule {
	u.rulesMu.RLock()
	if u.rulesCache != nil && time.Now().Before(u.rulesExpire) {
		rules := u.rulesCache
		u.rulesMu.RUnlock()
		return rules
	}
	u.rulesMu.RUnlock()

	rules, err := u.repo.ListAlertRules(ctx)
	if err != nil {
		return nil
	}

	u.rulesMu.Lock()
	u.rulesCache = rules
	u.rulesExpire = time.Now().Add(rulesCacheTTL)
	u.rulesMu.Unlock()

	return rules
}

func (u *Usecase) evaluateRunnerErrorRate(ctx context.Context, rule AlertRule) {
	if u == nil || u.repo == nil {
		return
	}
	window := rule.WindowMinutes
	if window <= 0 {
		window = 60
	}
	since := time.Now().UTC().Add(-time.Duration(window) * time.Minute).Format(time.RFC3339)
	total, errTotal := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "", since)
	if errTotal != nil {
		slog.Warn("EvaluateAlerts: CountMonitorEventsSince(total) failed", "rule_id", rule.ID, "error", errTotal)
		return
	}
	if total == 0 {
		// MON-OPT-02: if rule is firing and there are 0 completions, treat as recovered.
		if rule.FiringState == AlertFiringStateFiring {
			u.MarkAlertRecovered(ctx, rule, time.Now().UTC())
		}
		return
	}
	errors, errErrors := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "error", since)
	if errErrors != nil {
		slog.Warn("EvaluateAlerts: CountMonitorEventsSince(errors) failed", "rule_id", rule.ID, "error", errErrors)
		return
	}
	rate := float64(errors) / float64(total)
	now := time.Now().UTC()

	// MON-OPT-02: recovery transition — firing rule whose metric dropped below recovery threshold.
	if rule.FiringState == AlertFiringStateFiring && rate < recoveryThreshold(rule) {
		u.MarkAlertRecovered(ctx, rule, now)
		// Publish alert.recovered event.
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "error_rate": rate, "total": total, "window_minutes": window,
		})
		_ = u.RecordMonitorEvent(ctx, EventWrite{
			EventKey:     "alert.recovered",
			Name:         rule.Name,
			Description:  fmt.Sprintf("error rate %.2f recovered below %.2f", rate, recoveryThreshold(rule)),
			Status:       "recovered",
			MetadataJSON: string(meta),
		})
		if u.notifier != nil {
			u.notifier.Notify(ctx, rule, map[string]any{
				"rule_id": rule.ID, "name": rule.Name, "error_rate": rate,
				"recovered_at": now.Format(time.RFC3339),
			})
		}
		return
	}

	if rate < rule.Threshold {
		return
	}
	if !u.ShouldFireAlert(rule, now) {
		return
	}
	// MON-OPT-02: persist firing state before publishing (prevents duplicate fires on restart).
	u.MarkAlertFiredPersistent(ctx, rule, now, rate)
	meta, _ := json.Marshal(map[string]any{
		"rule_id": rule.ID, "error_rate": rate, "errors": errors, "total": total, "window_minutes": window,
	})
	if err := u.RecordMonitorEvent(ctx, EventWrite{
		EventKey:     "alert.fired",
		Name:         rule.Name,
		Description:  fmt.Sprintf("error rate %.2f >= %.2f", rate, rule.Threshold),
		Status:       strings.TrimSpace(rule.Severity),
		MetadataJSON: string(meta),
	}); err != nil {
		slog.Warn("RecordMonitorEvent for alert.fired failed", "rule_id", rule.ID, "error", err)
	}
	payload := map[string]any{
		"rule_id":        rule.ID,
		"name":           rule.Name,
		"error_rate":     rate,
		"errors":         errors,
		"total":          total,
		"window_minutes": window,
		"severity":       strings.TrimSpace(rule.Severity),
		"fired_at":       now.Format(time.RFC3339),
	}
	if u.notifier != nil {
		u.notifier.Notify(ctx, rule, payload)
	}
}

func (u *Usecase) evaluateSkillFilesystemMissingCount(ctx context.Context, rule AlertRule) {
	if u == nil || u.fsHealth == nil {
		return
	}
	missing, pending, err := u.fsHealth.FilesystemHealthStats(ctx)
	if err != nil {
		slog.Warn("EvaluateAlerts: FilesystemHealthStats failed", "rule_id", rule.ID, "error", err)
		return
	}
	missingF := float64(missing)
	now := time.Now().UTC()

	// MON-OPT-02: recovery transition — firing rule whose metric dropped below recovery threshold.
	if rule.FiringState == AlertFiringStateFiring && missingF < recoveryThreshold(rule) {
		u.MarkAlertRecovered(ctx, rule, now)
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "missing_count": missing, "threshold": rule.Threshold,
		})
		_ = u.RecordMonitorEvent(ctx, EventWrite{
			EventKey:     "alert.recovered",
			Name:         rule.Name,
			Description:  fmt.Sprintf("skill filesystem missing %d recovered below %.0f", missing, recoveryThreshold(rule)),
			Status:       "recovered",
			MetadataJSON: string(meta),
		})
		if u.notifier != nil {
			u.notifier.Notify(ctx, rule, map[string]any{
				"rule_id": rule.ID, "name": rule.Name, "missing_count": missing,
				"recovered_at": now.Format(time.RFC3339),
			})
		}
		return
	}

	if missingF < rule.Threshold {
		return
	}
	if !u.ShouldFireAlert(rule, now) {
		return
	}
	// MON-OPT-02: persist firing state before publishing (prevents duplicate fires on restart).
	u.MarkAlertFiredPersistent(ctx, rule, now, missingF)
	meta, _ := json.Marshal(map[string]any{
		"rule_id": rule.ID, "missing_count": missing, "pending_filesystem_count": pending, "threshold": rule.Threshold,
	})
	if err := u.RecordMonitorEvent(ctx, EventWrite{
		EventKey:     "alert.fired",
		Name:         rule.Name,
		Description:  fmt.Sprintf("skill filesystem missing %d >= %.0f", missing, rule.Threshold),
		Status:       strings.TrimSpace(rule.Severity),
		MetadataJSON: string(meta),
	}); err != nil {
		slog.Warn("RecordMonitorEvent for skill alert.fired failed", "rule_id", rule.ID, "error", err)
	}
	payload := map[string]any{
		"rule_id":                  rule.ID,
		"name":                     rule.Name,
		"missing_count":            missing,
		"pending_filesystem_count": pending,
		"threshold":                rule.Threshold,
		"severity":                 strings.TrimSpace(rule.Severity),
		"fired_at":                 now.Format(time.RFC3339),
	}
	if u.notifier != nil {
		u.notifier.Notify(ctx, rule, payload)
	}
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
	// recovered state also enforces cooldown
	if rule.FiringState == AlertFiringStateRecovered && rule.RecoveredAt != nil {
		if now.Sub(*rule.RecoveredAt) < cooldownDur {
			return false
		}
	}

	// Legacy in-memory fallback (pre-OPT-02 data or repo not wired).
	if rule.LastFiredAt == nil {
		if v, ok := u.lastFired.Load(rule.ID); ok {
			if last, ok := v.(time.Time); ok && now.Sub(last) < cooldownDur {
				return false
			}
		}
	}
	return true
}

// MarkAlertFired records the firing event both in-memory (fast path) and in the DB
// (persistent path, MON-OPT-02). Failures are logged but do not abort the alert.
func (u *Usecase) MarkAlertFired(ruleID string, now time.Time) {
	if u == nil {
		return
	}
	u.lastFired.Store(ruleID, now)
}

// MarkAlertFiredPersistent is the DB-backed version called by evaluateRunner* after
// the alert.fired event is published (MON-OPT-02). It also advances the state machine
// from idle/recovered → firing.
func (u *Usecase) MarkAlertFiredPersistent(ctx context.Context, rule AlertRule, now time.Time, metricValue float64) {
	if u == nil || u.repo == nil {
		return
	}
	u.lastFired.Store(rule.ID, now)
	if err := u.repo.UpdateAlertFiringState(ctx, rule.ID, AlertFiringStateFiring, &now, metricValue, nil); err != nil {
		slog.Warn("MarkAlertFiredPersistent: DB update failed", "rule_id", rule.ID, "error", err)
	}
	// Invalidate rules cache so next evaluation round reads fresh DB state.
	u.rulesMu.Lock()
	u.rulesExpire = time.Time{}
	u.rulesMu.Unlock()
}

// MarkAlertRecovered transitions a firing alert to recovered and persists it (MON-OPT-02).
func (u *Usecase) MarkAlertRecovered(ctx context.Context, rule AlertRule, now time.Time) {
	if u == nil || u.repo == nil {
		return
	}
	if err := u.repo.UpdateAlertFiringState(ctx, rule.ID, AlertFiringStateRecovered, rule.LastFiredAt, rule.LastFiredValue, &now); err != nil {
		slog.Warn("MarkAlertRecovered: DB update failed", "rule_id", rule.ID, "error", err)
	}
	u.rulesMu.Lock()
	u.rulesExpire = time.Time{}
	u.rulesMu.Unlock()
}

// recoveryThreshold returns the value below which a firing alert is considered recovered.
func recoveryThreshold(rule AlertRule) float64 {
	f := rule.RecoveryFactor
	if f <= 0 || f > 1 {
		f = defaultRecoveryFactor
	}
	return rule.Threshold * f
}

func (u *Usecase) CleanupStaleLastFired(now time.Time, maxAge time.Duration) {
	if u == nil || maxAge <= 0 {
		return
	}
	u.lastFired.Range(func(key, value any) bool {
		if t, ok := value.(time.Time); ok && now.Sub(t) > maxAge {
			u.lastFired.Delete(key)
		}
		return true
	})
}

// GetMonitorEvent returns one monitor event by ID.
func (u *Usecase) GetMonitorEvent(ctx context.Context, id string) (PlatformRow, error) {
	return u.repo.GetMonitorEvent(ctx, id)
}

// ListMonitorTraces returns paginated monitor traces.
func (u *Usecase) ListMonitorTraces(ctx context.Context, query TracesQuery) (ListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorTraces(ctx, query)
}

// GetMonitorTrace returns one monitor trace by ID.
func (u *Usecase) GetMonitorTrace(ctx context.Context, id string) (PlatformRow, error) {
	return u.repo.GetMonitorTrace(ctx, id)
}

// GetRunnerMetrics aggregates runner.completion monitor events.
func (u *Usecase) GetRunnerMetrics(ctx context.Context, windowMinutes int) (RunnerMetricsSummary, error) {
	out := RunnerMetricsSummary{WindowMinutes: windowMinutes}
	if u == nil || u.repo == nil {
		return out, nil
	}
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	out.WindowMinutes = windowMinutes
	since := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute).Format(time.RFC3339)
	total, err := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "", since)
	if err != nil {
		return out, err
	}
	errors, err := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "error", since)
	if err != nil {
		return out, err
	}
	out.TotalRuns = total
	out.ErrorRuns = errors
	if total > 0 {
		out.ErrorRate = float64(errors) / float64(total)
		out.SuccessRate = 1 - out.ErrorRate
	}
	if avg, err := u.repo.AvgRunnerCompletionDurationMsSince(ctx, since); err == nil {
		out.AvgDurationMs = avg
	}
	return out, nil
}

// RecordRunnerCompletion persists a runner.completion event and patches metadata.
func (u *Usecase) RecordRunnerCompletion(ctx context.Context, write EventWrite, sessionID, runID, invocationID, usageEventID, traceID string, bridge RunnerCompletionBridge) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if sessionID != "" && invocationID != "" {
		exists, err := u.repo.ExistsRunnerCompletion(ctx, sessionID, invocationID)
		if err != nil {
			return err
		}
		if exists {
			patched, err := u.PatchRunnerCompletionLink(ctx, sessionID, runID, invocationID, usageEventID, traceID, bridge)
			if err != nil {
				return err
			}
			if patched || strings.TrimSpace(usageEventID) != "" {
				bridge.ClearTurn(sessionID, runID)
			}
			return nil
		}
	}
	if err := u.repo.InsertMonitorEvent(ctx, write); err != nil {
		return err
	}
	patched, err := u.PatchRunnerCompletionLink(ctx, sessionID, runID, invocationID, usageEventID, traceID, bridge)
	if err != nil {
		return err
	}
	if patched || strings.TrimSpace(usageEventID) != "" {
		bridge.ClearTurn(sessionID, runID)
	}
	return nil
}

// LinkRunnerCompletionUsage patches the latest completion row with usage_event_id.
func (u *Usecase) LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string, bridge RunnerCompletionBridge) error {
	if u == nil || u.repo == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	usageEventID = strings.TrimSpace(usageEventID)
	if sessionID == "" || runID == "" || usageEventID == "" {
		return nil
	}
	bridge.RegisterTurnUsage(sessionID, runID, usageEventID, traceID, "", "")
	patched, err := u.PatchRunnerCompletionLink(ctx, sessionID, runID, runID, usageEventID, traceID, bridge)
	if err != nil {
		return err
	}
	if patched {
		bridge.ClearTurn(sessionID, runID)
	}
	return nil
}

// PatchRunnerCompletionLink patches runner completion metadata with usage correlation.
func (u *Usecase) PatchRunnerCompletionLink(ctx context.Context, sessionID, runID, invocationID, usageEventID, traceID string, bridge RunnerCompletionBridge) (bool, error) {
	usageEventID = strings.TrimSpace(usageEventID)
	traceID = strings.TrimSpace(traceID)
	if usageEventID == "" {
		if u2, t2, ok := bridge.PendingUsage(sessionID, runID); ok {
			usageEventID = u2
			if traceID == "" {
				traceID = t2
			}
		}
	}
	if usageEventID == "" {
		return false, nil
	}
	patch := MergeRunnerCompletionUsagePatch(usageEventID, traceID)
	return u.repo.PatchRunnerCompletionMetadata(ctx, sessionID, runID, invocationID, patch)
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
