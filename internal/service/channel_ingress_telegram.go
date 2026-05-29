package service

import (
	"io"
	"net/http"
	"strconv"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
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
	webhookSecret, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "webhook_secret")
	if err := telegram.VerifySecretToken(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), webhookSecret); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := telegram.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.Username, telegramChatRecipient(parsed.ChatID))
	recipient := telegramChatRecipient(parsed.ChatID)
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "telegram",
		PeerID:         peerID,
		Text:           parsed.Text,
		IdempotencyKey: "telegram:" + strconv.FormatInt(parsed.UpdateID, 10),
		OutboundMeta: map[string]string{
			"recipient": recipient,
			"chat_id":   recipient,
		},
	}))
	return nil
}
