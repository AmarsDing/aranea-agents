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
	arametrics "aranea-agents/internal/metrics"

	"github.com/go-kratos/kratos/v2/log"
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
		platform = channelTypeFromConfig(chRow.ConfigJSON)
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
	if strings.TrimSpace(reply) == "" {
		return nil
	}
	_, err := h.channels.EnqueueOutboundDelivery(ctx, chRow.ID, biz.ChannelOutboundPayload{
		Platform:       platform,
		Recipient:      recipient,
		Text:           reply,
		IdempotencyKey: idempotencyKey,
		Extra:          extra,
	})
	return err
}

// ChannelDeliveryWorker drains pending outbound channel deliveries.
type ChannelDeliveryWorker struct {
	channels *biz.ChannelUsecase
	ingress  *ChannelIngress
	log      *log.Helper
}

func NewChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *ChannelIngress, logger log.Logger) *ChannelDeliveryWorker {
	return &ChannelDeliveryWorker{
		channels: channels,
		ingress:  ingress,
		log:      log.NewHelper(logger),
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
			_, _ = w.channels.MarkOutboundAttempt(ctx, row, err)
			continue
		}
		platform := strings.ToLower(strings.TrimSpace(payload.Platform))
		if platform == "" {
			platform = "unknown"
		}
		if payload.Kind != "" && payload.Kind != "outbound_text" {
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "invalid").Inc()
			_, _ = w.channels.MarkOutboundAttempt(ctx, row, fmt.Errorf("unsupported delivery kind %q", payload.Kind))
			continue
		}
		start := time.Now()
		sendErr := w.ingress.sendOutboundPayload(ctx, row.ChannelID, payload)
		arametrics.ChannelDeliveryDuration.WithLabelValues(platform).Observe(time.Since(start).Seconds())
		deadLetter, _ := w.channels.MarkOutboundAttempt(ctx, row, sendErr)
		switch {
		case sendErr == nil:
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "delivered").Inc()
		case deadLetter:
			arametrics.ChannelDeliveryTotal.WithLabelValues(platform, "dead_letter").Inc()
			if w.log != nil {
				w.log.Warnf("channel delivery dead-letter channel=%s delivery=%s platform=%s attempts=%d err=%v",
					row.ChannelID, row.ID, payload.Platform, payload.Attempts+1, sendErr)
			}
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

func wechatAppCreds(configJSON string, creds []biz.ChannelCredential, ctx context.Context) (appID, appSecret string) {
	var env struct {
		Config struct {
			AppID string `json:"app_id"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	appID = strings.TrimSpace(env.Config.AppID)
	appSecret, _ = resolveCredentialPlain(ctx, creds, "app_secret")
	return appID, strings.TrimSpace(appSecret)
}
