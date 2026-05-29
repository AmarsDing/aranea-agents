package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

type channelTurnJobContextKey struct{}

type channelTurnJobCtx struct {
	jobID     string
	sessionID string
}

func withChannelTurnJob(ctx context.Context, jobID, sessionID string) context.Context {
	jobID = strings.TrimSpace(jobID)
	sessionID = strings.TrimSpace(sessionID)
	if jobID == "" && sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, channelTurnJobContextKey{}, channelTurnJobCtx{
		jobID:     jobID,
		sessionID: sessionID,
	})
}

func withChannelTurnJobID(ctx context.Context, jobID string) context.Context {
	_, sessionID := channelTurnJobFromContext(ctx)
	return withChannelTurnJob(ctx, jobID, sessionID)
}

func channelTurnJobFromContext(ctx context.Context) (jobID, sessionID string) {
	if ctx == nil {
		return "", ""
	}
	v, _ := ctx.Value(channelTurnJobContextKey{}).(channelTurnJobCtx)
	return strings.TrimSpace(v.jobID), strings.TrimSpace(v.sessionID)
}

func channelTurnJobIDFromContext(ctx context.Context) string {
	jobID, _ := channelTurnJobFromContext(ctx)
	return jobID
}

func (h *ChannelIngress) createTurnJob(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (string, context.Context, error) {
	if h == nil || h.turnJobs == nil {
		return "", ctx, nil
	}
	idempotency := strings.TrimSpace(ev.IdempotencyKey)
	if idempotency == "" {
		idempotency = biz.InboundIdempotencyKey(platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	}
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		peerKey = strings.TrimSpace(ev.PeerKey)
	}
	sessionID := ""
	if h.channels != nil && h.sessions != nil {
		if input, perr := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text, false); perr == nil {
			sessionID = strings.TrimSpace(input.SessionID)
		}
	}
	now := biz.ChannelTurnJobNow()
	jobID, err := h.turnJobs.CreateAccepted(ctx, biz.ChannelTurnJob{
		ID:             biz.NewChannelTurnJobID(),
		ChannelID:      chRow.ID,
		SessionID:      sessionID,
		PeerID:         ev.PeerID,
		PeerKey:        peerKey,
		IdempotencyKey: idempotency,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return "", ctx, err
	}
	ctx = withChannelTurnJob(ctx, jobID, sessionID)
	h.publishBackgroundJobRefresh(ctx, jobID, sessionID, biz.ChannelTurnJobStatusAccepted)
	return jobID, ctx, nil
}

func (h *ChannelIngress) markTurnJob(ctx context.Context, status, errMsg, previewID, preview string) {
	if h == nil || h.turnJobs == nil {
		return
	}
	jobID, sessionID := channelTurnJobFromContext(ctx)
	if jobID == "" {
		return
	}
	if err := h.turnJobs.UpdateStatus(ctx, jobID, status, errMsg, previewID, preview); err != nil {
		event.SysLogWarn("channel.job.status_update_failed", "TurnJob 状态更新失败",
			event.P("job_id", jobID),
			event.P("status", status),
			event.P("error", err.Error()),
		)
	}
	h.publishBackgroundJobRefresh(ctx, jobID, sessionID, status)
}

func (h *ChannelIngress) markTurnJobAsyncTarget(ctx context.Context, targetType, targetID string) {
	if h == nil || h.turnJobs == nil {
		return
	}
	jobID := channelTurnJobIDFromContext(ctx)
	if jobID == "" {
		return
	}
	_ = h.turnJobs.UpdateAsyncTarget(ctx, jobID, targetType, targetID)
}

func (h *ChannelIngress) publishBackgroundJobRefresh(ctx context.Context, jobID, sessionID, status string) {
	if h == nil || h.eventBus == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		_, sessionID = channelTurnJobFromContext(ctx)
	}
	PublishBackgroundJobRefresh(h.eventBus, sessionID, jobID, status)
}
