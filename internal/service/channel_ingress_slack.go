package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/slack"
)

func (h *ChannelIngress) handleSlackWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	signingSecret, _ := resolveCredentialPlain(r.Context(), creds, "signing_secret")
	if err := slack.VerifyRequest(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), signingSecret, raw); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	challenge, msg, err := slack.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	if challenge != "" {
		h.writeJSON(w, http.StatusOK, map[string]string{"challenge": challenge})
		return nil
	}
	if msg == nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}

	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
		http.Error(w, "routing", http.StatusInternalServerError)
		return nil
	}
	peerID := ingressFirstNonEmpty(msg.UserID, msg.ChannelID)
	peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
	reply, err := h.runChatTurn(r.Context(), chRow, "slack", peerKey, peerID, msg.Text)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		http.Error(w, "agent error", http.StatusInternalServerError)
		return nil
	}
	idempotency := "slack:" + msg.ChannelID + ":" + msg.EventTS
	if err := h.enqueueOutboundReply(r.Context(), chRow, "slack", msg.ChannelID, reply, nil, idempotency); err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		http.Error(w, "queue", http.StatusInternalServerError)
		return nil
	}
	_ = h.recordDelivery(r.Context(), chRow.ID, "queued", map[string]any{"channel_id": msg.ChannelID, "event_ts": msg.EventTS}, "")
	w.WriteHeader(http.StatusOK)
	return nil
}
