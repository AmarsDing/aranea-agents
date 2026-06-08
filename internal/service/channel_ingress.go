package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"
)

// ChannelIngress bridges external channel webhooks to in-process chat turns.
type ChannelIngress struct {
	channels        *biz.ChannelUsecase
	turnJobs        *biz.ChannelTurnJobUsecase
	sessions        *biz.SessionUsecase
	chat            biz.ChannelTurnGateway
	flowBuffer      *event.Buffer
	graphs          biz.GraphExecutor
	cron            biz.CronTriggerGateway
	eventBus        event.Bus
	http            *http.Client
	inboundInflight inboundInflightSet
	messageDedupe   *ingressMessageDedupe
	peerDebouncer   *ingressPeerDebouncer
	previewRegistry *turnPreviewRegistry
	concurrentGate  *channelConcurrentGate
	lg              loggateway.Logger
}

// NewChannelIngress wires channel runtime ingress.
// chat is the narrow turn gateway; flowBuffer is the event buffer for flow logging.
// Accepts biz.ChannelTurnGateway instead of *ChatService so Channel never depends on
// Chat concrete internals (Phase B1: port-first).
func NewChannelIngress(
	channels *biz.ChannelUsecase,
	turnJobs *biz.ChannelTurnJobUsecase,
	sessions *biz.SessionUsecase,
	chat biz.ChannelTurnGateway,
	flowBuffer *event.Buffer,
	graphs biz.GraphExecutor,
	cron biz.CronTriggerGateway,
	eventBus event.Bus,
	lg loggateway.Logger,
) *ChannelIngress {
	return &ChannelIngress{
		channels:        channels,
		turnJobs:        turnJobs,
		sessions:        sessions,
		chat:            chat,
		flowBuffer:      flowBuffer,
		graphs:          graphs,
		cron:            cron,
		eventBus:        eventBus,
		lg:              lg,
		http:            lark.DefaultHTTPClient(),
		messageDedupe:   newIngressMessageDedupe(defaultMessageDedupeTTL),
		peerDebouncer:   newIngressPeerDebouncer(defaultIngressDebounce, lg),
		previewRegistry: newTurnPreviewRegistry(),
		concurrentGate:  newChannelConcurrentGate(),
	}
}

func (h *ChannelIngress) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// FeishuWebhookHTTP returns a handler for POST /webhooks/{channel_key}.
func (h *ChannelIngress) FeishuWebhookHTTP() func(ctx khttp.Context) error {
	return func(kctx khttp.Context) error {
		if h == nil || h.channels == nil || h.chat == nil || h.sessions == nil {
			return kerrors.InternalServer("CHANNEL", "ingress not configured")
		}
		r := kctx.Request()
		w := kctx.Response()
		channelKey := strings.TrimSpace(mux.Vars(r)["channel_key"])
		if channelKey == "" {
			http.Error(w, "missing channel_key", http.StatusBadRequest)
			return nil
		}
		if !allowWebhookRequest(channelKey, h.lg) {
			webhookRateLimitResponse(w)
			return nil
		}
		chRow, err := h.channels.GetByKey(r.Context(), channelKey)
		if err != nil {
			http.Error(w, "channel not found", http.StatusNotFound)
			return nil
		}
		if !chRow.Enabled {
			http.Error(w, "channel disabled", http.StatusForbidden)
			return nil
		}
		channelType := channelTypeFromConfig(chRow.ConfigJSON, h.lg)
		switch channelType {
		case "dingtalk":
			if err := h.handleDingTalkWebhook(w, r, chRow); err != nil {
				h.lg.Warn("钉钉 Webhook 处理失败",
					loggateway.StepID("channel.webhook.dingtalk_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "wecom", "wecom-app":
			if err := h.handleWeComWebhook(w, r, chRow); err != nil {
				h.lg.Warn("企微 Webhook 处理失败",
					loggateway.StepID("channel.webhook.wecom_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "slack":
			if err := h.handleSlackWebhook(w, r, chRow); err != nil {
				h.lg.Warn("Slack Webhook 处理失败",
					loggateway.StepID("channel.webhook.slack_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "telegram":
			if err := h.handleTelegramWebhook(w, r, chRow); err != nil {
				h.lg.Warn("Telegram Webhook 处理失败",
					loggateway.StepID("channel.webhook.telegram_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "wechat":
			if err := h.handleWeChatWebhook(w, r, chRow); err != nil {
				h.lg.Warn("微信 Webhook 处理失败",
					loggateway.StepID("channel.webhook.wechat_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "personal_qq":
			if err := h.handleOneBotWebhook(w, r, chRow); err != nil {
				h.lg.Warn("OneBot Webhook 处理失败",
					loggateway.StepID("channel.webhook.onebot_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "qq":
			if err := h.handleQQWebhook(w, r, chRow); err != nil {
				h.lg.Warn("QQ Webhook 处理失败",
					loggateway.StepID("channel.webhook.qq_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "line":
			if err := h.handleLINEWebhook(w, r, chRow); err != nil {
				h.lg.Warn("LINE Webhook 处理失败",
					loggateway.StepID("channel.webhook.line_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "mattermost":
			if err := h.handleMattermostWebhook(w, r, chRow); err != nil {
				h.lg.Warn("Mattermost Webhook 处理失败",
					loggateway.StepID("channel.webhook.mattermost_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "teams":
			if err := h.handleTeamsWebhook(w, r, chRow); err != nil {
				h.lg.Warn("Teams Webhook 处理失败",
					loggateway.StepID("channel.webhook.teams_failed"),
					loggateway.Err(err),
					loggateway.Str("channel_id", chRow.ID),
				)
			}
			return nil
		case "feishu":
			// continue below
		default:
			http.Error(w, "unsupported channel type", http.StatusBadRequest)
			return nil
		}

		raw, err := lark.ReadBodyDrain(r)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return nil
		}
		creds, err := h.channels.ListCredentialsRaw(r.Context(), chRow.ID)
		if err != nil {
			http.Error(w, "credentials", http.StatusInternalServerError)
			return nil
		}
		encryptKey, encErr := resolveCredentialPlain(r.Context(), h.channels, creds, "encrypt_key", h.lg)
		if encErr != nil {
			h.lg.Warn("凭证解析失败",
				loggateway.StepID("channel.credential.resolve_failed"),
				loggateway.Str("key", "encrypt_key"),
				loggateway.Err(encErr),
			)
		}
		raw, err = lark.UnwrapEncryptedWebhookBody(encryptKey, raw)
		if err != nil {
			http.Error(w, "decrypt failed", http.StatusBadRequest)
			return nil
		}
		if err := lark.VerifyHTTPRequest(r, encryptKey, raw); err != nil {
			h.lg.Warn("飞书 Webhook 签名验证失败",
				loggateway.StepID("channel.feishu.webhook.verify_fail"),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Err(err),
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
		verTok, verErr := resolveCredentialPlain(r.Context(), h.channels, creds, "verification_token", h.lg)
		if verErr != nil {
			h.lg.Warn("凭证解析失败",
				loggateway.StepID("channel.credential.resolve_failed"),
				loggateway.Str("key", "verification_token"),
				loggateway.Err(verErr),
			)
		}
		parsed, err := lark.ParseWebhookPost(raw, verTok)
		if err != nil {
			http.Error(w, "bad event", http.StatusBadRequest)
			return nil
		}
		if parsed.IsURLVerification {
			h.writeJSON(w, http.StatusOK, map[string]string{"challenge": parsed.Challenge})
			return nil
		}
		if parsed.EventType == "card.action.trigger" || parsed.EventType == "card.action.trigger_v1" {
			if action, ok := lark.CardActionPayloadFromWebhook(parsed); ok {
				resp := h.HandleFeishuCardAction(r.Context(), chRow, action)
				h.writeJSON(w, http.StatusOK, resp)
				return nil
			}
			w.WriteHeader(http.StatusOK)
			return nil
		}
		if parsed.EventType != "im.message.receive_v1" {
			w.WriteHeader(http.StatusOK)
			return nil
		}
		if channelReceiveModeFromConfig(chRow.ConfigJSON, h.lg) == "websocket" {
			w.WriteHeader(http.StatusOK)
			return nil
		}
		ev, ok, rejectReason := lark.InboundEventFromWebhook(parsed)
		if !ok {
			h.recordDelivery(r.Context(), chRow.ID, "skipped_"+rejectReason, map[string]any{
				"message_id": parsed.MessageID,
				"peer_id":    ingressFirstNonEmpty(parsed.SenderOpenID, parsed.ChatID),
				"via":        "webhook",
			}, "")
			w.WriteHeader(http.StatusOK)
			return nil
		}
		writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, ev))
		return nil
	}
}

func channelTypeFromConfig(configJSON string, lg loggateway.Logger) string {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(configJSON), &env); err != nil {
		lg.Warn("渠道配置 JSON 解析失败",
			loggateway.StepID("channel.config.parse_failed"),
			loggateway.Err(err),
		)
	}
	return strings.TrimSpace(strings.ToLower(env.Type))
}

func channelReceiveModeFromConfig(configJSON string, lg loggateway.Logger) string {
	var env struct {
		ReceiveMode string `json:"receive_mode"`
	}
	if err := json.Unmarshal([]byte(configJSON), &env); err != nil {
		lg.Warn("渠道配置 JSON 解析失败",
			loggateway.StepID("channel.config.parse_failed"),
			loggateway.Err(err),
		)
	}
	return strings.TrimSpace(strings.ToLower(env.ReceiveMode))
}

func (h *ChannelIngress) recordDelivery(ctx context.Context, channelID, status string, payload map[string]any, errMsg string) {
	b, _ := json.Marshal(payload)
	if err := h.channels.AddInboundDelivery(ctx, channelID, status, string(b), errMsg); err != nil {
		h.lg.Warn("recordDelivery failed",
			loggateway.StepID("monitor.alert_channel_fail"),
			loggateway.Str("channel_id", channelID),
			loggateway.Str("status", status),
			loggateway.Err(err),
		)
	}
}

func ingressFirstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

func resolveCredentialPlain(ctx context.Context, channels *biz.ChannelUsecase, creds []biz.ChannelCredential, key string, lg loggateway.Logger) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if !strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			continue
		}
		ref := strings.TrimSpace(c.SecretRef)
		if ref == "" {
			lg.Warn("凭证 secret_ref 为空",
				loggateway.StepID("channel.credential.empty_ref"),
				loggateway.Str("key", key),
			)
			return "", nil
		}
		return ResolveSecretRef(ctx, channels, ref)
	}
	lg.Warn("凭证 key 未找到",
		loggateway.StepID("channel.credential.not_found"),
		loggateway.Str("key", key),
	)
	return "", nil
}
