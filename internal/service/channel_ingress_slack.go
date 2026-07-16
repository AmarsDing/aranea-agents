package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/pkg/loggateway"
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
	signingSecret, ok := h.loadRequiredCredential(r.Context(), chRow.ID, creds, "signing_secret")
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	if err := slack.VerifyRequest(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), signingSecret, raw); err != nil {
		h.lg.Warn("Slack Webhook 签名验证失败",
			loggateway.StepID("channel.slack.webhook.verify_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Err(err),
		)
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
	peerID := ingressFirstNonEmpty(msg.UserID, msg.ChannelID)
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "slack",
		PeerID:         peerID,
		Text:           msg.Text,
		IdempotencyKey: "slack:" + msg.ChannelID + ":" + msg.EventTS,
		OutboundMeta: map[string]string{
			port.MetaRecipient: msg.ChannelID,
			"channel":          msg.ChannelID,
		},
	}))
	return nil
}
