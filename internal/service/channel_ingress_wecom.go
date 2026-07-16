package service

import (
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/wecom"
	"aranea-agents/pkg/loggateway"
)

func (h *ChannelIngress) handleWeComWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	token, ok := h.loadRequiredCredential(r.Context(), chRow.ID, creds, "token")
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	if err := wecom.VerifySignature(token, r.URL.Query().Get("timestamp"), r.URL.Query().Get("nonce"), r.URL.Query().Get("msg_signature")); err != nil {
		h.lg.Warn("WeCom Webhook 签名验证失败",
			loggateway.StepID("channel.wecom.webhook.verify_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Err(err),
		)
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := wecom.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	peerID := ingressFirstNonEmpty(parsed.SenderUserID, parsed.ChatID)
	// COR-05: prefer platform-provided msg_id; fall back to URL timestamp.
	var idempotency string
	if parsed.MsgID != "" {
		idempotency = "wecom:msg:" + parsed.MsgID
	} else {
		idempotency = "wecom:" + ingressFirstNonEmpty(parsed.ChatID, parsed.SenderUserID) + ":" + strings.TrimSpace(r.URL.Query().Get("timestamp"))
	}
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "wecom",
		PeerID:         peerID,
		Text:           parsed.Text,
		IdempotencyKey: idempotency,
		OutboundMeta: map[string]string{
			port.MetaRecipient:   parsed.ResponseURL,
			port.MetaResponseURL: parsed.ResponseURL,
		},
	}))
	return nil
}
