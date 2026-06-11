package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/teams"
	"aranea-agents/pkg/loggateway"
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
	appID, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "app_id", h.lg)
	appSecret, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "app_secret", h.lg)
	if err := teams.VerifyRequest(appID, appSecret, r.Header, raw); err != nil {
		h.lg.Warn("Teams Webhook 签名验证失败",
			loggateway.StepID("channel.teams.webhook.verify_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Err(err),
		)
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
			port.MetaRecipient:      parsed.ConversationID,
			port.MetaChatID:         parsed.ConversationID,
			port.MetaServiceURL:     parsed.ServiceURL,
			port.MetaConversationID: parsed.ConversationID,
		},
	}))
	return nil
}
