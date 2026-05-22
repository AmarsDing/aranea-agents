package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

type channelTurnJobContextKey struct{}

func withChannelTurnJobID(ctx context.Context, jobID string) context.Context {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ctx
	}
	return context.WithValue(ctx, channelTurnJobContextKey{}, jobID)
}

func channelTurnJobIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(channelTurnJobContextKey{}).(string)
	return strings.TrimSpace(id)
}

func (h *ChannelIngress) createTurnJob(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (string, context.Context, error) {
	if h == nil || h.turnJobs == nil {
		return "", ctx, nil
	}
	idempotency := strings.TrimSpace(ev.IdempotencyKey)
	if idempotency == "" {
		idempotency = biz.InboundIdempotencyKey(platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	}
	peerKey := ev.PeerKey
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err == nil && peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	sessionID := ""
	if h.peers != nil && h.sessions != nil {
		if req, perr := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text); perr == nil {
			sessionID = strings.TrimSpace(req.GetSessionId())
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
	return jobID, withChannelTurnJobID(ctx, jobID), nil
}

func (h *ChannelIngress) markTurnJob(ctx context.Context, status, errMsg, previewID, preview string) {
	if h == nil || h.turnJobs == nil {
		return
	}
	jobID := channelTurnJobIDFromContext(ctx)
	if jobID == "" {
		return
	}
	_ = h.turnJobs.UpdateStatus(ctx, jobID, status, errMsg, previewID, preview)
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
