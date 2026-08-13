package service

import (
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/mattermost"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
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
	// Prefer HMAC signature verification; fallback to token verification only when
	// signing_secret is not configured. If signing_secret IS configured but the
	// X-Signature header is missing, reject the request to prevent downgrade attacks.
	signingSecret, secretErr := resolveCredentialPlain(r.Context(), h.channels, creds, "signing_secret")
	if secretErr != nil {
		h.lg.Warn("Mattermost signing_secret unresolved; falling back when empty",
			loggateway.StepID("channel.credential.resolve_fail"),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Str("credential_key", "signing_secret"),
			loggateway.Err(secretErr),
		)
	}
	if strings.TrimSpace(signingSecret) != "" {
		sigHeader := r.Header.Get("X-Signature")
		if strings.TrimSpace(sigHeader) == "" {
			h.lg.Warn("Mattermost Webhook 缺少签名头",
				loggateway.StepID("channel.mattermost.webhook.missing_signature"),
				loggateway.Str("channel_id", chRow.ID),
			)
			http.Error(w, "forbidden: missing signature", http.StatusForbidden)
			return nil
		}
		if err := mattermost.VerifySignature(signingSecret, raw, sigHeader); err != nil {
			h.lg.Warn("Mattermost Webhook HMAC 签名验证失败",
				loggateway.StepID("channel.mattermost.webhook.verify_fail"),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Err(err),
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
	} else {
		receiveToken, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "receive_token")
		if err := mattermost.VerifyToken(receiveToken, r.URL.Query().Get("token")); err != nil {
			h.lg.Warn("Mattermost Webhook token 验证失败",
				loggateway.StepID("channel.mattermost.webhook.verify_fail"),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Err(err),
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil
		}
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
			port.MetaRecipient: parsed.ChannelID,
			port.MetaChatID:    parsed.ChannelID,
		},
	}))
	return nil
}
