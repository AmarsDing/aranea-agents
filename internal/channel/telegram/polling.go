package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func init() {
	runtime.RegisterStarterWithLogger("telegram", "polling", RunPolling)
}

func RunPolling(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("Telegram Polling 连接器启动",
		loggateway.StepID("channel.telegram.polling.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	token, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		lg.Error("Telegram 凭据获取失败",
			loggateway.StepID("channel.telegram.polling.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errBotTokenRequired
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		apiErr := telegramAPIError("telegram polling: new bot", err.Error())
		runtime.EmitConnectError(ctx, "telegram", ch.ID, "Telegram Bot API 连接失败", apiErr)
		return apiErr
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	lg.Info("Telegram Polling 已连接",
		loggateway.StepID("channel.telegram.polling.connected"),
		loggateway.Str("channel_id", ch.ID),
		loggateway.Str("bot_username", bot.Self.UserName),
	)
	runtime.EmitConnectOpen(ctx, "telegram", ch.ID, bot.Self.UserName, "Telegram Polling 已连接")
	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			ev, ok := parsePollingUpdate(update)
			if !ok || strings.TrimSpace(ev.Text) == "" {
				continue
			}
			ev.PlatformType = "telegram"
			if err := handler.ProcessInbound(ctx, ch, ev); err != nil {
				lg.Warn("Telegram 入站处理失败",
					loggateway.StepID("channel.telegram.inbound_failed"),
					loggateway.Err(err),
				)
			}
		}
	}
}

func parsePollingUpdate(update tgbotapi.Update) (port.InboundEvent, bool) {
	if update.Message == nil {
		return port.InboundEvent{}, false
	}
	msg := update.Message
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	text := strings.TrimSpace(msg.Text)
	peerID := chatID
	if msg.From != nil && msg.From.ID != 0 {
		peerID = strconv.FormatInt(msg.From.ID, 10)
	}
	return port.InboundEvent{
		PeerID: peerID,
		Text:   text,
		// Telegram MessageID is unique per chat but NOT globally unique.
		// Use chat_id:message_id composite key to ensure cross-chat uniqueness.
		IdempotencyKey: fmt.Sprintf("telegram:%s:%d", chatID, msg.MessageID),
		OutboundMeta: map[string]string{
			port.MetaRecipient: chatID,
			port.MetaChatID:    chatID,
		},
	}, true
}
