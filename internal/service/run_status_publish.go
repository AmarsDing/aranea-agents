package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/pkg/loggateway"
)

// PublishRunStatus emits a run_status v2 RunStatusEvent for WS subscribers.
func PublishRunStatus(bus biz.EventBus, sessionID, runID, status, errMsg string) {
	PublishRunStatusMeta(bus, sessionID, runID, status, errMsg, nil)
}

// AwaitStatusMeta is optional metadata for awaiting_user runs.
type AwaitStatusMeta = biz.ChatAwaitMeta

// PublishRunStatusMeta emits run_status with optional await metadata.
func PublishRunStatusMeta(bus biz.EventBus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	PublishRunStatusFull(bus, sessionID, runID, status, errMsg, await, "", "")
}

// PublishRunStatusFull emits run_status with optional session_run_id and turn_id (CC-R-04).
func PublishRunStatusFull(bus biz.EventBus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta, sessionRunID, turnID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	meta := map[string]any{
		"run_id":        runID,
		"status":        status,
		"error_message": errMsg,
	}
	if sr := strings.TrimSpace(sessionRunID); sr != "" {
		meta["session_run_id"] = sr
	}
	if tid := strings.TrimSpace(turnID); tid != "" {
		meta["turn_id"] = tid
	}
	if await != nil {
		if k := strings.TrimSpace(await.Kind); k != "" {
			meta["await_kind"] = k
		}
		if k := strings.TrimSpace(await.ToolKey); k != "" {
			meta["await_tool_key"] = k
		}
		if k := strings.TrimSpace(await.ToolCallID); k != "" {
			meta["await_tool_call_id"] = k
		}
	}
	noticeType, _ := runStatusNoticeTypeAndLabel(status)
	meta["notice_type"] = noticeType
	bus.Publish(context.Background(), biz.NewRunStatusEvent(sessionID, runID, status, meta))
}

// mapRunStatusToActivityStatus maps a raw run-status string to the closest
// ActivityStatus. Unknown/non-terminal values default to Running.
func mapRunStatusToActivityStatus(status string) biz.ActivityStatus {
	switch status {
	case "running", "streaming", "awaiting_user":
		return biz.ActivityStatusRunning
	case "paused":
		return biz.ActivityStatusPaused
	case "completed", "succeeded":
		return biz.ActivityStatusCompleted
	case "failed", "error":
		return biz.ActivityStatusFailed
	case "cancelled", "canceled":
		return biz.ActivityStatusCancelled
	default:
		return biz.ActivityStatusRunning
	}
}

// runStatusNoticeTypeAndLabel maps a raw run-status string to a frontend
// notice type (info/success/warning) and a human-readable Chinese label.
// Used for run_status and session_status_changed notice events so the chat
// UI renders them as NoticeBlock instead of empty AgentCards.
func runStatusNoticeTypeAndLabel(status string) (noticeType, label string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return "info", "运行中"
	case "streaming":
		return "info", "流式输出中"
	case "awaiting_user":
		return "info", "等待用户输入"
	case "paused":
		return "info", "已暂停"
	case "completed", "succeeded":
		return "success", "已完成"
	case "failed", "error":
		return "warning", "失败"
	case "cancelled", "canceled":
		return "warning", "已取消"
	default:
		return "info", status
	}
}

// PublishBackgroundJobRefresh notifies Web clients to reload background job panels (DECO-12 · M55-JOB-01).
func PublishBackgroundJobRefresh(bus biz.EventBus, sessionID, jobID, status string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	meta := map[string]any{
		"background_job_refresh": true,
		"job_id":                 strings.TrimSpace(jobID),
		"job_status":             strings.TrimSpace(status),
		"status":                 "background_job",
	}
	bus.Publish(context.Background(), biz.NewSystemNoticeEvent(sessionID, "background_job_refresh", "", meta))
}

// CancelSessionRunSideEffects publishes cancelled run_status and marks running activity cards cancelled.
func CancelSessionRunSideEffects(ctx context.Context, bus biz.EventBus, stepReader biz.StepV2Reader, writer biz.StepV2Writer, sessionID, runID string, lg loggateway.Logger) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	PublishRunStatus(bus, sessionID, runID, biz.SessionRunPhaseCancelled, "")
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, stepReader, writer, sessionID, lg); err != nil {
		lg.Warn("取消执行卡片查询失败",
			loggateway.StepID("chat.activity.cancel"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
}

// PublishSessionStatusChanged emits a session.status_changed v2 SystemNoticeEvent for WS subscribers.
func PublishSessionStatusChanged(bus biz.EventBus, sessionID, status, statusReason, statusChangedAt string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	noticeType, statusLabel := runStatusNoticeTypeAndLabel(status)
	meta := map[string]any{
		"session_id":        sessionID,
		"status":            status,
		"status_reason":     statusReason,
		"status_changed_at": statusChangedAt,
		"notice_type":       noticeType,
	}
	bus.Publish(context.Background(), biz.NewSystemNoticeEvent(sessionID, "session_status_changed", "会话状态："+statusLabel, meta))
}

type sessionStatusPublisher struct {
	bus biz.EventBus
}

func (p *sessionStatusPublisher) PublishSessionStatusChanged(sessionID, status, statusReason, statusChangedAt string) {
	PublishSessionStatusChanged(p.bus, sessionID, status, statusReason, statusChangedAt)
}

// metricsUpdatedPublisher publishes metrics_updated events via the v2 EventBus.
type metricsUpdatedPublisher struct {
	bus biz.EventBus
}

func (p *metricsUpdatedPublisher) PublishMetricsUpdated(sessionID string) {
	PublishMetricsUpdated(p.bus, sessionID)
}

// PublishMetricsUpdated emits a metrics_updated v2 SystemNoticeEvent for WS subscribers.
func PublishMetricsUpdated(bus biz.EventBus, sessionID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	meta := map[string]any{
		"session_id": sessionID,
		"stage":      "metrics_updated",
	}
	bus.Publish(context.Background(), biz.NewSystemNoticeEvent(sessionID, "metrics_updated", "会话指标", meta))
}

// ProvideSessionStatusPublisher creates a SessionStatusPublisher backed by the v2 EventBus.
func ProvideSessionStatusPublisher(bus biz.EventBus) biz.SessionStatusPublisher {
	if bus == nil {
		return nil
	}
	return &sessionStatusPublisher{bus: bus}
}

// ProvideMetricsUpdatedPublisher creates a MetricsUpdatedPublisher backed by the v2 EventBus.
func ProvideMetricsUpdatedPublisher(bus biz.EventBus) biz.MetricsUpdatedPublisher {
	if bus == nil {
		return nil
	}
	return &metricsUpdatedPublisher{bus: bus}
}
