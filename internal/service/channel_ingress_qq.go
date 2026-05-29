package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/qq"

	qqwebhook "github.com/tencent-connect/botgo/interaction/webhook"
)

func (h *ChannelIngress) handleQQWebhook(w http.ResponseWriter, r *http.Request, chRow biz.Channel) error {
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
	appSecret, _ := resolveCredentialPlain(r.Context(), h.channels, creds, "app_secret")
	if err := qq.VerifyRequest(appSecret, r.Header, raw); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	parsed, err := qq.ParseWebhook(raw, r.Header, appSecret)
	if err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return nil
	}
	if len(parsed.ValidationBody) > 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(parsed.ValidationBody)
		return nil
	}
	if parsed.HeartbeatBody != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(parsed.HeartbeatBody))
		return nil
	}
	if parsed.Message == nil {
		if parsed.DispatchACK != "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(parsed.DispatchACK))
		} else {
			w.WriteHeader(http.StatusOK)
		}
		return nil
	}
	msg := parsed.Message
	meta := map[string]string{
		"recipient": msg.UserID,
		"group_id":  msg.GroupID,
	}
	if msg.GroupID != "" {
		meta["recipient"] = msg.GroupID
	}
	idem := "qq:" + strings.TrimSpace(msg.MessageID)
	if idem == "qq:" {
		idem = "qq:" + strings.TrimSpace(msg.EventID)
	}
	ev := port.InboundEvent{
		PlatformType:   "qq",
		PeerID:         msg.PeerID,
		Text:           msg.Text,
		IdempotencyKey: idem,
		OutboundMeta:   meta,
	}
	result := h.processInboundHTTP(r, chRow, ev)
	if parsed.DispatchACK != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if result.Err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(qqwebhook.GenDispatchACK(false)))
			return nil
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(parsed.DispatchACK))
		return nil
	}
	writeInboundHTTPResponse(w, result)
	return nil
}

func qqAppID(configJSON string) string {
	var env struct {
		Config struct {
			AppID string `json:"app_id"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(configJSON), &env)
	return strings.TrimSpace(env.Config.AppID)
}
