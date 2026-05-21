package service

import (
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/dingtalk"
)

func (h *ChannelIngress) handleDingTalkWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	secret, _ := resolveCredentialPlain(r.Context(), creds, "secret")
	if err := dingtalk.VerifySign(r.URL.Query().Get("timestamp"), r.URL.Query().Get("sign"), secret); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := dingtalk.ParseInbound(raw)
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
	peerID := ingressFirstNonEmpty(parsed.SenderStaffID, parsed.ConversationID, parsed.SenderNick)
	peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
	reply, err := h.runChatTurn(r.Context(), chRow, "dingtalk", peerKey, peerID, parsed.Text)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		http.Error(w, "agent error", http.StatusInternalServerError)
		return nil
	}
	idempotency := "dingtalk:" + parsed.ConversationID + ":" + strings.TrimSpace(r.URL.Query().Get("timestamp"))
	if err := h.enqueueOutboundReply(r.Context(), chRow, "dingtalk", parsed.SessionWebhook, reply, map[string]string{
		"session_webhook": parsed.SessionWebhook,
	}, idempotency); err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		http.Error(w, "queue", http.StatusInternalServerError)
		return nil
	}
	_ = h.recordDelivery(r.Context(), chRow.ID, "queued", map[string]any{"conversation_id": parsed.ConversationID}, "")
	w.WriteHeader(http.StatusOK)
	return nil
}
