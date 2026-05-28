package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/event"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"
)

// ChannelIngress bridges external channel webhooks to in-process chat turns.
type ChannelIngress struct {
	channels        *biz.ChannelUsecase
	peers           biz.ChannelPeerSessionRepo
	inboundReceipts biz.ChannelInboundReceiptRepo
	turnJobs        *biz.ChannelTurnJobUsecase
	sessions        *biz.SessionUsecase
	agents          biz.AgentRepository
	teams           biz.TeamRepository
	chat            biz.NativeTurnGateway
	flowBuffer      *event.Buffer
	graphs          biz.GraphExecutor
	cron            *CronService
	eventBus        event.Bus
	http            *http.Client
	inboundInflight inboundInflightSet
	messageDedupe   *ingressMessageDedupe
	peerDebouncer   *ingressPeerDebouncer
	previewRegistry *turnPreviewRegistry
	concurrentGate  *channelConcurrentGate
}

// NewChannelIngress wires channel runtime ingress.
// chat is the narrow turn gateway; flowBuffer is the event buffer for flow logging.
// Accepts biz.NativeTurnGateway instead of *ChatService so Channel never depends on
// Chat concrete internals (Phase B1: port-first).
func NewChannelIngress(
	channels *biz.ChannelUsecase,
	peers biz.ChannelPeerSessionRepo,
	inboundReceipts biz.ChannelInboundReceiptRepo,
	turnJobs *biz.ChannelTurnJobUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	teams biz.TeamRepository,
	chat biz.NativeTurnGateway,
	flowBuffer *event.Buffer,
	graphs biz.GraphExecutor,
	cron *CronService,
	eventBus event.Bus,
) *ChannelIngress {
	return &ChannelIngress{
		channels:        channels,
		peers:           peers,
		inboundReceipts: inboundReceipts,
		turnJobs:        turnJobs,
		sessions:        sessions,
		agents:          agents,
		teams:           teams,
		chat:            chat,
		flowBuffer:      flowBuffer,
		graphs:          graphs,
		cron:            cron,
		eventBus:        eventBus,
		http:            lark.DefaultHTTPClient(),
		messageDedupe:   newIngressMessageDedupe(defaultMessageDedupeTTL),
		peerDebouncer:   newIngressPeerDebouncer(defaultIngressDebounce),
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
		if h == nil || h.channels == nil || h.chat == nil || h.peers == nil || h.sessions == nil || h.agents == nil || h.teams == nil {
			return kerrors.InternalServer("CHANNEL", "ingress not configured")
		}
		r := kctx.Request()
		w := kctx.Response()
		channelKey := strings.TrimSpace(mux.Vars(r)["channel_key"])
		if channelKey == "" {
			http.Error(w, "missing channel_key", http.StatusBadRequest)
			return nil
		}
		if !allowWebhookRequest(channelKey) {
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
		channelType := channelTypeFromConfig(chRow.ConfigJSON)
		switch channelType {
		case "dingtalk":
			if err := h.handleDingTalkWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.dingtalk_failed", "钉钉 Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "wecom", "wecom-app":
			if err := h.handleWeComWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.wecom_failed", "企微 Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "slack":
			if err := h.handleSlackWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.slack_failed", "Slack Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "telegram":
			if err := h.handleTelegramWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.telegram_failed", "Telegram Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "wechat":
			if err := h.handleWeChatWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.wechat_failed", "微信 Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "personal_qq":
			if err := h.handleOneBotWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.onebot_failed", "OneBot Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "qq":
			if err := h.handleQQWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.qq_failed", "QQ Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "line":
			if err := h.handleLINEWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.line_failed", "LINE Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "mattermost":
			if err := h.handleMattermostWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.mattermost_failed", "Mattermost Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
				)
			}
			return nil
		case "teams":
			if err := h.handleTeamsWebhook(w, r, chRow); err != nil {
				event.SysLogWarn("channel.webhook.teams_failed", "Teams Webhook 处理失败",
					event.P("error", err.Error()),
					event.P("channel_id", chRow.ID),
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
		encryptKey, encErr := resolveCredentialPlain(r.Context(), creds, "encrypt_key")
		if encErr != nil {
			event.SysLogWarn("channel.credential.resolve_failed", "凭证解析失败",
				event.P("key", "encrypt_key"),
				event.P("error", encErr.Error()),
			)
		}
		raw, err = lark.UnwrapEncryptedWebhookBody(encryptKey, raw)
		if err != nil {
			http.Error(w, "decrypt failed", http.StatusBadRequest)
			return nil
		}
		if err := lark.VerifyHTTPRequest(r, encryptKey, raw); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
		verTok, verErr := resolveCredentialPlain(r.Context(), creds, "verification_token")
		if verErr != nil {
			event.SysLogWarn("channel.credential.resolve_failed", "凭证解析失败",
				event.P("key", "verification_token"),
				event.P("error", verErr.Error()),
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
		if channelReceiveModeFromConfig(chRow.ConfigJSON) == "websocket" {
			w.WriteHeader(http.StatusOK)
			return nil
		}
		ev, ok, rejectReason := lark.InboundEventFromWebhook(parsed)
		if !ok {
			if dErr := h.recordDelivery(r.Context(), chRow.ID, "skipped_"+rejectReason, map[string]any{
				"message_id": parsed.MessageID,
				"peer_id":    ingressFirstNonEmpty(parsed.SenderOpenID, parsed.ChatID),
				"via":        "webhook",
			}, ""); dErr != nil {
				event.SysLogWarn("system.monitor.alert_channel_fail", "recordDelivery failed", event.P("channel_id", chRow.ID), event.P("status", "skipped_"+rejectReason), event.P("error", dErr.Error()))
			}
			w.WriteHeader(http.StatusOK)
			return nil
		}
		writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, ev))
		return nil
	}
}

func channelTypeFromConfig(configJSON string) string {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(configJSON), &env); err != nil {
		event.SysLogWarn("channel.config.parse_failed", "渠道配置 JSON 解析失败",
			event.P("error", err.Error()),
		)
	}
	return strings.TrimSpace(strings.ToLower(env.Type))
}

func channelReceiveModeFromConfig(configJSON string) string {
	var env struct {
		ReceiveMode string `json:"receive_mode"`
	}
	if err := json.Unmarshal([]byte(configJSON), &env); err != nil {
		event.SysLogWarn("channel.config.parse_failed", "渠道配置 JSON 解析失败",
			event.P("error", err.Error()),
		)
	}
	return strings.TrimSpace(strings.ToLower(env.ReceiveMode))
}

func (h *ChannelIngress) recordDelivery(ctx context.Context, channelID, status string, payload map[string]any, errMsg string) error {
	b, _ := json.Marshal(payload)
	if err := h.channels.AddInboundDelivery(ctx, channelID, status, string(b), errMsg); err != nil {
		event.SysLogWarn("system.monitor.alert_channel_fail", "recordDelivery failed", event.P("channel_id", channelID), event.P("status", status), event.P("error", err.Error()))
		return err
	}
	return nil
}

func ingressFirstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

func resolveCredentialPlain(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if !strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			continue
		}
		ref := strings.TrimSpace(c.SecretRef)
		if ref == "" {
			return "", nil
		}
		return ResolveSecretRef(ctx, ref)
	}
	return "", nil
}
