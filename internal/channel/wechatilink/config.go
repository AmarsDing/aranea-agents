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
// 注意：iLink 网关要求请求体必须携带 ilink_user_id（未文档化），
// 缺省时返回 ret=-2 errmsg="ilink_user_id required"。
func (c *client) GetConfig(ctx context.Context, ilinkUserID string) (*getConfigResp, error) {
	req := struct {
		BaseInfo    baseInfo `json:"base_info"`
		ILinkUserID string   `json:"ilink_user_id"`
	}{BaseInfo: baseInfo{ChannelVersion: channelVersion}, ILinkUserID: ilinkUserID}
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
// ilinkUserID 为扫码登录时写入的凭据（getconfig 必传字段）。
func TestConnection(ctx context.Context, httpClient *http.Client, baseURL, botToken, ilinkUserID string, lg loggateway.Logger) error {
	if botToken == "" {
		return fmt.Errorf("wechat_ilink: bot_token not configured")
	}
	if ilinkUserID == "" {
		return fmt.Errorf("wechat_ilink: ilink_user_id not configured，请重新扫码登录")
	}
	c := newClient(baseURL, botToken, lg)
	if httpClient != nil {
		c.http = httpClient
	}
	_, err := c.GetConfig(ctx, ilinkUserID)
	return err
}
