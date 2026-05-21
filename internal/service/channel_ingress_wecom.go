package service

import (
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/wecom"
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
	token, _ := resolveCredentialPlain(r.Context(), creds, "token")
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
	reply, err := h.runChatTurn(r.Context(), chRow, "wecom", peerKey, peerID, parsed.Text)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		http.Error(w, "agent error", http.StatusInternalServerError)
		return nil
	}
	idempotency := "wecom:" + ingressFirstNonEmpty(parsed.ChatID, parsed.SenderUserID) + ":" + strings.TrimSpace(r.URL.Query().Get("timestamp"))
	if err := h.enqueueOutboundReply(r.Context(), chRow, "wecom", parsed.ResponseURL, reply, map[string]string{
		"response_url": parsed.ResponseURL,
	}, idempotency); err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		http.Error(w, "queue", http.StatusInternalServerError)
		return nil
	}
	_ = h.recordDelivery(r.Context(), chRow.ID, "queued", map[string]any{"chat_id": parsed.ChatID}, "")
	w.WriteHeader(http.StatusOK)
	return nil
}
