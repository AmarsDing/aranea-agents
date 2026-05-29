package service

import (
	"io"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/mattermost"
	"aranea-agents/internal/channel/port"
)

func (h *ChannelIngress) handleMattermostWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	receiveToken, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "receive_token")
	if err := mattermost.VerifyToken(receiveToken, r.URL.Query().Get("token")); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := mattermost.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.UserID, parsed.ChannelID)
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "mattermost",
		PeerID:         peerID,
		Text:           parsed.Text,
		IdempotencyKey: "mattermost:" + parsed.PostID,
		OutboundMeta: map[string]string{
			"recipient": parsed.ChannelID,
			"chat_id":   parsed.ChannelID,
		},
	}))
	return nil
}
