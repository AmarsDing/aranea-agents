package wechatilink

import (
	"context"
	"fmt"
	"net/http"

	"aranea-agents/pkg/loggateway"
)

// getConfigResp is the response of POST /ilink/bot/getconfig.
type getConfigResp struct {
	Ret          int    `json:"ret"`
	TypingTicket string `json:"typing_ticket"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

// GetConfig fetches the bot's runtime config (typing ticket, etc.).
func (c *client) GetConfig(ctx context.Context) (*getConfigResp, error) {
	req := struct {
		BaseInfo baseInfo `json:"base_info"`
	}{BaseInfo: baseInfo{ChannelVersion: channelVersion}}
	resp, err := c.post(ctx, "/ilink/bot/getconfig", req)
	if err != nil {
		return nil, fmt.Errorf("wechat_ilink: getconfig: %w", err)
	}
	r, err := decodeJSON[getConfigResp](resp)
	if err != nil {
		return nil, err
	}
	if isSessionExpired(r.ErrCode) {
		return nil, ErrSessionExpired
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return nil, fmt.Errorf("wechat_ilink: getconfig failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return &r, nil
}

// TestConnection verifies bot_token validity via the read-only getconfig call
// (same probe pattern as telegram.GetMe / slack.AuthTest).
func TestConnection(ctx context.Context, httpClient *http.Client, baseURL, botToken string, lg loggateway.Logger) error {
	if botToken == "" {
		return fmt.Errorf("wechat_ilink: bot_token not configured")
	}
	c := newClient(baseURL, botToken, lg)
	if httpClient != nil {
		c.http = httpClient
	}
	_, err := c.GetConfig(ctx)
	return err
}
