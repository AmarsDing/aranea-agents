package service

import (
	"io"
	"net/http"
	"strconv"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/telegram"
	"aranea-agents/pkg/loggateway"
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
	webhookSecret, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "webhook_secret", h.lg)
	if err := telegram.VerifySecretToken(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), webhookSecret); err != nil {
		h.lg.Warn("Telegram Webhook 签名验证失败",
			loggateway.StepID("channel.telegram.webhook.verify_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Err(err),
		)
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
			port.MetaRecipient: recipient,
			port.MetaChatID:    recipient,
		},
	}))
	return nil
}
