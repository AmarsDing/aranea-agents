package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// PublishRunStatus emits a run_status envelope for WS subscribers.
func PublishRunStatus(bus event.Bus, sessionID, runID, status, errMsg string) {
	PublishRunStatusMeta(bus, sessionID, runID, status, errMsg, nil)
}

// AwaitStatusMeta is optional metadata for awaiting_user runs.
type AwaitStatusMeta = biz.ChatAwaitMeta

// PublishRunStatusMeta emits run_status with optional await metadata.
func PublishRunStatusMeta(bus event.Bus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	PublishRunStatusFull(bus, sessionID, runID, status, errMsg, await, "", "")
}

// PublishRunStatusFull emits run_status with optional session_run_id and turn_id (CC-R-04).
func PublishRunStatusFull(bus event.Bus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta, sessionRunID, turnID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "run-service", sessionID)
	env.Channel = event.RouteChannel(env)
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
	env.Metadata = meta
	bus.Publish(context.Background(), env)
}

// PublishBackgroundJobRefresh notifies Web clients to reload background job panels (DECO-12 · M55-JOB-01).
func PublishBackgroundJobRefresh(bus event.Bus, sessionID, jobID, status string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "background-job", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Source = "channel"
	env.Metadata = map[string]any{
		"background_job_refresh": true,
		"job_id":                 strings.TrimSpace(jobID),
		"job_status":             strings.TrimSpace(status),
		"status":                 "background_job",
	}
	bus.Publish(context.Background(), env)
}

// CancelSessionRunSideEffects publishes cancelled run_status and marks running activity cards cancelled.
func CancelSessionRunSideEffects(ctx context.Context, bus event.Bus, sessions *biz.SessionUsecase, sessionID, runID string, lg loggateway.Logger) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	PublishRunStatus(bus, sessionID, runID, "cancelled", "")
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, sessions, sessionID, lg); err != nil {
		lg.Warn("取消执行卡片查询失败",
			loggateway.StepID("chat.activity.cancel"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
}

// PublishSessionStatusChanged emits a session.status_changed envelope for WS subscribers.
func PublishSessionStatusChanged(bus event.Bus, sessionID, status, statusReason, statusChangedAt string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeSessionStatusChanged, "session-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"session_id":        sessionID,
		"status":            status,
		"status_reason":     statusReason,
		"status_changed_at": statusChangedAt,
	}
	bus.Publish(context.Background(), env)
}

type sessionStatusPublisher struct {
	bus event.Bus
}

func (p *sessionStatusPublisher) PublishSessionStatusChanged(sessionID, status, statusReason, statusChangedAt string) {
	PublishSessionStatusChanged(p.bus, sessionID, status, statusReason, statusChangedAt)
}

// metricsUpdatedPublisher publishes metrics_updated events via EventBus.
type metricsUpdatedPublisher struct {
	bus event.Bus
}

func (p *metricsUpdatedPublisher) PublishMetricsUpdated(sessionID string) {
	PublishMetricsUpdated(p.bus, sessionID)
}

// PublishMetricsUpdated emits a metrics_updated envelope for WS subscribers.
func PublishMetricsUpdated(bus event.Bus, sessionID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeMetricsUpdated, "session-metrics", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"session_id": sessionID,
	}
	bus.Publish(context.Background(), env)
}

// WireSessionStatusPublisher injects the service-layer WS publisher into SessionUsecase.
func WireSessionStatusPublisher(uc *biz.SessionUsecase, teamUC *biz.TeamUsecase, orchestrator biz.TaskOrchestratorPort, infra *event.Infra, lg loggateway.Logger) *SessionStatusGuard {
	if uc != nil && infra != nil {
		uc.SetStatusPublisher(&sessionStatusPublisher{bus: infra.SessionBus})
		uc.SetMetricsUpdatedPublisher(&metricsUpdatedPublisher{bus: infra.SessionBus})
	}
	return NewSessionStatusGuard(uc, teamUC, orchestrator, lg)
}
