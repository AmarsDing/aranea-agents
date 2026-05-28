package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (h *ChannelIngress) runChatTurnWithOutcome(ctx context.Context, chRow biz.Channel, platform string, ev port.InboundEvent) (biz.ChannelTurnResult, error) {
	var last biz.ChannelTurnResult
	var lastErr error
	for attempt := 0; attempt < channelTurnBusyRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(channelTurnBusyBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return biz.ChannelTurnResult{}, ctx.Err()
			case <-timer.C:
			}
		}
		result, err := h.runChatTurnWithOutcomeOnce(ctx, chRow, platform, ev)
		last, lastErr = result, err
		if err == nil || !IsTurnBusyError(err) {
			return result, err
		}
	}
	return last, lastErr
}

func (h *ChannelIngress) runChatTurnWithOutcomeOnce(ctx context.Context, chRow biz.Channel, platform string, ev port.InboundEvent) (biz.ChannelTurnResult, error) {
	if h == nil || h.chat == nil {
		return biz.ChannelTurnResult{}, nil
	}
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
		return biz.ChannelTurnResult{}, err
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return biz.ChannelTurnResult{}, err
	}
	input := channelChatRequestToTurnInput(req, channelAllowQueueFromConfig(chRow.ConfigJSON))
	sessionID := strings.TrimSpace(input.SessionID)
	h.maybeInterruptActiveTurn(ctx, chRow, sessionID)

	result, err := h.runNativeTurnWithBusyRetry(ctx, chRow, platform, input)
	if err != nil {
		if IsTurnMessageQueued(err) || result.Outcome == biz.NativeTurnOutcomeQueued {
			pendingID := strings.TrimSpace(result.PendingID)
			if pendingID == "" {
				pendingID = h.chat.LastPendingMessageID(sessionID)
			}
			return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeQueued, PendingID: pendingID}, nil
		}
		if IsTurnBusyError(err) {
			return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeFailed}, err
		}
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeFailed}, err
	}
	return h.channelTurnResultFromNative(sessionID, result)
}

// runNativeTurnWithBusyRetry wraps RunNativeTurnWithOutcome with short retries for CHAT_TURN_BUSY.
func (h *ChannelIngress) runNativeTurnWithBusyRetry(ctx context.Context, chRow biz.Channel, platform string, input biz.TurnInput) (biz.NativeTurnResult, error) {
	if h == nil || h.chat == nil {
		return biz.NativeTurnResult{}, nil
	}
	var last biz.NativeTurnResult
	var lastErr error
	for attempt := 0; attempt < channelTurnBusyRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(channelTurnBusyBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return biz.NativeTurnResult{}, ctx.Err()
			case <-timer.C:
			}
		}
		result, err := h.chat.RunNativeTurnWithOutcome(
			event.WithChannelEnvelopeContext(ctx, platform, chRow.Key),
			input,
		)
		last, lastErr = result, err
		if err == nil || !IsTurnBusyError(err) {
			return result, err
		}
	}
	return last, lastErr
}

func (h *ChannelIngress) channelTurnResultFromNative(sessionID string, result biz.NativeTurnResult) (biz.ChannelTurnResult, error) {
	switch result.Outcome {
	case biz.NativeTurnOutcomeQueued:
		pendingID := strings.TrimSpace(result.PendingID)
		if pendingID == "" && h != nil && h.chat != nil {
			pendingID = h.chat.LastPendingMessageID(sessionID)
		}
		return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeQueued, PendingID: pendingID}, nil
	case biz.NativeTurnOutcomeCompleted:
		reply := strings.TrimSpace(result.AssistantMsg.ContentMarkdown)
		return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeCompleted, Reply: reply}, nil
	default:
		return biz.ChannelTurnResult{Outcome: biz.TurnOutcomeFailed},
			kerrors.InternalServer("CHANNEL", fmt.Sprintf("unexpected native turn outcome: %s", result.Outcome))
	}
}
