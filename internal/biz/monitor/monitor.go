// Package monitor implements audit logging, monitor events, and alert evaluation workflows.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
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

// Usecase implements monitor workflows.
type Usecase struct {
	repo      Repo
	notifier  AlertNotifier
	lastFired sync.Map
}

// NewUsecase constructs a MonitorUsecase.
func NewUsecase(repo Repo, notifier AlertNotifier) *Usecase {
	return &Usecase{repo: repo, notifier: notifier}
}

// RecordAuditLog persists an admin audit row (best-effort).
func (u *Usecase) RecordAuditLog(ctx context.Context, entry AuditLog) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = uuid.NewString()
	}
	return u.repo.InsertAuditLog(ctx, entry)
}

// RecordMonitorEvent persists a monitor_events row (best-effort).
func (u *Usecase) RecordMonitorEvent(ctx context.Context, ev EventWrite) error {
	if u == nil || u.repo == nil {
		return nil
	}
	return u.repo.InsertMonitorEvent(ctx, ev)
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

// ReplaceAlertRules replaces all alert rules.
func (u *Usecase) ReplaceAlertRules(ctx context.Context, rules []AlertRule) error {
	if u == nil || u.repo == nil {
		return nil
	}
	return u.repo.ReplaceAlertRules(ctx, rules)
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (u *Usecase) EvaluateAlerts(ctx context.Context) {
	if u == nil || u.repo == nil {
		return
	}
	rules, err := u.repo.ListAlertRules(ctx)
	if err != nil {
		return
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.MetricKey != "runner.error_rate" {
			continue
		}
		window := rule.WindowMinutes
		if window <= 0 {
			window = 60
		}
		since := time.Now().UTC().Add(-time.Duration(window) * time.Minute).Format(time.RFC3339)
		total, _ := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "", since)
		if total == 0 {
			continue
		}
		errors, _ := u.repo.CountMonitorEventsSince(ctx, "runner.completion", "error", since)
		rate := float64(errors) / float64(total)
		if rate < rule.Threshold {
			continue
		}
		now := time.Now().UTC()
		if !u.ShouldFireAlert(rule, now) {
			continue
		}
		u.MarkAlertFired(rule.ID, now)
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "error_rate": rate, "errors": errors, "total": total, "window_minutes": window,
		})
		_ = u.RecordMonitorEvent(ctx, EventWrite{
			EventKey:     "alert.fired",
			Name:         rule.Name,
			Description:  fmt.Sprintf("error rate %.2f >= %.2f", rate, rule.Threshold),
			Status:       strings.TrimSpace(rule.Severity),
			MetadataJSON: string(meta),
		})
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
}

func (u *Usecase) ShouldFireAlert(rule AlertRule, now time.Time) bool {
	if u == nil {
		return false
	}
	cooldown := rule.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 60
	}
	if v, ok := u.lastFired.Load(rule.ID); ok {
		if last, ok := v.(time.Time); ok && now.Sub(last) < time.Duration(cooldown)*time.Minute {
			return false
		}
	}
	return true
}

func (u *Usecase) MarkAlertFired(ruleID string, now time.Time) {
	if u == nil {
		return
	}
	u.lastFired.Store(ruleID, now)
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
