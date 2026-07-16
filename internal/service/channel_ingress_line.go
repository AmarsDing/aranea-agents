package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/line"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

func (h *ChannelIngress) handleLINEWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	channelSecret, ok := h.loadRequiredCredential(r.Context(), chRow.ID, creds, "channel_secret")
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	if err := line.VerifySignature(channelSecret, raw, r.Header.Get("X-Line-Signature")); err != nil {
		h.lg.Warn("LINE Webhook 签名验证失败",
			loggateway.StepID("channel.line.webhook.verify_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Err(err),
		)
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	msgs, err := line.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	// Accept every event first, then write a single HTTP response.
	// Writing per-message would call WriteHeader multiple times and drop later events.
	result := inboundHTTPResult{}
	for _, m := range msgs {
		peerID := ingressFirstNonEmpty(m.UserID, m.GroupID, m.RoomID)
		recipient := ingressFirstNonEmpty(m.GroupID, m.RoomID, m.UserID)
		meta := map[string]string{
			port.MetaRecipient:  recipient,
			port.MetaChatID:     recipient,
			port.MetaReplyToken: m.ReplyToken,
		}
		one := h.processInboundHTTP(r, chRow, port.InboundEvent{
			PlatformType:   "line",
			PeerID:         peerID,
			Text:           m.Text,
			IdempotencyKey: "line:" + m.MessageID,
			OutboundMeta:   meta,
		})
		if one.Err != nil {
			result.Err = one.Err
			h.lg.Warn("LINE inbound accept failed",
				loggateway.StepID("channel.line.webhook.accept_fail"),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Str("message_id", m.MessageID),
				loggateway.Err(one.Err),
			)
		}
	}
	writeInboundHTTPResponse(w, result)
	return nil
}
