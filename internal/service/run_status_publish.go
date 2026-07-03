package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// PublishRunStatus emits a run_status ActivityEvent for WS subscribers.
func PublishRunStatus(bus biz.ActivityEventBus, sessionID, runID, status, errMsg string) {
	PublishRunStatusMeta(bus, sessionID, runID, status, errMsg, nil)
}

// AwaitStatusMeta is optional metadata for awaiting_user runs.
type AwaitStatusMeta = biz.ChatAwaitMeta

// PublishRunStatusMeta emits run_status with optional await metadata.
func PublishRunStatusMeta(bus biz.ActivityEventBus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	PublishRunStatusFull(bus, sessionID, runID, status, errMsg, await, "", "")
}

// PublishRunStatusFull emits run_status with optional session_run_id and turn_id (CC-R-04).
func PublishRunStatusFull(bus biz.ActivityEventBus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta, sessionRunID, turnID string) {
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
	noticeType, statusLabel := runStatusNoticeTypeAndLabel(status)
	meta["notice_type"] = noticeType
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindNotice,
			Status:    mapRunStatusToActivityStatus(status),
			Timestamp: time.Now().UTC(),
			SessionID: sessionID,
			AgentKey:  "run-service",
			AgentName: "运行状态",
			Stage:     "run_status",
			Content:   "运行状态：" + statusLabel,
			Meta:      meta,
		},
		Domain: biz.ActivityDomainChat,
	}
	bus.Publish(context.Background(), ev)
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
func PublishBackgroundJobRefresh(bus biz.ActivityEventBus, sessionID, jobID, status string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindNotice,
			Status:    biz.ActivityStatusRunning,
			Timestamp: time.Now().UTC(),
			SessionID: sessionID,
			AgentKey:  "background-job",
			Stage:     "background_job_refresh",
			Meta: map[string]any{
				"background_job_refresh": true,
				"job_id":                 strings.TrimSpace(jobID),
				"job_status":             strings.TrimSpace(status),
				"status":                 "background_job",
			},
		},
		Domain: biz.ActivityDomainSystem,
	}
	bus.Publish(context.Background(), ev)
}

// CancelSessionRunSideEffects publishes cancelled run_status and marks running activity cards cancelled.
func CancelSessionRunSideEffects(ctx context.Context, bus biz.ActivityEventBus, stepReader biz.StepV2Reader, writer biz.ActivityWriter, sessionID, runID string, lg loggateway.Logger) {
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

// PublishSessionStatusChanged emits a session.status_changed ActivityEvent for WS subscribers.
func PublishSessionStatusChanged(bus biz.ActivityEventBus, sessionID, status, statusReason, statusChangedAt string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	noticeType, statusLabel := runStatusNoticeTypeAndLabel(status)
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindNotice,
			Status:    mapRunStatusToActivityStatus(status),
			Timestamp: time.Now().UTC(),
			SessionID: sessionID,
			AgentKey:  "session-service",
			AgentName: "会话状态",
			Stage:     "session_status_changed",
			Content:   "会话状态：" + statusLabel,
			Meta: map[string]any{
				"session_id":        sessionID,
				"status":            status,
				"status_reason":     statusReason,
				"status_changed_at": statusChangedAt,
				"notice_type":       noticeType,
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	bus.Publish(context.Background(), ev)
}

type sessionStatusPublisher struct {
	bus biz.ActivityEventBus
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

// ProvideSessionStatusPublisher creates a SessionStatusPublisher backed by ActivityEventBus.
func ProvideSessionStatusPublisher(bus biz.ActivityEventBus) biz.SessionStatusPublisher {
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
