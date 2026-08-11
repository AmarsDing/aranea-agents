package wechatilink

import (
	"context"
	"fmt"
)

// sendtyping status values.
const (
	typingStatusOn  = 1
	typingStatusOff = 2
)

type sendTypingReq struct {
	BaseInfo     baseInfo `json:"base_info"`
	ILinkUserID  string   `json:"ilink_user_id"`
	TypingTicket string   `json:"typing_ticket"`
	Status       int      `json:"status"`
}

type sendTypingResp struct {
	Ret     int    `json:"ret"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// SendTyping shows/cancels the "typing..." indicator in the user's chat
// window. ticket comes from GetConfig (typing_ticket); ilinkUserID is the
// bot's own ilink user id stored in credentials at login.
func (c *client) SendTyping(ctx context.Context, ilinkUserID, ticket string, typing bool) error {
	status := typingStatusOff
	if typing {
		status = typingStatusOn
	}
	req := sendTypingReq{
		BaseInfo:     baseInfo{ChannelVersion: channelVersion},
		ILinkUserID:  ilinkUserID,
		TypingTicket: ticket,
		Status:       status,
	}
	resp, err := c.post(ctx, "/ilink/bot/sendtyping", req)
	if err != nil {
		return fmt.Errorf("wechat_ilink: sendtyping: %w", err)
	}
	r, err := decodeJSON[sendTypingResp](resp)
	if err != nil {
		return err
	}
	if isSessionExpired(r.ErrCode) {
		return ErrSessionExpired
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return fmt.Errorf("wechat_ilink: sendtyping failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return nil
}
