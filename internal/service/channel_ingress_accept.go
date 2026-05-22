package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

const flowStepChannelInboundAccept = "channel.inbound.accept"

// inboundAcceptOutcome tells the caller what background work to schedule after accept.
type inboundAcceptOutcome struct {
	ExecuteSync   bool
	DispatchAsync bool
}

func (o inboundAcceptOutcome) needsBackgroundWork() bool {
	return o.ExecuteSync || o.DispatchAsync
}

// acceptInbound validates idempotency/access and sends the configured ACK message.
// When ExecuteSync or DispatchAsync is set, the caller must run background work and release inflight.
func (h *ChannelIngress) acceptInbound(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, viaWebhook bool) (inboundAcceptOutcome, error) {
	var noop inboundAcceptOutcome
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	dedupKey := biz.InboundIdempotencyKey(platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	viaLabel := "runtime"
	if viaWebhook {
		viaLabel = "webhook"
	}
	ok, skipReason, err := h.shouldProcessInbound(ctx, chRow, ev, viaWebhook)
	if err != nil {
		h.inboundInflight.release(dedupKey)
		return noop, err
	}
	if !ok {
		h.inboundInflight.release(dedupKey)
		_ = h.recordDelivery(ctx, chRow.ID, "skipped_"+skipReason, map[string]any{
			"peer_id":         ev.PeerID,
			"idempotency_key": ev.IdempotencyKey,
			"ingress_source":  strings.TrimSpace(ev.OutboundMeta["ingress_source"]),
			"via":             viaLabel,
			"text_preview":    truncateForLog(ev.Text, 80),
		}, "")
		return noop, nil
	}
	h.logInboundAccepted(ctx, chRow, ev, viaLabel)
	allowed, reason, err := h.checkInboundAccess(ctx, chRow, ev)
	if err != nil {
		h.inboundInflight.release(dedupKey)
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "access", "error": err.Error()}, err.Error())
		return noop, err
	}
	if !allowed {
		h.inboundInflight.release(dedupKey)
		return noop, h.rejectInboundAccess(ctx, chRow, ev, reason)
	}
	if handled, cerr := h.tryCancelInboundTurn(ctx, chRow, ev, platform); handled || cerr != nil {
		h.inboundInflight.release(dedupKey)
		return noop, cerr
	}
	ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
	if !biz.ChannelSupportsLongTaskIngress(platform, chRow.ConfigJSON) {
		ltCfg.AckMessage = ""
		ltCfg.ExecutionMode = "sync"
		ltCfg.AsyncGraphID = ""
		ltCfg.AsyncCronTaskID = ""
	}
	if ltCfg.ShouldRunAsync(ev.Text) {
		if !isPureAsyncExecutionMode(ltCfg) {
			if err := h.sendInboundAckIfNeeded(ctx, chRow, ev, platform, ltCfg); err != nil {
				h.inboundInflight.release(dedupKey)
				_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "ack", "error": err.Error()}, err.Error())
				return noop, err
			}
		}
		event.SysLogInfo(flowStepChannelInboundAccept, "Channel 入站 ACK 已发送",
			event.P("channel_id", chRow.ID),
			event.P("peer_id", ev.PeerID),
			event.P("async", true),
		)
		return inboundAcceptOutcome{DispatchAsync: true}, nil
	}
	if err := h.sendInboundAckIfNeeded(ctx, chRow, ev, platform, ltCfg); err != nil {
		h.inboundInflight.release(dedupKey)
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "ack", "error": err.Error()}, err.Error())
		return noop, err
	}
	event.SysLogInfo(flowStepChannelInboundAccept, "Channel 入站 ACK 已发送",
		event.P("channel_id", chRow.ID),
		event.P("peer_id", ev.PeerID),
	)
	return inboundAcceptOutcome{ExecuteSync: true}, nil
}

func isPureAsyncExecutionMode(ltCfg biz.ChannelLongTaskConfig) bool {
	return strings.EqualFold(strings.TrimSpace(ltCfg.ExecutionMode), "async")
}

func (h *ChannelIngress) releaseInboundInflight(ev port.InboundEvent, platform string) {
	if h == nil {
		return
	}
	if platform == "" {
		platform = "unknown"
	}
	dedupKey := biz.InboundIdempotencyKey(platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	h.inboundInflight.release(dedupKey)
}

func (h *ChannelIngress) sendInboundAckIfNeeded(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig) error {
	if biz.ChannelACKDeferredToPreview(chRow.ConfigJSON, platform) {
		return nil
	}
	return h.sendInboundAck(ctx, chRow, ev, platform, ltCfg)
}

func (h *ChannelIngress) sendInboundAck(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig) error {
	text := strings.TrimSpace(ltCfg.AckMessage)
	if text == "" {
		return nil
	}
	recipient := outboundRecipient(ev)
	idempotency := ackIdempotencyKey(platform, ev, "ack")
	return h.enqueueOutboundReply(ctx, chRow, platform, recipient, text, ev.OutboundMeta, idempotency)
}

func (h *ChannelIngress) sendInboundQueuedAck(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, pendingID string) error {
	text := strings.TrimSpace(ltCfg.AckOnQueued)
	if text == "" {
		return nil
	}
	text = biz.RenderChannelTemplate(text, map[string]string{"pending_id": pendingID})
	recipient := outboundRecipient(ev)
	idempotency := ackIdempotencyKey(platform, ev, "queued")
	return h.enqueueOutboundReply(ctx, chRow, platform, recipient, text, ev.OutboundMeta, idempotency)
}

func outboundRecipient(ev port.InboundEvent) string {
	if r := strings.TrimSpace(ev.OutboundMeta["recipient"]); r != "" {
		return r
	}
	return ev.PeerID
}

func ackIdempotencyKey(platform string, ev port.InboundEvent, suffix string) string {
	base := strings.TrimSpace(ev.IdempotencyKey)
	if base == "" {
		base = platform + ":" + ev.PeerID
	}
	return base + ":" + suffix
}

func inboundPlatform(chRow biz.Channel, ev port.InboundEvent) string {
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	return platform
}
