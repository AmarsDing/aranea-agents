package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/event"

	graphv1 "aranea-agents/api/kratos/graph/v1"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/mux"
)

// channelAsyncGraphExecutor runs async graph/team_graph targets from channel ingress.
type channelAsyncGraphExecutor interface {
	ExecuteGraph(ctx context.Context, req *graphv1.ExecuteGraphRequest) (*graphv1.ExecuteGraphResponse, error)
	ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID string, cfg biz.GraphBuildConfig, initialState map[string]any) (*graphv1.ExecuteGraphResponse, error)
}

// ChannelIngress bridges external channel webhooks to in-process chat (native runner via ChatService).
type ChannelIngress struct {
	channels        *biz.ChannelUsecase
	peers           biz.ChannelPeerSessionRepo
	inboundReceipts biz.ChannelInboundReceiptRepo
	turnJobs        *biz.ChannelTurnJobUsecase
	sessions        *biz.SessionUsecase
	agents          biz.AgentRepository
	teams           biz.TeamRepository
	chat            *ChatService
	graphs          channelAsyncGraphExecutor
	cron            *CronService
	eventBus        event.Bus
	http            *http.Client
	inboundInflight inboundInflightSet
}

// NewChannelIngress wires channel runtime ingress.
func NewChannelIngress(
	channels *biz.ChannelUsecase,
	peers biz.ChannelPeerSessionRepo,
	inboundReceipts biz.ChannelInboundReceiptRepo,
	turnJobs *biz.ChannelTurnJobUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	teams biz.TeamRepository,
	chat *ChatService,
	graphs *GraphService,
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
		graphs:          graphs,
		cron:            cron,
		eventBus:        eventBus,
		http:            lark.DefaultHTTPClient(),
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
			_ = h.handleDingTalkWebhook(w, r, chRow)
			return nil
		case "wecom", "wecom-app":
			_ = h.handleWeComWebhook(w, r, chRow)
			return nil
		case "slack":
			_ = h.handleSlackWebhook(w, r, chRow)
			return nil
		case "telegram":
			_ = h.handleTelegramWebhook(w, r, chRow)
			return nil
		case "wechat":
			_ = h.handleWeChatWebhook(w, r, chRow)
			return nil
		case "personal_qq":
			_ = h.handleOneBotWebhook(w, r, chRow)
			return nil
		case "qq":
			_ = h.handleQQWebhook(w, r, chRow)
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
		encryptKey, _ := resolveCredentialPlain(r.Context(), creds, "encrypt_key")
		if err := lark.VerifyHTTPRequest(r, encryptKey, raw); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
		verTok, _ := resolveCredentialPlain(r.Context(), creds, "verification_token")
		parsed, err := lark.ParseWebhookPost(raw, verTok)
		if err != nil {
			http.Error(w, "bad event", http.StatusBadRequest)
			return nil
		}
		if parsed.IsURLVerification {
			h.writeJSON(w, http.StatusOK, map[string]string{"challenge": parsed.Challenge})
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
			_ = h.recordDelivery(r.Context(), chRow.ID, "skipped_"+rejectReason, map[string]any{
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

func channelTypeFromConfig(configJSON string) string {
	var env struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	return strings.TrimSpace(strings.ToLower(env.Type))
}

func channelReceiveModeFromConfig(configJSON string) string {
	var env struct {
		ReceiveMode string `json:"receive_mode"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	return strings.TrimSpace(strings.ToLower(env.ReceiveMode))
}

func (h *ChannelIngress) recordDelivery(ctx context.Context, channelID, status string, payload map[string]any, errMsg string) error {
	b, _ := json.Marshal(payload)
	return h.channels.AddInboundDelivery(ctx, channelID, status, string(b), errMsg)
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
