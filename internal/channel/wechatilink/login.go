package wechatilink

import (
	"context"
	"fmt"
	"net/url"
)

// getBotQRCodeResp is the response of GET /ilink/bot/get_bot_qrcode.
type getBotQRCodeResp struct {
	Ret              int    `json:"ret"`
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"` // 扫码内容（当前为 liteapp URL，需自行编码成二维码图），非图片 data URL
	ErrCode          int    `json:"errcode"`
	ErrMsg           string `json:"errmsg"`
}

// QR login status values.
const (
	QRStatusWait      = "wait"
	QRStatusScanned   = "scaned" // iLink's own spelling
	QRStatusConfirmed = "confirmed"
	QRStatusExpired   = "expired"
)

// getQRCodeStatusResp is the response of GET /ilink/bot/get_qrcode_status.
type getQRCodeStatusResp struct {
	Ret         int    `json:"ret"`
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	BaseURL     string `json:"baseurl"`
	ILinkUserID string `json:"ilink_user_id"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

// GetBotQRCode fetches a fresh login QR code (bot_type=3: personal bot).
func (lc *LoginClient) GetBotQRCode(ctx context.Context) (*getBotQRCodeResp, error) {
	resp, err := lc.get(ctx, "/ilink/bot/get_bot_qrcode?bot_type=3")
	if err != nil {
		return nil, fmt.Errorf("wechat_ilink: get_bot_qrcode: %w", err)
	}
	r, err := decodeJSON[getBotQRCodeResp](resp)
	if err != nil {
		return nil, err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return nil, fmt.Errorf("wechat_ilink: get_bot_qrcode failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return &r, nil
}

// GetQRCodeStatus long-polls the QR scan status (server holds up to ~35s).
func (lc *LoginClient) GetQRCodeStatus(ctx context.Context, qrcode string) (*getQRCodeStatusResp, error) {
	resp, err := lc.get(ctx, "/ilink/bot/get_qrcode_status?qrcode="+url.QueryEscape(qrcode))
	if err != nil {
		return nil, fmt.Errorf("wechat_ilink: get_qrcode_status: %w", err)
	}
	r, err := decodeJSON[getQRCodeStatusResp](resp)
	if err != nil {
		return nil, err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return nil, fmt.Errorf("wechat_ilink: get_qrcode_status failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return &r, nil
}
