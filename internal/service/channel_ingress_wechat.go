package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/onebot"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/wechat"
)

func (h *ChannelIngress) handleWeChatWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
	q := r.URL.Query()
	creds, err := h.channels.ListCredentialsRaw(r.Context(), chRow.ID)
	if err != nil {
		http.Error(w, "credentials", http.StatusInternalServerError)
		return nil
	}
	token, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "token")
	if r.Method == http.MethodGet {
		echo, err := wechat.VerifyURL(token, q.Get("timestamp"), q.Get("nonce"), q.Get("echostr"), q.Get("signature"))
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
		_, _ = w.Write([]byte(echo))
		return nil
	}
	raw, err := wechat.ReadBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return nil
	}
	if err := wechat.VerifyPOST(token, q.Get("timestamp"), q.Get("nonce"), q.Get("signature")); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := wechat.ParseTextInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	ev := port.InboundEvent{
		PlatformType:   "wechat",
		PeerID:         parsed.FromUser,
		Text:           parsed.Content,
		IdempotencyKey: fmt.Sprintf("wechat:%d", parsed.MsgID),
		OutboundMeta: map[string]string{
			"recipient": parsed.FromUser,
			"to_user":   parsed.ToUser,
		},
	}
	if wechat.ActiveModeFromConfig(chRow.ConfigJSON) {
		writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, ev))
		return nil
	}
	reply, err := h.processWeChatPassiveInbound(r.Context(), chRow, ev)
	if err != nil {
		http.Error(w, "agent error", http.StatusInternalServerError)
		return nil
	}
	if strings.TrimSpace(reply) == "" {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(wechat.ReplyXML(parsed.FromUser, parsed.ToUser, reply)))
	return nil
}

func (h *ChannelIngress) handleOneBotWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	if err := onebot.VerifySignature(receiveToken, raw, r.Header.Get("X-Signature")); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := onebot.ParseInbound(raw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	meta := map[string]string{
		"recipient": parsed.UserID,
		"group_id":  parsed.GroupID,
	}
	if parsed.GroupID != "" {
		meta["recipient"] = parsed.GroupID
	}
	writeInboundHTTPResponse(w, h.processInboundHTTP(r, chRow, port.InboundEvent{
		PlatformType:   "personal_qq",
		PeerID:         parsed.PeerID,
		Text:           parsed.Text,
		IdempotencyKey: "onebot:" + strings.TrimSpace(parsed.MessageID),
		OutboundMeta:   meta,
	}))
	return nil
}
