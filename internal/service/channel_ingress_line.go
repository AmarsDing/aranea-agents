package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/line"
	"aranea-agents/internal/channel/port"
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
	channelSecret, _ := resolveCredentialPlain(r.Context(), creds, "channel_secret")
	if err := line.VerifySignature(channelSecret, string(raw), r.Header.Get("X-Line-Signature")); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	msgs, err := line.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	for _, m := range msgs {
		peerID := ingressFirstNonEmpty(m.UserID, m.GroupID, m.RoomID)
		recipient := ingressFirstNonEmpty(m.GroupID, m.RoomID, m.UserID)
		meta := map[string]string{
			"recipient":   recipient,
			"chat_id":     recipient,
			"reply_token": m.ReplyToken,
		}
		writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
			PlatformType:   "line",
			PeerID:         peerID,
			Text:           m.Text,
			IdempotencyKey: "line:" + m.MessageID,
			OutboundMeta:   meta,
		}))
	}
	return nil
}
