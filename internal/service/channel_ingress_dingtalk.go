package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/dingtalk"
	"aranea-agents/internal/channel/port"
)

func (h *ChannelIngress) handleDingTalkWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	secret, _ := resolveCredentialPlain(r.Context(), creds, "secret")
	if err := dingtalk.VerifySign(r.URL.Query().Get("timestamp"), r.URL.Query().Get("sign"), secret); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := dingtalk.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.SenderStaffID, parsed.ConversationID, parsed.SenderNick)
	// COR-05: prefer platform-provided msg_id, then body createAt, then URL
	// timestamp (weakest — shared across concurrent requests in the same second).
	var idempotency string
	if parsed.MsgID != "" {
		idempotency = "dingtalk:msg:" + parsed.MsgID
	} else if parsed.CreateAt > 0 {
		idempotency = "dingtalk:" + parsed.ConversationID + ":" + fmt.Sprintf("%d", parsed.CreateAt)
	} else {
		idempotency = "dingtalk:" + parsed.ConversationID + ":" + strings.TrimSpace(r.URL.Query().Get("timestamp"))
	}
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "dingtalk",
		PeerID:         peerID,
		Text:           parsed.Text,
		IdempotencyKey: idempotency,
		OutboundMeta: map[string]string{
			"recipient":       parsed.SessionWebhook,
			"session_webhook": parsed.SessionWebhook,
		},
	}))
	return nil
}
