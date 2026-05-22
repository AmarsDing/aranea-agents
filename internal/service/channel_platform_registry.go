package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/discord"
	"aranea-agents/internal/channel/dingtalk"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/onebot"
	"aranea-agents/internal/channel/qq"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/internal/channel/telegram"
	"aranea-agents/internal/channel/wechat"
	"aranea-agents/internal/channel/wecom"
)

type outboundHandler func(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error

type streamFactory func(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, meta map[string]string) (streamPreviewUpdater, error)

type platformAdapter struct {
	outbound outboundHandler
	stream   streamFactory
}

var platformAdapters = map[string]platformAdapter{}

func registerPlatform(platform string, outbound outboundHandler, stream streamFactory) {
	platformAdapters[strings.ToLower(strings.TrimSpace(platform))] = platformAdapter{
		outbound: outbound,
		stream:   stream,
	}
}

func init() {
	registerPlatform("feishu", outboundFeishu, streamFeishu)
	registerPlatform("dingtalk", outboundDingtalk, nil)
	registerPlatform("wecom", outboundWecom, nil)
	registerPlatform("wecom-app", outboundWecom, nil)
	registerPlatform("slack", outboundSlack, streamSlack)
	registerPlatform("telegram", outboundTelegram, streamTelegram)
	registerPlatform("discord", outboundDiscord, nil)
	registerPlatform("personal_qq", outboundPersonalQQ, nil)
	registerPlatform("wechat", outboundWechat, nil)
	registerPlatform("qq", outboundQQ, nil)
}

func outboundFeishu(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
	if err != nil {
		return err
	}
	sec, err := resolveCredentialPlain(ctx, creds, "app_secret")
	if err != nil {
		return err
	}
	return (&lark.FeishuTextSender{
		Region:        region,
		AppID:         appID,
		AppSecret:     sec,
		ReceiveIDType: lark.ReceiveIDTypeFromMeta(payload.Extra),
		HTTP:          h.http,
	}).SendText(ctx, payload.Recipient, payload.Text)
}

func outboundDingtalk(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	secret, _ := resolveCredentialPlain(ctx, creds, "secret")
	webhookURL, _ := resolveCredentialPlain(ctx, creds, "webhook_url")
	target := payload.Extra["session_webhook"]
	return (&dingtalk.TextSender{WebhookURL: webhookURL, Secret: secret, HTTP: h.http}).SendText(ctx, target, payload.Text)
}

func outboundWecom(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	webhookURL, _ := resolveCredentialPlain(ctx, creds, "webhook_url")
	target := payload.Extra["response_url"]
	return (&wecom.TextSender{WebhookURL: webhookURL, HTTP: h.http}).SendText(ctx, target, payload.Text)
}

func outboundSlack(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	token, err := resolveCredentialPlain(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	return (&slack.TextSender{BotToken: token, HTTP: h.http}).SendText(ctx, payload.Recipient, payload.Text)
}

func outboundTelegram(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	token, err := resolveCredentialPlain(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	return (&telegram.TextSender{BotToken: token, HTTP: h.http}).SendText(ctx, payload.Recipient, payload.Text)
}

func outboundDiscord(ctx context.Context, _ *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	token, err := resolveCredentialPlain(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	return (&discord.TextSender{BotToken: token}).SendText(ctx, payload.Recipient, payload.Text)
}

func outboundPersonalQQ(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	sendToken, _ := resolveCredentialPlain(ctx, creds, "send_token")
	httpServer := oneBotHTTPServer(chRow.ConfigJSON)
	return (&onebot.TextSender{
		HTTPServer: httpServer,
		SendToken:  sendToken,
		HTTP:       h.http,
	}).SendText(ctx, payload.Recipient, payload.Text, payload.Extra["group_id"])
}

func outboundWechat(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	appID, appSecret := wechatAppCreds(chRow.ConfigJSON, creds, ctx)
	return (&wechat.TextSender{
		AppID:     appID,
		AppSecret: appSecret,
		HTTP:      h.http,
	}).SendText(ctx, payload.Recipient, payload.Text)
}

func outboundQQ(ctx context.Context, _ *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	appSecret, err := resolveCredentialPlain(ctx, creds, "app_secret")
	if err != nil {
		return err
	}
	return (&qq.TextSender{
		AppID:     qqAppID(chRow.ConfigJSON),
		AppSecret: appSecret,
	}).SendText(ctx, payload.Recipient, payload.Text, payload.Extra)
}

func streamTelegram(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, _ map[string]string) (streamPreviewUpdater, error) {
	token, err := resolveCredentialPlain(ctx, creds, "bot_token")
	if err != nil {
		return nil, err
	}
	return &telegram.StreamSender{BotToken: token, HTTP: h.http}, nil
}

func streamFeishu(ctx context.Context, h *ChannelIngress, chRow biz.Channel, creds []biz.ChannelCredential, meta map[string]string) (streamPreviewUpdater, error) {
	region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
	if err != nil {
		return nil, err
	}
	sec, err := resolveCredentialPlain(ctx, creds, "app_secret")
	if err != nil {
		return nil, err
	}
	return &lark.StreamSender{
		Region:        region,
		AppID:         appID,
		AppSecret:     sec,
		HTTP:          h.http,
		ReceiveIDType: lark.ReceiveIDTypeFromMeta(meta),
	}, nil
}

func streamSlack(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, _ map[string]string) (streamPreviewUpdater, error) {
	token, err := resolveCredentialPlain(ctx, creds, "bot_token")
	if err != nil {
		return nil, err
	}
	return &slack.StreamSender{BotToken: token, HTTP: h.http}, nil
}

func (h *ChannelIngress) newStreamUpdater(ctx context.Context, chRow biz.Channel, platform string, meta map[string]string) (streamPreviewUpdater, error) {
	creds, err := h.channels.ListCredentialsRaw(ctx, chRow.ID)
	if err != nil {
		return nil, err
	}
	adap, ok := platformAdapters[strings.ToLower(strings.TrimSpace(platform))]
	if !ok || adap.stream == nil {
		return nil, nil
	}
	return adap.stream(ctx, h, chRow, creds, meta)
}

func (h *ChannelIngress) sendOutboundPayload(ctx context.Context, channelID string, payload biz.ChannelOutboundPayload) error {
	chRow, err := h.channels.Get(ctx, channelID)
	if err != nil {
		return err
	}
	creds, err := h.channels.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return err
	}
	adap, ok := platformAdapters[strings.ToLower(strings.TrimSpace(payload.Platform))]
	if !ok || adap.outbound == nil {
		return fmt.Errorf("unsupported outbound platform %q", payload.Platform)
	}
	return adap.outbound(ctx, h, chRow, creds, payload)
}

func streamPlatformSupported(platform string) bool {
	adap, ok := platformAdapters[strings.ToLower(strings.TrimSpace(platform))]
	return ok && adap.stream != nil
}
