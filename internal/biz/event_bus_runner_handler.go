package biz

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// runnerCompletionHandler processes runner.completion domain events (monitor, usage, memory worker).
type runnerCompletionHandler struct {
	sessions  *SessionUsecase
	usage     *UsageUsecase
	monitor   *MonitorUsecase
	memWorker *TurnMemoryWorker
	logger    SessionLogWriter
}

func newRunnerCompletionHandler(
	sessions *SessionUsecase,
	usage *UsageUsecase,
	monitor *MonitorUsecase,
	memWorker *TurnMemoryWorker,
) *runnerCompletionHandler {
	return &runnerCompletionHandler{
		sessions:  sessions,
		usage:     usage,
		monitor:   monitor,
		memWorker: memWorker,
	}
}

func (h *runnerCompletionHandler) SetLogger(logger SessionLogWriter) {
	h.logger = logger
}

func runnerUsageRecordingEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHAT_RECORD_RUNNER_USAGE")))
	return raw == "1" || raw == "true" || raw == "yes"
}

func (h *runnerCompletionHandler) Handle(ctx context.Context, de DomainEvent) {
	if h == nil {
		return
	}
	if h.memWorker != nil {
		h.memWorker.OnRunnerCompletion(ctx, de)
	}
	if h.monitor != nil {
		if started, ok := DefaultTurnCompletionBridge().TurnStart(de.SessionID, de.RunID); ok && de.DurationMS == 0 {
			de.DurationMS = CompletionDurationMS(de, started)
		}
		if err := RecordRunnerCompletion(ctx, h.monitor, de); err != nil {
			h.logError(context.Background(), de.SessionID, "event_bus.monitor.persist", "监控事件写入失败", LogPair{Key: "error", Value: err})
		}
		h.monitor.EvaluateAlerts(ctx)
	}
	if de.Usage == nil || h.usage == nil {
		return
	}
	if !runnerUsageRecordingEnabled() {
		return
	}
	now := time.Now().UTC()
	status := "ok"
	if de.Error != nil {
		status = "error"
	}
	_, err := h.usage.RecordTokenUsageEvent(ctx, TokenUsageEvent{
		ID:            uuid.NewString(),
		SessionID:     de.SessionID,
		AgentID:       de.Author,
		UsageKind:     "runner_completion",
		InputTokens:   de.Usage.PromptTokens,
		OutputTokens:  de.Usage.CompletionTokens,
		TotalTokens:   de.Usage.TotalTokens,
		OccurredAt:    now.Format(time.RFC3339),
		DateKey:       now.Format("2006-01-02"),
		HourKey:       now.Format("2006-01-02T15"),
		Status:        status,
		StreamEnabled: true,
		MetadataJSON:  `{"source":"event_bus.runner_completion"}`,
	})
	if err != nil {
		h.logError(context.Background(), de.SessionID, "event_bus.usage.record", "用量事件写入失败", LogPair{Key: "error", Value: err})
	}
}

func (h *runnerCompletionHandler) logError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair) {
	if h.logger != nil {
		h.logger.SessionSysLogError(ctx, sessionID, stepID, message, pairs...)
	}
}
