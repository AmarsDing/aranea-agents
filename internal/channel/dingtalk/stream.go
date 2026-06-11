package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

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
		}
		return nil, nil
	}
	streamClient := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
		client.WithUserAgent(client.NewDingtalkGoSDKUserAgent()),
		client.WithSubscription(dingUtils.SubscriptionTypeKCallback, "/v1.0/im/bot/messages/get",
			chatbot.NewDefaultChatBotFrameHandler(onChat).OnEventReceived),
	)
	return streamClient.Start(ctx)
}

func dingStreamCreds(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup, lg loggateway.Logger) (string, string, error) {
	var cfg struct {
		Config struct {
			ClientID string `json:"client_id"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(ch.ConfigJSON), &cfg); err != nil {
		return "", "", kerrors.BadRequest("DINGTALK_CONFIG", fmt.Sprintf("dingtalk stream: parse config: %s", err.Error()))
	}
	clientID := strings.TrimSpace(cfg.Config.ClientID)
	if clientID == "" {
		s, _ := lookup(ctx, creds, "client_id")
		clientID = strings.TrimSpace(s)
	}
	secret, err := lookup(ctx, creds, "client_secret")
	secret = strings.TrimSpace(secret)
	if clientID == "" || secret == "" {
		return "", "", kerrors.BadRequest("DINGTALK_CONFIG", "dingtalk stream: client_id and client_secret required")
	}
	return clientID, secret, err
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
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		lg.Warn("解析 dingtalk stream message 失败", loggateway.StepID("channel.dingtalk.parse"), loggateway.Err(err))
	}
	text := strings.TrimSpace(generic.Text.Content)
	if text == "" {
		return port.InboundEvent{}, false
	}
	peerID := port.FirstNonEmpty(generic.SenderStaffId, generic.ConversationId)
	return port.InboundEvent{
		PeerID:         peerID,
		Text:           text,
		IdempotencyKey: "dingtalk:" + generic.ConversationId,
		OutboundMeta: map[string]string{
			port.MetaSessionWebhook: generic.SessionWebhook,
			port.MetaRecipient:      generic.SessionWebhook,
		},
	}, true
}
