package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"
)

// ChannelIngress bridges external channel webhooks to in-process chat turns.
type ChannelIngress struct {
	channels       *biz.ChannelUsecase
	turnJobs       *biz.ChannelTurnJobUsecase
	sessions       *biz.SessionUsecase
	chat           biz.ChannelTurnGateway
	graphs         biz.GraphExecutor
	cron           biz.CronTriggerGateway
	eventBus       biz.EventBus // v2: run_status + graph completion watch + notices
	http           *http.Client
	deduplicator   biz.IngressDeduplicator
	peerDebouncer  biz.PeerDebouncer
	previewManager biz.TurnPreviewManager
	concurrentGate biz.ConcurrencyGate
	admission      *biz.TurnAdmissionUsecase
	teamCompiler   biz.TeamCompiler
	lg             loggateway.Logger
}

// NewChannelIngress wires channel runtime ingress.
// chat is the narrow turn gateway.
// Accepts biz.ChannelTurnGateway instead of *ChatService so Channel never depends on
// Chat concrete internals (Phase B1: port-first).
// deduplicator, peerDebouncer, previewManager, and concurrentGate are injected
// biz-layer components rather than constructed inline, keeping the service layer
// free of biz factory calls and enabling test doubles.
func NewChannelIngress(
	channels *biz.ChannelUsecase,
	turnJobs *biz.ChannelTurnJobUsecase,
	sessions *biz.SessionUsecase,
	chat biz.ChannelTurnGateway,
	graphs biz.GraphExecutor,
	cron biz.CronTriggerGateway,
	eventBus biz.EventBus,
	deduplicator biz.IngressDeduplicator,
	peerDebouncer biz.PeerDebouncer,
	previewManager biz.TurnPreviewManager,
	concurrentGate biz.ConcurrencyGate,
	admission *biz.TurnAdmissionUsecase,
	teamCompiler biz.TeamCompiler,
	lg loggateway.Logger,
) *ChannelIngress {
	return &ChannelIngress{
		channels:       channels,
		turnJobs:       turnJobs,
		sessions:       sessions,
		chat:           chat,
		graphs:         graphs,
		cron:           cron,
		eventBus:       eventBus,
		lg:             lg,
		http:           lark.DefaultHTTPClient(),
		deduplicator:   deduplicator,
		peerDebouncer:  peerDebouncer,
		previewManager: previewManager,
		concurrentGate: concurrentGate,
		admission:      admission,
		teamCompiler:   teamCompiler,
	}
}

func (h *ChannelIngress) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.lg.Warn("writeJSON encode failed", loggateway.StepID("channel.ingress.json_encode"), loggateway.Err(err))
	}
}

// FeishuWebhookHTTP returns a handler for POST /webhooks/{channel_key}.
func (h *ChannelIngress) FeishuWebhookHTTP() func(ctx khttp.Context) error {
	return func(kctx khttp.Context) error {
		if h == nil || h.channels == nil || h.chat == nil || h.sessions == nil {
			return apierror.Internal("CHANNEL", "ingress not configured")
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
		channelType := biz.ChannelTypeFromConfig(chRow.ConfigJSON)
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
		encryptKey, err := resolveCredentialPlain(r.Context(), h.channels, creds, "encrypt_key", h.lg)
		if err != nil {
			h.lg.Warn("encrypt_key 凭证解析失败",
				loggateway.StepID("channel.credential.resolve_failed"),
				loggateway.Err(err),
			)
			http.Error(w, "forbidden: encrypt_key not configured", http.StatusForbidden)
			return nil
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
		verTok, err := resolveCredentialPlain(r.Context(), h.channels, creds, "verification_token", h.lg)
		if err != nil {
			h.lg.Warn("verification_token 凭证解析失败",
				loggateway.StepID("channel.credential.resolve_failed"),
				loggateway.Err(err),
			)
			// verification_token 缺失时仍可继续处理（仅影响事件验签），不阻断请求
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
		if biz.ChannelReceiveModeFromConfig(chRow.ConfigJSON) == "websocket" {
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

func (h *ChannelIngress) recordDelivery(ctx context.Context, channelID, status string, payload map[string]any, errMsg string) {
	b, err := json.Marshal(payload)
	if err != nil {
		h.lg.Warn("recordDelivery marshal failed", loggateway.StepID("channel.ingress.delivery_marshal"), loggateway.Err(err))
	}
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

// Close releases long-lived resources owned by ChannelIngress.
func (h *ChannelIngress) Close() {
	if h == nil {
		return
	}
	if h.concurrentGate != nil {
		h.concurrentGate.Close()
	}
	if h.deduplicator != nil {
		h.deduplicator.Stop()
	}
}

func resolveCredentialPlain(ctx context.Context, channels *biz.ChannelUsecase, creds []biz.ChannelCredential, key string, lg loggateway.Logger) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if !strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			continue
		}
		ref := strings.TrimSpace(c.SecretRef)
		if ref == "" {
			return "", apierror.BadRequest("CHANNEL_CREDENTIAL", fmt.Sprintf("credential %q has empty secret_ref", key))
		}
		return ResolveSecretRef(ctx, channels, ref)
	}
	return "", apierror.NotFound(apierror.DomainChannel, fmt.Sprintf("credential key %q not found", key))
}

// loadRequiredCredential resolves a credential and fails closed when missing/unresolved.
// Callers should return HTTP 403 when ok is false.
func (h *ChannelIngress) loadRequiredCredential(ctx context.Context, channelID string, creds []biz.ChannelCredential, key string) (string, bool) {
	val, err := resolveCredentialPlain(ctx, h.channels, creds, key, h.lg)
	if err != nil || strings.TrimSpace(val) == "" {
		h.lg.Warn("channel credential missing or unresolved",
			loggateway.StepID("channel.credential.resolve_fail"),
			loggateway.Str("channel_id", channelID),
			loggateway.Str("credential_key", key),
			loggateway.Err(err),
		)
		return "", false
	}
	return val, true
}
