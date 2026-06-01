package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/preview"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (h *ChannelIngress) runChatTurn(
	ctx context.Context,
	chRow biz.Channel,
	titlePrefix string,
	peerKey string,
	peerID string,
	content string,
) (reply string, err error) {
	ev := port.InboundEvent{PeerID: peerID, PeerKey: peerKey, Text: content}
	platform := strings.TrimSpace(titlePrefix)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON, h.lg)
	}
	result, err := h.runChatTurnWithOutcome(ctx, chRow, platform, ev)
	if err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (h *ChannelIngress) enqueueOutboundReply(
	ctx context.Context,
	chRow biz.Channel,
	platform string,
	recipient string,
	reply string,
	extra map[string]string,
	idempotencyKey string,
) error {
	return h.enqueueOutboundText(ctx, chRow, platform, recipient, reply, extra, idempotencyKey, outboundFormatAssistant, false)
}

func (h *ChannelIngress) enqueueOutboundTranscript(
	ctx context.Context,
	chRow biz.Channel,
	platform string,
	recipient string,
	reply string,
	extra map[string]string,
	idempotencyKey string,
	alreadyFormatted bool,
) error {
	return h.enqueueOutboundText(ctx, chRow, platform, recipient, reply, extra, idempotencyKey, outboundFormatTranscript, alreadyFormatted)
}

type outboundFormatMode int

const (
	outboundFormatAssistant outboundFormatMode = iota
	outboundFormatTranscript
)

func (h *ChannelIngress) enqueueOutboundText(
	ctx context.Context,
	chRow biz.Channel,
	platform string,
	recipient string,
	reply string,
	extra map[string]string,
	idempotencyKey string,
	mode outboundFormatMode,
	alreadyFormatted bool,
) error {
	if !alreadyFormatted {
		switch mode {
		case outboundFormatTranscript:
			reply = preview.FormatRenderedTranscriptForIM(platform, reply)
		default:
			reply = preview.FormatAssistantReplyForIM(platform, reply)
		}
	}
	if strings.TrimSpace(reply) == "" {
		h.lg.Warn("Channel outbound empty after format",
			loggateway.StepID(flowStepChannelOutbound),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Str("platform", platform),
			loggateway.Str("idempotency_key", idempotencyKey),
		)
		reply = channelOutboundEmptyFallback
	}
	limit := preview.PlatformTextLimit(platform)
	pages := preview.SplitPages(reply, limit)
	extra = h.applyFeishuOutboundMeta(chRow, extra)
	if len(pages) == 0 {
		pages = []string{reply}
	}
	var firstErr error
	for i, page := range pages {
		key := idempotencyKey
		if len(pages) > 1 {
			key = fmt.Sprintf("%s:page:%d", idempotencyKey, i+1)
		}
		if _, err := h.channels.EnqueueOutboundDelivery(ctx, chRow.ID, biz.ChannelOutboundPayload{
			Platform:       platform,
			Recipient:      recipient,
			Text:           page,
			IdempotencyKey: key,
			Extra:          extra,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ChannelDeliveryWorker drains pending outbound channel deliveries.
type ChannelDeliveryWorker struct {
	channels *biz.ChannelUsecase
	ingress  *ChannelIngress
	lg       loggateway.Logger
}

func NewChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *ChannelIngress, lg loggateway.Logger) *ChannelDeliveryWorker {
	return &ChannelDeliveryWorker{
		channels: channels,
		ingress:  ingress,
		lg:       lg,
	}
}

func (w *ChannelDeliveryWorker) ProcessPending(ctx context.Context, limit int) error {
	if w == nil || w.channels == nil || w.ingress == nil {
		return nil
	}
	items, err := w.channels.ListPendingOutboundDeliveries(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range items {
		if !w.channels.IsOutboundDeliveryReady(row) {
			continue
		}
		var payload biz.ChannelOutboundPayload
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			arametrics.ChannelDeliveryTotal.WithLabelValues("unknown", "invalid").Inc()
			if _, markErr := w.channels.MarkOutboundAttempt(ctx, row, err); markErr != nil {
				w.lg.Warn("标记投递尝试失败",
					loggateway.StepID("channel.delivery.mark_attempt_failed"),
					loggateway.Str("error", markErr.Error()),
				)
			}
			continue
		}
		platform := strings.ToLower(strings.TrimSpace(payload.Platform))
		if platform == "" {
			platform = "unknown"
		}
		if payload.Kind != "" && payload.Kind != biz.ChannelOutboundTextKind && payload.Kind != biz.ChannelOutboundCardKind {
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "invalid").Inc()
			if _, markErr := w.channels.MarkOutboundAttempt(ctx, row, kerrors.BadRequest("CHANNEL", fmt.Sprintf("unsupported delivery kind %q", payload.Kind))); markErr != nil {
				w.lg.Warn("标记投递尝试失败",
					loggateway.StepID("channel.delivery.mark_attempt_failed"),
					loggateway.Str("error", markErr.Error()),
				)
			}
			continue
		}
		if payload.Kind == biz.ChannelOutboundCardKind && strings.TrimSpace(payload.CardJSON) == "" && strings.TrimSpace(payload.Text) == "" {
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "invalid").Inc()
			if _, markErr := w.channels.MarkOutboundAttempt(ctx, row, kerrors.BadRequest("CHANNEL", "outbound_card missing card_json")); markErr != nil {
				w.lg.Warn("标记投递尝试失败",
					loggateway.StepID("channel.delivery.mark_attempt_failed"),
					loggateway.Str("error", markErr.Error()),
				)
			}
			continue
		}
		if payload.Kind == "" || payload.Kind == biz.ChannelOutboundTextKind {
			if strings.TrimSpace(payload.Text) == "" {
				arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "invalid").Inc()
				if _, markErr := w.channels.MarkOutboundAttempt(ctx, row, kerrors.BadRequest("CHANNEL", "outbound_text missing text")); markErr != nil {
					w.lg.Warn("标记投递尝试失败",
						loggateway.StepID("channel.delivery.mark_attempt_failed"),
						loggateway.Str("error", markErr.Error()),
					)
				}
				continue
			}
		}
		start := time.Now()
		sendErr := w.ingress.sendOutboundPayload(ctx, row.ChannelID, payload)
		arametrics.ChannelDeliveryDuration.WithLabelValues(platform).Observe(time.Since(start).Seconds())
		deadLetter, markErr := w.channels.MarkOutboundAttempt(ctx, row, sendErr)
		if markErr != nil {
			w.lg.Warn("标记投递尝试失败",
				loggateway.StepID("channel.delivery.mark_attempt_failed"),
				loggateway.Str("error", markErr.Error()),
			)
		}
		switch {
		case sendErr == nil:
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "delivered").Inc()
		case deadLetter:
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "dead_letter").Inc()
			w.lg.Warn("channel delivery dead-letter",
				loggateway.StepID("system.channel.dead_letter"),
				loggateway.Str("channel_id", row.ChannelID),
				loggateway.Str("delivery_id", row.ID),
				loggateway.Str("platform", payload.Platform),
				loggateway.Str("attempts", fmt.Sprint(payload.Attempts+1)),
				loggateway.Str("error", sendErr.Error()),
			)
		default:
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "retry").Inc()
		}
	}
	return nil
}

func telegramChatRecipient(chatID int64) string {
	return strconv.FormatInt(chatID, 10)
}

func oneBotHTTPServer(configJSON string) string {
	var env struct {
		Config struct {
			HTTPServer string `json:"onebot_http_server"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	return strings.TrimSpace(env.Config.HTTPServer)
}

func wechatAppCreds(configJSON string, creds []biz.ChannelCredential, ctx context.Context, channels *biz.ChannelUsecase, lg loggateway.Logger) (appID, appSecret string) {
	var env struct {
		Config struct {
			AppID string `json:"app_id"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	appID = strings.TrimSpace(env.Config.AppID)
	appSecret, _ = resolveCredentialPlain(ctx, channels, creds, "app_secret", lg)
	return appID, strings.TrimSpace(appSecret)
}
