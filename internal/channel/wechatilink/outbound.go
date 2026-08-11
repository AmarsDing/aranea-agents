package wechatilink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"aranea-agents/pkg/loggateway"
)

type sendMessageReq struct {
	BaseInfo baseInfo          `json:"base_info"`
	Msg      WeixinSendMessage `json:"msg"`
}

type sendMessageResp struct {
	Ret     int    `json:"ret"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (c *client) SendMessage(ctx context.Context, msg *WeixinSendMessage) error {
	req := sendMessageReq{BaseInfo: baseInfo{ChannelVersion: channelVersion}, Msg: *msg}
	resp, err := c.post(ctx, "/ilink/bot/sendmessage", req)
	if err != nil {
		return fmt.Errorf("wechat_ilink: sendmessage: %w", err)
	}
	r, err := decodeJSON[sendMessageResp](resp)
	if err != nil {
		return err
	}
	if isSessionExpired(r.ErrCode) {
		return ErrSessionExpired
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return fmt.Errorf("wechat_ilink: sendmessage failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return nil
}

// TextSender sends text replies via iLink sendmessage. Constructed per send
// with channel credentials (same pattern as telegram.TextSender).
type TextSender struct {
	BotToken string
	BaseURL  string // optional; default ilinkai.weixin.qq.com
	// ContextToken echoes the inbound message's context_token so the reply
	// lands in the correct conversation window.
	ContextToken string
	HTTP         *http.Client
	Lg           loggateway.Logger
}

// ID identifies the platform for the channel.OutboundText contract.
func (s *TextSender) ID() string { return "wechat_ilink" }

// SendText sends a text reply to recipient.
func (s *TextSender) SendText(ctx context.Context, recipient, text string) error {
	if s.BotToken == "" {
		return fmt.Errorf("wechat_ilink: bot_token not configured")
	}
	c := newClient(s.BaseURL, s.BotToken, s.Lg)
	if s.HTTP != nil {
		c.http = s.HTTP
	}
	msg := WeixinSendMessage{
		ToUserID:     recipient,
		ClientID:     newClientID(),
		MessageType:  MessageTypeBot,
		MessageState: MessageStateFinish,
		ContextToken: s.ContextToken,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: text}}},
	}
	return c.SendMessage(ctx, &msg)
}

// newClientID returns a random client_id for outbound messages.
func newClientID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "aranea-" + hex.EncodeToString(b[:])
}
