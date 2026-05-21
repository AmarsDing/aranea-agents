package service

import (
	"io"
	"net/http"
	"strconv"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/telegram"
)

func (h *ChannelIngress) handleTelegramWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	webhookSecret, _ := resolveCredentialPlain(r.Context(), creds, "webhook_secret")
	if err := telegram.VerifySecretToken(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), webhookSecret); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := telegram.ParseInbound(raw)
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
	peerID := ingressFirstNonEmpty(parsed.Username, telegramChatRecipient(parsed.ChatID))
	peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
	reply, err := h.runChatTurn(r.Context(), chRow, "telegram", peerKey, peerID, parsed.Text)
	if err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		http.Error(w, "agent error", http.StatusInternalServerError)
		return nil
	}
	idempotency := "telegram:" + strconv.FormatInt(parsed.UpdateID, 10)
	recipient := telegramChatRecipient(parsed.ChatID)
	if err := h.enqueueOutboundReply(r.Context(), chRow, "telegram", recipient, reply, nil, idempotency); err != nil {
		_ = h.recordDelivery(r.Context(), chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		http.Error(w, "queue", http.StatusInternalServerError)
		return nil
	}
	_ = h.recordDelivery(r.Context(), chRow.ID, "queued", map[string]any{"update_id": parsed.UpdateID, "chat_id": parsed.ChatID}, "")
	w.WriteHeader(http.StatusOK)
	return nil
}
