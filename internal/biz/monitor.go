package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID          string
	Action      string
	Resource    string
	ResourceID  string
	RequestID   string
	Detail      string
	CreatedAt   string
	Actor       string
	IP          string
	UserAgent   string
	Severity    string
	MetadataJSON string
}

type AuditQuery struct {
	Limit    int32
	Offset   int32
	Action   string
	Resource string
	Actor    string
	Keyword  string
}

type AuditListResult struct {
	Items []AuditLog
	Total int32
}

type MonitorPlatformRow struct {
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

type MonitorEventsQuery struct {
	Limit     int32
	Offset    int32
	EventType string
	AgentID   string
	Status    string
}

type MonitorTracesQuery struct {
	Limit    int32
	Offset   int32
	AgentID  string
	Provider string
	Model    string
	Status   string
}

type MonitorListResult struct {
	Items []MonitorPlatformRow
	Total int32
}

type MonitorEventWrite struct {
	EventKey     string
	Name         string
	Description  string
	Status       string
	MetadataJSON string
}

// MonitorAlertRule defines a simple threshold alert on monitor_events aggregates.
type MonitorAlertRule struct {
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
	Notify(ctx context.Context, rule MonitorAlertRule, payload map[string]any)
}

type MonitorRepo interface {
	ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error)
	InsertAuditLog(ctx context.Context, entry AuditLog) error
	InsertMonitorEvent(ctx context.Context, ev MonitorEventWrite) error
	ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error)
	GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error)
	ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error)
	GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error)
	ListAlertRules(ctx context.Context) ([]MonitorAlertRule, error)
	ReplaceAlertRules(ctx context.Context, rules []MonitorAlertRule) error
	CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339 string) (int32, error)
	AvgRunnerCompletionDurationMsSince(ctx context.Context, sinceRFC3339 string) (float64, error)
	ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error)
	// PatchRunnerCompletionMetadata returns patched=true when a row was updated.
	PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error)
}

type MonitorUsecase struct {
	repo      MonitorRepo
	notifier  AlertNotifier
	lastFired sync.Map // rule id -> time.Time
}

func NewMonitorUsecase(repo MonitorRepo, notifier AlertNotifier) *MonitorUsecase {
	return &MonitorUsecase{repo: repo, notifier: notifier}
}

func auditLimit(limit int32) int {
	l := int(limit)
	if l <= 0 {
		l = 200
	}
	return l
}

// RecordAuditLog persists an admin audit row (best-effort).
func (u *MonitorUsecase) RecordAuditLog(ctx context.Context, entry AuditLog) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = uuid.NewString()
	}
	return u.repo.InsertAuditLog(ctx, entry)
}

// RecordMonitorEvent persists a monitor_events row (best-effort).
func (u *MonitorUsecase) RecordMonitorEvent(ctx context.Context, ev MonitorEventWrite) error {
	if u == nil || u.repo == nil {
		return nil
	}
	return u.repo.InsertMonitorEvent(ctx, ev)
}

func (u *MonitorUsecase) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	return u.repo.ListAuditLogs(ctx, query)
}

func (u *MonitorUsecase) ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorEvents(ctx, query)
}

func (u *MonitorUsecase) ListAlertRules(ctx context.Context) ([]MonitorAlertRule, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	return u.repo.ListAlertRules(ctx)
}

func (u *MonitorUsecase) ReplaceAlertRules(ctx context.Context, rules []MonitorAlertRule) error {
	if u == nil || u.repo == nil {
		return nil
	}
	return u.repo.ReplaceAlertRules(ctx, rules)
}

// EvaluateAlerts checks enabled rules after runner completion and records alert.fired events.
func (u *MonitorUsecase) EvaluateAlerts(ctx context.Context) {
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
		if !u.shouldFireAlert(rule, now) {
			continue
		}
		u.markAlertFired(rule.ID, now)
		meta, _ := json.Marshal(map[string]any{
			"rule_id": rule.ID, "error_rate": rate, "errors": errors, "total": total, "window_minutes": window,
		})
		_ = u.RecordMonitorEvent(ctx, MonitorEventWrite{
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

func (u *MonitorUsecase) shouldFireAlert(rule MonitorAlertRule, now time.Time) bool {
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

func (u *MonitorUsecase) markAlertFired(ruleID string, now time.Time) {
	if u == nil {
		return
	}
	u.lastFired.Store(ruleID, now)
}

func (u *MonitorUsecase) GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorEvent(ctx, id)
}

func (u *MonitorUsecase) ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorTraces(ctx, query)
}

func (u *MonitorUsecase) GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorTrace(ctx, id)
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

func (u *MonitorUsecase) GetRunnerMetrics(ctx context.Context, windowMinutes int) (RunnerMetricsSummary, error) {
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
