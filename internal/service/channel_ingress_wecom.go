package service

import (
	"database/sql"
	"io"
	"net/http"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/wecom"

	"github.com/google/uuid"
)

func (h *ChannelIngress) handleWeComWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return nil
	}
	creds, err := h.channels.ListCredentialsRaw(r.Context(), chRow.ID)
	if err != nil {
		http.Error(w, "credentials", http.StatusInternalServerError)
		return nil
	}
	token, _ := resolveCredentialPlain(creds, "token")
	if err := wecom.VerifySignature(token, r.URL.Query().Get("timestamp"), r.URL.Query().Get("nonce"), r.URL.Query().Get("msg_signature")); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := wecom.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}

	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
		http.Error(w, "routing", http.StatusInternalServerError)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.SenderUserID, parsed.ChatID)
	peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
	ownerType, agentID, teamID, err := biz.ResolveChannelTarget(r.Context(), h.agents, h.teams, routing, peerID)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "resolve", "error": err.Error()}, err.Error())
		http.Error(w, "route", http.StatusBadRequest)
		return nil
	}

	channelKey := strings.TrimSpace(chRow.Key)
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
		title := "wecom:" + channelKey + ":" + peerKey
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
			ID: uuid.NewString(), ChannelID: chRow.ID, PeerKey: peerKey, SessionID: sessionID,
		}); cerr != nil {
			_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "peer_create", "error": cerr.Error()}, cerr.Error())
			http.Error(w, "peer row", http.StatusInternalServerError)
			return nil
		}
	}

	req := &chatv1.SendChatMessageRequest{SessionId: sessionID, Content: parsed.Text}
	if ownerType == "team" && teamID != "" {
		tid := teamID
		req.TeamId = &tid
	}
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
	webhookURL, _ := resolveCredentialPlain(creds, "webhook_url")
	sender := &wecom.TextSender{WebhookURL: webhookURL, HTTP: h.http}
	if err := sender.SendText(r.Context(), parsed.ResponseURL, reply); err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "send", "error": err.Error()}, err.Error())
		http.Error(w, "send", http.StatusBadGateway)
		return nil
	}
	_ = h.recordDelivery(r.Context(), chRow.ID, "delivered", map[string]any{"chat_id": parsed.ChatID}, "")
	w.WriteHeader(http.StatusOK)
	return nil
}
