package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const runnerCompletionSchemaV1 = "runner.completion/v1"

type turnPendingUsage struct {
	UsageEventID string
	TraceID      string
}

// TurnCompletionBridge links runner.completion monitor rows with chat turn usage rows.
type TurnCompletionBridge struct {
	mu           sync.Mutex
	turnStarts   map[string]time.Time
	pendingUsage map[string]turnPendingUsage
}

var defaultTurnCompletionBridge = &TurnCompletionBridge{}

// DefaultTurnCompletionBridge returns the process-wide correlation bridge.
func DefaultTurnCompletionBridge() *TurnCompletionBridge {
	return defaultTurnCompletionBridge
}

func turnBridgeKey(sessionID, runID string) string {
	return strings.TrimSpace(sessionID) + "|" + strings.TrimSpace(runID)
}

// RegisterTurnStart records turn wall-clock start for duration_ms on completion.
func (b *TurnCompletionBridge) RegisterTurnStart(sessionID, runID string, startedAt time.Time) {
	if b == nil || startedAt.IsZero() {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return
	}
	key := turnBridgeKey(sessionID, runID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.turnStarts == nil {
		b.turnStarts = make(map[string]time.Time)
	}
	b.turnStarts[key] = startedAt
}

// TurnStart returns the registered start time for a turn.
func (b *TurnCompletionBridge) TurnStart(sessionID, runID string) (time.Time, bool) {
	if b == nil {
		return time.Time{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.turnStarts == nil {
		return time.Time{}, false
	}
	t, ok := b.turnStarts[turnBridgeKey(sessionID, runID)]
	return t, ok
}

// RegisterTurnUsage records usage correlation before completion row exists (usage-before-completion race).
func (b *TurnCompletionBridge) RegisterTurnUsage(sessionID, runID, usageEventID, traceID, agentID, agentKey string) {
	if b == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	usageEventID = strings.TrimSpace(usageEventID)
	if sessionID == "" || runID == "" || usageEventID == "" {
		return
	}
	_ = agentID
	_ = agentKey
	key := turnBridgeKey(sessionID, runID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pendingUsage == nil {
		b.pendingUsage = make(map[string]turnPendingUsage)
	}
	b.pendingUsage[key] = turnPendingUsage{
		UsageEventID: usageEventID,
		TraceID:      strings.TrimSpace(traceID),
	}
}

// PendingUsage returns staged usage correlation for a turn.
func (b *TurnCompletionBridge) PendingUsage(sessionID, runID string) (usageEventID, traceID string, ok bool) {
	if b == nil {
		return "", "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pendingUsage == nil {
		return "", "", false
	}
	p, ok := b.pendingUsage[turnBridgeKey(sessionID, runID)]
	if !ok || strings.TrimSpace(p.UsageEventID) == "" {
		return "", "", false
	}
	return p.UsageEventID, p.TraceID, true
}

// ClearTurn releases per-turn bridge state after correlation is persisted.
func (b *TurnCompletionBridge) ClearTurn(sessionID, runID string) {
	if b == nil {
		return
	}
	key := turnBridgeKey(sessionID, runID)
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.turnStarts, key)
	delete(b.pendingUsage, key)
}

func enrichRunnerCompletionFromBridge(de *DomainEvent) {
	if de == nil {
		return
	}
	usageID, traceID, ok := DefaultTurnCompletionBridge().PendingUsage(de.SessionID, de.RunID)
	if !ok {
		return
	}
	if strings.TrimSpace(de.UsageEventID) == "" {
		de.UsageEventID = usageID
	}
	if strings.TrimSpace(de.TraceID) == "" {
		de.TraceID = traceID
	}
}

// RunnerCompletionLabels are human-readable monitor row fields.
type RunnerCompletionLabels struct {
	Name        string
	Description string
}

// RunnerCompletionLabelsFor builds display name/description from domain event + status.
func RunnerCompletionLabelsFor(de DomainEvent, status string) RunnerCompletionLabels {
	if strings.TrimSpace(status) == "error" {
		return RunnerCompletionLabels{
			Name:        "对话失败",
			Description: runnerCompletionDescription(de, status),
		}
	}
	return RunnerCompletionLabels{
		Name:        "对话完成",
		Description: runnerCompletionDescription(de, status),
	}
}

func runnerCompletionDescription(de DomainEvent, status string) string {
	parts := []string{}
	if name := strings.TrimSpace(de.AgentDisplayName); name != "" {
		parts = append(parts, "Agent "+name)
	} else if key := strings.TrimSpace(de.Author); key != "" {
		parts = append(parts, "Agent "+key)
	}
	if de.DurationMS > 0 {
		parts = append(parts, formatDurationMS(de.DurationMS))
	}
	if de.Usage != nil && de.Usage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", de.Usage.TotalTokens))
	}
	if sid := shortID(de.SessionID); sid != "" {
		parts = append(parts, "会话 "+sid)
	}
	if len(parts) == 0 {
		if status == "error" {
			return "运行失败"
		}
		return "运行完成"
	}
	return strings.Join(parts, " · ")
}

func formatDurationMS(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	sec := float64(ms) / 1000
	if sec < 60 {
		return fmt.Sprintf("%.1f s", sec)
	}
	return fmt.Sprintf("%.1f min", sec/60)
}

func shortID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

// BuildRunnerCompletionMetadataJSON returns metadata_json for monitor_events.
func BuildRunnerCompletionMetadataJSON(de DomainEvent, status string) string {
	meta := map[string]any{
		"schema_version": runnerCompletionSchemaV1,
		"session_id":     strings.TrimSpace(de.SessionID),
		"status":         strings.TrimSpace(status),
		"run_kind":       strings.TrimSpace(de.RunKind),
	}
	if de.RunKind == "" {
		meta["run_kind"] = inferRunKind(de)
	}
	if v := strings.TrimSpace(de.TeamID); v != "" {
		meta["team_id"] = v
	}
	if v := strings.TrimSpace(de.Author); v != "" {
		meta["agent_key"] = v
	}
	if v := strings.TrimSpace(de.AgentID); v != "" {
		meta["agent_id"] = v
	}
	if v := strings.TrimSpace(de.AgentDisplayName); v != "" {
		meta["agent_display_name"] = v
	}
	if v := strings.TrimSpace(de.RunID); v != "" {
		meta["run_id"] = v
	}
	if v := strings.TrimSpace(de.TraceID); v != "" {
		meta["trace_id"] = v
	}
	if v := strings.TrimSpace(de.RequestID); v != "" {
		meta["request_id"] = v
	}
	if v := strings.TrimSpace(de.InvocationID); v != "" {
		meta["invocation_id"] = v
	}
	if v := strings.TrimSpace(de.UsageEventID); v != "" {
		meta["usage_event_id"] = v
	}
	if de.DurationMS > 0 {
		meta["duration_ms"] = de.DurationMS
	}
	if de.Usage != nil {
		meta["usage"] = map[string]any{
			"prompt_tokens":     de.Usage.PromptTokens,
			"completion_tokens": de.Usage.CompletionTokens,
			"total_tokens":      de.Usage.TotalTokens,
		}
	}
	if de.Error != nil {
		errObj := map[string]any{"message": strings.TrimSpace(de.Error.Message)}
		if t := strings.TrimSpace(de.Error.Type); t != "" {
			errObj["type"] = t
		}
		meta["error"] = errObj
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func inferRunKind(de DomainEvent) string {
	if strings.TrimSpace(de.TeamID) != "" {
		return "team"
	}
	return "chat"
}

// CompletionCorrelationKey returns the idempotency key parts.
func CompletionCorrelationKey(de DomainEvent) (sessionID, invocationID string) {
	sessionID = strings.TrimSpace(de.SessionID)
	invocationID = strings.TrimSpace(de.InvocationID)
	if invocationID == "" {
		invocationID = strings.TrimSpace(de.RunID)
	}
	return sessionID, invocationID
}

// MergeRunnerCompletionUsagePatch builds metadata patch after usage is recorded.
func MergeRunnerCompletionUsagePatch(usageEventID, traceID string) string {
	patch := map[string]any{"schema_version": runnerCompletionSchemaV1}
	if v := strings.TrimSpace(usageEventID); v != "" {
		patch["usage_event_id"] = v
	}
	if v := strings.TrimSpace(traceID); v != "" {
		patch["trace_id"] = v
	}
	raw, _ := json.Marshal(patch)
	return string(raw)
}

// RecordRunnerCompletion persists a runner.completion row with idempotency and usage correlation.
func (u *MonitorUsecase) RecordRunnerCompletion(ctx context.Context, de DomainEvent) error {
	if u == nil || u.repo == nil {
		return nil
	}
	enrichRunnerCompletionFromBridge(&de)
	status := "ok"
	if de.Error != nil {
		status = "error"
	}
	sessionID, invocationID := CompletionCorrelationKey(de)
	runID := strings.TrimSpace(de.RunID)
	if runID == "" {
		runID = invocationID
	}

	if sessionID != "" && invocationID != "" {
		exists, err := u.repo.ExistsRunnerCompletion(ctx, sessionID, invocationID)
		if err != nil {
			return err
		}
		if exists {
			patched, err := u.patchRunnerCompletionLink(ctx, sessionID, runID, invocationID, de.UsageEventID, de.TraceID)
			if err != nil {
				return err
			}
			if patched || strings.TrimSpace(de.UsageEventID) != "" {
				DefaultTurnCompletionBridge().ClearTurn(sessionID, runID)
			}
			return nil
		}
	}
	labels := RunnerCompletionLabelsFor(de, status)
	write := MonitorEventWrite{
		EventKey:     "runner.completion",
		Name:         labels.Name,
		Description:  labels.Description,
		Status:       status,
		MetadataJSON: BuildRunnerCompletionMetadataJSON(de, status),
	}
	if err := u.repo.InsertMonitorEvent(ctx, write); err != nil {
		return err
	}
	patched, err := u.patchRunnerCompletionLink(ctx, sessionID, runID, invocationID, de.UsageEventID, de.TraceID)
	if err != nil {
		return err
	}
	if patched || strings.TrimSpace(de.UsageEventID) != "" {
		DefaultTurnCompletionBridge().ClearTurn(sessionID, runID)
	}
	return nil
}

// LinkRunnerCompletionUsage patches the latest completion row for session+run with usage_event_id.
func (u *MonitorUsecase) LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string) error {
	if u == nil || u.repo == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	usageEventID = strings.TrimSpace(usageEventID)
	if sessionID == "" || runID == "" || usageEventID == "" {
		return nil
	}
	DefaultTurnCompletionBridge().RegisterTurnUsage(sessionID, runID, usageEventID, traceID, "", "")
	patched, err := u.patchRunnerCompletionLink(ctx, sessionID, runID, runID, usageEventID, traceID)
	if err != nil {
		return err
	}
	if patched {
		DefaultTurnCompletionBridge().ClearTurn(sessionID, runID)
	}
	return nil
}

func (u *MonitorUsecase) patchRunnerCompletionLink(ctx context.Context, sessionID, runID, invocationID, usageEventID, traceID string) (bool, error) {
	usageEventID = strings.TrimSpace(usageEventID)
	traceID = strings.TrimSpace(traceID)
	if usageEventID == "" {
		if u2, t2, ok := DefaultTurnCompletionBridge().PendingUsage(sessionID, runID); ok {
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

// CompletionDurationMS computes wall-clock duration when Timestamp is set on the domain event.
func CompletionDurationMS(de DomainEvent, startedAt time.Time) int64 {
	if de.Timestamp.IsZero() || startedAt.IsZero() {
		return 0
	}
	d := de.Timestamp.Sub(startedAt)
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}
