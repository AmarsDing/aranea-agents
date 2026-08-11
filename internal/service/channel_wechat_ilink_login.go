package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/wechatilink"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// qrLoginPollDeadline bounds the background QR-status polling goroutine.
const qrLoginPollDeadline = 3 * time.Minute

// WechatILinkLogin starts a QR-code login flow: returns a fresh QR code and
// spawns a background goroutine that waits for scan confirmation, then writes
// the bot_token credential and reloads the runtime.
func (s *ChannelService) WechatILinkLogin(ctx context.Context, req *v1.WechatILinkLoginRequest) (*v1.WechatILinkLoginResponse, error) {
	channelID := strings.TrimSpace(req.GetChannelId())
	if err := s.assertChannelMutateAccess(ctx, channelID); err != nil {
		return nil, err
	}
	cl := wechatilink.NewLoginClient("", s.lg)
	resp, err := cl.GetBotQRCode(ctx)
	if err != nil {
		return nil, err
	}
	s.lg.Info("WeChat iLink 扫码登录发起",
		loggateway.StepID("channel.wechat_ilink.login.start"),
		loggateway.Str("channel_id", channelID),
	)
	safego.Go(context.WithoutCancel(ctx), "channel.wechat_ilink.login_poll", func() {
		s.pollWechatILinkQRStatus(context.Background(), channelID, resp.QRCode)
	})
	return &v1.WechatILinkLoginResponse{
		QrcodeDataUrl: resp.QRCodeImgContent,
		QrcodeSession: resp.QRCode,
		Status:        wechatilink.QRStatusWait,
	}, nil
}

// pollWechatILinkQRStatus waits for the user to scan + confirm, then persists
// credentials and triggers a runtime reload so the polling connector starts.
func (s *ChannelService) pollWechatILinkQRStatus(ctx context.Context, channelID, qrcode string) {
	cl := wechatilink.NewLoginClient("", s.lg)
	deadline := time.Now().Add(qrLoginPollDeadline)
	for time.Now().Before(deadline) {
		status, err := cl.GetQRCodeStatus(ctx, qrcode)
		if err != nil {
			s.lg.Warn("WeChat iLink 扫码状态轮询失败",
				loggateway.StepID("channel.wechat_ilink.login.poll_fail"),
				loggateway.Str("channel_id", channelID),
				loggateway.Err(err),
			)
		} else {
			switch status.Status {
			case wechatilink.QRStatusConfirmed:
				s.completeWechatILinkLogin(ctx, channelID, status.BotToken, status.BaseURL, status.ILinkUserID)
				return
			case wechatilink.QRStatusExpired:
				s.lg.Info("WeChat iLink 二维码已过期",
					loggateway.StepID("channel.wechat_ilink.login.expired"),
					loggateway.Str("channel_id", channelID),
				)
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	s.lg.Warn("WeChat iLink 扫码登录超时放弃",
		loggateway.StepID("channel.wechat_ilink.login.timeout"),
		loggateway.Str("channel_id", channelID),
	)
}

func (s *ChannelService) completeWechatILinkLogin(ctx context.Context, channelID, botToken, baseURL, ilinkUserID string) {
	_, err := s.uc.UpsertCredentials(ctx, channelID, []biz.ChannelCredentialInput{
		{CredentialKey: "bot_token", Secret: botToken},
		{CredentialKey: "baseurl", Secret: baseURL},
		{CredentialKey: "ilink_user_id", Secret: ilinkUserID},
	})
	if err != nil {
		s.lg.Error("WeChat iLink 扫码登录凭证写入失败",
			loggateway.StepID("channel.wechat_ilink.login.creds_fail"),
			loggateway.Str("channel_id", channelID),
			loggateway.Err(err),
		)
		return
	}
	s.lg.Info("WeChat iLink 扫码登录完成",
		loggateway.StepID("channel.wechat_ilink.login.confirmed"),
		loggateway.Str("channel_id", channelID),
	)
	s.reloadRuntime(ctx)
}

// WechatILinkPoll reports whether the QR login has completed (credentials written).
func (s *ChannelService) WechatILinkPoll(ctx context.Context, req *v1.WechatILinkPollRequest) (*v1.WechatILinkPollResponse, error) {
	channelID := strings.TrimSpace(req.GetChannelId())
	if err := s.assertChannelAccess(ctx, channelID); err != nil {
		return nil, err
	}
	creds, err := s.uc.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if c.CredentialKey == "bot_token" && strings.TrimSpace(c.SecretRef) != "" && strings.TrimSpace(c.DeletedAt) == "" {
			return &v1.WechatILinkPollResponse{Status: wechatilink.QRStatusConfirmed}, nil
		}
	}
	return &v1.WechatILinkPollResponse{Status: wechatilink.QRStatusWait}, nil
}
