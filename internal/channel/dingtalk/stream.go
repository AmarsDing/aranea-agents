package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	dingUtils "github.com/open-dingtalk/dingtalk-stream-sdk-go/utils"
)

func init() {
	runtime.RegisterStarterWithLogger("dingtalk", "stream", RunStream)
}

func RunStream(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("钉钉 Stream 连接器启动",
		loggateway.StepID("channel.dingtalk.stream.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	clientID, clientSecret, err := dingStreamCreds(ctx, ch, creds, lookup, lg)
	if err != nil {
		lg.Error("钉钉 Stream 凭据获取失败",
			loggateway.StepID("channel.dingtalk.stream.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		runtime.EmitConnectError(ctx, "dingtalk", ch.ID, "钉钉 Stream 凭据获取失败", err)
		return err
	}
	chRow := ch
	onChat := func(ctx context.Context, message *chatbot.BotCallbackDataModel) ([]byte, error) {
		ev, ok := parseStreamMessage(message, lg)
		if !ok {
			return nil, nil
		}
		ev.PlatformType = "dingtalk"
		if err := handler.ProcessInbound(ctx, chRow, ev); err != nil {
			lg.Warn("钉钉入站处理失败",
				loggateway.StepID("channel.dingtalk.inbound_failed"),
				loggateway.Err(err),
			)
			// Return error to SDK so it can retry delivery.
			// This prevents silent message loss on transient failures.
			return nil, err
		}
		return nil, nil
	}
	streamClient := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
		client.WithUserAgent(client.NewDingtalkGoSDKUserAgent()),
		// SDK 内部重连使用 context.Background() 无法取消，会与 supervisor 的
		// 统一重连（backoff/lease/fingerprint）冲突并泄漏旧连接；禁用后由
		// supervisor 负责重连，与其他平台连接器语义一致。
		client.WithAutoReconnect(false),
		client.WithSubscription(dingUtils.SubscriptionTypeKCallback, "/v1.0/im/bot/messages/get",
			chatbot.NewDefaultChatBotFrameHandler(onChat).OnEventReceived),
	)
	// Start 拨号成功即返回（读循环在 SDK goroutine 内），因此这里阻塞到
	// ctx 取消，保证连接生命周期与 supervisor 托管一致。
	if err := streamClient.Start(ctx); err != nil {
		lg.Error("钉钉 Stream 连接失败",
			loggateway.StepID("channel.dingtalk.stream.connect_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		runtime.EmitConnectError(ctx, "dingtalk", ch.ID, "钉钉 Stream 连接失败", err)
		return err
	}
	lg.Info("钉钉 Stream 已连接",
		loggateway.StepID("channel.dingtalk.stream.connected"),
		loggateway.Str("channel_id", ch.ID),
		loggateway.Str("client_id", clientID),
	)
	runtime.EmitConnectOpen(ctx, "dingtalk", ch.ID, clientID, "钉钉 Stream 已连接")
	<-ctx.Done()
	streamClient.Close()
	return ctx.Err()
}

func dingStreamCreds(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup, lg loggateway.Logger) (string, string, error) {
	var cfg struct {
		Config struct {
			ClientID string `json:"client_id"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(ch.ConfigJSON), &cfg); err != nil {
		return "", "", apierror.BadRequest("DINGTALK_CONFIG", fmt.Sprintf("dingtalk stream: parse config: %s", err.Error()))
	}
	clientID := strings.TrimSpace(cfg.Config.ClientID)
	if clientID == "" {
		s, _ := lookup(ctx, creds, "client_id")
		clientID = strings.TrimSpace(s)
	}
	secret, lookupErr := lookup(ctx, creds, "client_secret")
	secret = strings.TrimSpace(secret)
	if clientID == "" || secret == "" {
		return "", "", apierror.BadRequest("DINGTALK_CONFIG", "dingtalk stream: client_id and client_secret required")
	}
	// Credentials validated; discard lookupErr to avoid returning an error
	// alongside valid credentials, which would cause the caller to fail
	// even though the credentials are usable.
	_ = lookupErr
	return clientID, secret, nil
}

func parseStreamMessage(message *chatbot.BotCallbackDataModel, lg loggateway.Logger) (port.InboundEvent, bool) {
	if message == nil {
		return port.InboundEvent{}, false
	}
	raw, _ := json.Marshal(message)
	var generic struct {
		Text struct {
			Content string `json:"content"`
		} `json:"text"`
		SenderStaffId  string `json:"senderStaffId"`
		ConversationId string `json:"conversationId"`
		SessionWebhook string `json:"sessionWebhook"`
		MsgId          string `json:"msgId"`
		CreateAt       int64  `json:"createAt"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		lg.Warn("解析 dingtalk stream message 失败", loggateway.StepID("channel.dingtalk.parse"), loggateway.Err(err))
	}
	text := strings.TrimSpace(generic.Text.Content)
	if text == "" {
		return port.InboundEvent{}, false
	}
	peerID := port.FirstNonEmpty(generic.SenderStaffId, generic.ConversationId)
	// IdempotencyKey must be message-level unique to avoid deduplicating
	// different messages within the same conversation.
	// Primary: msgId (DingTalk message-level unique ID).
	// Fallback: conversationId + senderStaffId + createAt composite key.
	idempotencyKey := strings.TrimSpace(generic.MsgId)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s:%s:%d", generic.ConversationId, generic.SenderStaffId, generic.CreateAt)
	}
	return port.InboundEvent{
		PeerID:         peerID,
		Text:           text,
		IdempotencyKey: "dingtalk:" + idempotencyKey,
		OutboundMeta: map[string]string{
			port.MetaSessionWebhook: generic.SessionWebhook,
			port.MetaRecipient:      generic.SessionWebhook,
		},
	}, true
}
