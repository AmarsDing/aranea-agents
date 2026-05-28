package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/teams"
)

func (h *ChannelIngress) handleTeamsWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	appID, _ := resolveCredentialPlain(r.Context(), creds, "app_id")
	appSecret, _ := resolveCredentialPlain(r.Context(), creds, "app_secret")
	if err := teams.VerifyRequest(appID, appSecret, r.Header, raw); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := teams.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.FromID, parsed.ConversationID)
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "teams",
		PeerID:         peerID,
		Text:           parsed.Text,
		IdempotencyKey: "teams:" + parsed.ActivityID,
		OutboundMeta: map[string]string{
			"recipient":        parsed.ConversationID,
			"chat_id":          parsed.ConversationID,
			"service_url":      parsed.ServiceURL,
			"conversation_id":  parsed.ConversationID,
		},
	}))
	return nil
}
