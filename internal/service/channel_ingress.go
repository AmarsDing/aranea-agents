package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// ChannelIngress bridges Feishu/Lark webhooks to in-process chat (native runner via ChatService).
type ChannelIngress struct {
	channels *biz.ChannelUsecase
	peers    biz.ChannelPeerSessionRepo
	sessions *biz.SessionUsecase
	agents   biz.AgentRepository
	teams    biz.TeamRepository
	chat     *ChatService
	http     *http.Client
}

// NewChannelIngress wires channel runtime ingress (no ADK types in this package beyond ChatService).
func NewChannelIngress(
	channels *biz.ChannelUsecase,
	peers biz.ChannelPeerSessionRepo,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	teams biz.TeamRepository,
	chat *ChatService,
) *ChannelIngress {
	return &ChannelIngress{
		channels: channels,
		peers:    peers,
		sessions: sessions,
		agents:   agents,
		teams:    teams,
		chat:     chat,
		http:     lark.DefaultHTTPClient(),
	}
}

func (h *ChannelIngress) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// FeishuWebhookHTTP returns a handler for POST /webhooks/{channel_key}.
func (h *ChannelIngress) FeishuWebhookHTTP() func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if h == nil || h.channels == nil || h.chat == nil || h.peers == nil || h.sessions == nil || h.agents == nil || h.teams == nil {
			return kerrors.InternalServer("CHANNEL", "ingress not configured")
		}
		r := ctx.Request()
		w := ctx.Response()
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
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(chRow.ConfigJSON), &env); err != nil || strings.TrimSpace(strings.ToLower(env.Type)) != "feishu" {
			http.Error(w, "not a feishu channel", http.StatusBadRequest)
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
		encryptKey, _ := resolveCredentialPlain(creds, "encrypt_key")
		if err := lark.VerifyHTTPRequest(r, encryptKey, raw); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
		verTok, _ := resolveCredentialPlain(creds, "verification_token")
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
			_, _ = io.WriteString(w, "{}")
			return nil
		}
		if strings.TrimSpace(parsed.Text) == "" {
			w.WriteHeader(http.StatusOK)
			return nil
		}

		routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
			http.Error(w, "routing", http.StatusInternalServerError)
			return nil
		}
		peerID := ingressFirstNonEmpty(parsed.SenderOpenID, parsed.ChatID)
		peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
		ownerType, agentID, teamID, err := biz.ResolveChannelTarget(r.Context(), h.agents, h.teams, routing, peerID)
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "resolve", "error": err.Error()}, err.Error())
			http.Error(w, "route", http.StatusBadRequest)
			return nil
		}

		bind, err := h.peers.GetByChannelAndPeer(r.Context(), chRow.ID, peerKey)
		var sessionID string
		switch {
		case err == nil && strings.TrimSpace(bind.SessionID) != "":
			sessionID = bind.SessionID
		case err != nil && err != sql.ErrNoRows:
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "peer_bind", "error": err.Error()}, err.Error())
			http.Error(w, "bind", http.StatusInternalServerError)
			return nil
		default:
			title := "feishu:" + channelKey + ":" + peerKey
			created, cerr := h.sessions.Create(r.Context(), biz.Session{
				OwnerType: ownerType,
				AgentID:   agentID,
				TeamID:    teamID,
				Title:     title,
			})
			if cerr != nil {
				_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "session", "error": cerr.Error()}, cerr.Error())
				http.Error(w, "session", http.StatusInternalServerError)
				return nil
			}
			sessionID = created.ID
			if _, cerr = h.peers.Create(r.Context(), biz.ChannelPeerSession{
				ID:        uuid.NewString(),
				ChannelID: chRow.ID,
				PeerKey:   peerKey,
				SessionID: sessionID,
			}); cerr != nil {
				_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "peer_create", "error": cerr.Error()}, cerr.Error())
				http.Error(w, "peer row", http.StatusInternalServerError)
				return nil
			}
		}

		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   parsed.Text,
		}
		if ownerType == "team" && teamID != "" {
			tid := teamID
			req.TeamId = &tid
		}
		// Uses ChatService.runNativeAgentTurn → shared RunGateway (active run / pending / cancel).
		_, asst, err := h.chat.RunNativeTurnUnary(r.Context(), req)
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
			http.Error(w, "agent error", http.StatusInternalServerError)
			return nil
		}
		reply := strings.TrimSpace(asst.ContentMarkdown)
		if reply == "" {
			w.WriteHeader(http.StatusOK)
			return nil
		}
		region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "feishu_cfg", "error": err.Error()}, err.Error())
			http.Error(w, "config", http.StatusInternalServerError)
			return nil
		}
		appRef, err := ChannelCredentialSecretRef(creds, "app_secret")
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "cred", "error": err.Error()}, err.Error())
			http.Error(w, "secret", http.StatusInternalServerError)
			return nil
		}
		sec, err := ResolveSecretRef(appRef)
		if err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "resolve_secret", "error": err.Error()}, err.Error())
			http.Error(w, "secret", http.StatusInternalServerError)
			return nil
		}
		if err := (&lark.FeishuTextSender{
			Region:    region,
			AppID:     appID,
			AppSecret: sec,
			HTTP:      h.http,
		}).SendText(r.Context(), parsed.SenderOpenID, reply); err != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "send", "error": err.Error()}, err.Error())
			http.Error(w, "send", http.StatusBadGateway)
			return nil
		}
		_ = h.recordDelivery(r.Context(), chRow.ID, "delivered", map[string]any{"message_id": parsed.MessageID, "dm_scope": routing.DMScope}, "")
		w.WriteHeader(http.StatusOK)
		return nil
	}
}

func (h *ChannelIngress) recordDelivery(ctx context.Context, channelID, status string, payload map[string]any, errMsg string) error {
	b, _ := json.Marshal(payload)
	return h.channels.AddInboundDelivery(ctx, channelID, status, string(b), errMsg)
}

func ingressFirstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func resolveCredentialPlain(creds []biz.ChannelCredential, key string) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if !strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			continue
		}
		ref := strings.TrimSpace(c.SecretRef)
		if ref == "" {
			return "", nil
		}
		return ResolveSecretRef(ref)
	}
	return "", nil
}
