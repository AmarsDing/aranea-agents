package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	arametrics "aranea-agents/internal/metrics"
)

const (
	channelStatusReplyNoRun  = "当前没有进行中的任务。"
	channelStatusReplyActive = "任务进行中…"
)

func recordIngressIntentMetric(intent string) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return
	}
	arametrics.ChannelBusyIntentTotal.WithLabelValues(intent).Inc()
}

// applyPreTurnIngressPolicy handles busy-line intents before starting a sync Channel turn (DECO-09).
func (h *ChannelIngress) applyPreTurnIngressPolicy(
	ctx context.Context,
	chRow biz.Channel,
	ev port.InboundEvent,
	platform, sessionID string,
	ltCfg biz.ChannelLongTaskConfig,
) (handled bool, err error) {
	if h == nil || h.chat == nil {
		return false, nil
	}
	hasActive := h.chat.HasActiveRun(sessionID)
	hasRunner := h.channelHasActiveRunner(sessionID)
	allowQueue := channelAllowQueueFromConfig(chRow.ConfigJSON)
	contextPressure := h.sessionContextPressure(ctx, sessionID, ltCfg)
	policy := EvaluateIngressPolicy(channelIngressPolicyInput(ev.Text, ltCfg, allowQueue, hasActive, hasRunner, contextPressure))
	recordIngressIntentMetric(policy.Intent)

	switch policy.Decision {
	case IngressRejectBusy:
		if policy.Intent == "context_pressure" {
			idempotency := ackIdempotencyKey(platform, ev, "context_pressure")
			return true, h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), channelTurnErrorContextOverflow, ev.OutboundMeta, idempotency)
		}
		return false, nil
	case IngressStatus:
		reply := channelStatusReplyNoRun
		if hasActive {
			phase := strings.TrimSpace(h.chat.ActiveSessionRunPhase(ctx, sessionID))
			if phase != "" {
				reply = fmt.Sprintf("当前任务阶段：%s", phase)
			} else {
				reply = channelStatusReplyActive
			}
		}
		idempotency := ackIdempotencyKey(platform, ev, "status")
		return true, h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), reply, ev.OutboundMeta, idempotency)
	case IngressSteer:
		pendingID, err := h.steerIntoActiveTurn(ctx, chRow, ev, platform, sessionID, ltCfg)
		if err != nil {
			return true, err
		}
		_ = h.recordDelivery(ctx, chRow.ID, "steered", map[string]any{
			"peer_id":    ev.PeerID,
			"session_id": sessionID,
			"pending_id": pendingID,
		}, "")
		return true, nil
	default:
		return false, nil
	}
}

func (h *ChannelIngress) steerIntoActiveTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, sessionID string, ltCfg biz.ChannelLongTaskConfig) (string, error) {
	if h == nil || h.chat == nil || sessionID == "" {
		return "", nil
	}
	content := strings.TrimSpace(ev.Text)
	if content == "" {
		return "", nil
	}
	accepted, err := h.chat.TryEnqueueUserMessage(sessionID, content)
	if err != nil {
		return "", err
	}
	if !accepted {
		return "", fmt.Errorf("steer rejected")
	}
	pendingID := h.chat.LastPendingMessageID(sessionID)
	if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, pendingID); err != nil {
		return pendingID, err
	}
	return pendingID, nil
}

func (h *ChannelIngress) channelHasActiveRunner(sessionID string) bool {
	return h.chat.HasActiveRun(sessionID)
}
