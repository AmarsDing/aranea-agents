package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/internal/event"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	dingUtils "github.com/open-dingtalk/dingtalk-stream-sdk-go/utils"
)

func init() {
	runtime.RegisterStarter("dingtalk", "stream", RunStream)
}

// RunStream uses DingTalk Stream SDK (MuseBot StartDingRobot).
func RunStream(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
) error {
	clientID, clientSecret, err := dingStreamCreds(ctx, ch, creds, lookup)
	if err != nil {
		return err
	}
	chRow := ch
	onChat := func(ctx context.Context, message *chatbot.BotCallbackDataModel) ([]byte, error) {
		ev, ok := parseStreamMessage(message)
		if !ok {
			return nil, nil
		}
		ev.PlatformType = "dingtalk"
		if err := handler.ProcessInbound(ctx, chRow, ev); err != nil {
			event.SysLogWarn("channel.dingtalk.inbound_failed", "钉钉入站处理失败",
				event.P("error", err.Error()),
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

func dingStreamCreds(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup) (string, string, error) {
	var cfg struct {
		Config struct {
			ClientID string `json:"client_id"`
		} `json:"config"`
	}
	_ = json.Unmarshal([]byte(ch.ConfigJSON), &cfg)
	clientID := strings.TrimSpace(cfg.Config.ClientID)
	if clientID == "" {
		s, _ := lookup(ctx, creds, "client_id")
		clientID = strings.TrimSpace(s)
	}
	secret, err := lookup(ctx, creds, "client_secret")
	secret = strings.TrimSpace(secret)
	if clientID == "" || secret == "" {
		return "", "", fmt.Errorf("dingtalk stream: client_id and client_secret required")
	}
	return clientID, secret, err
}

func parseStreamMessage(message *chatbot.BotCallbackDataModel) (port.InboundEvent, bool) {
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
	_ = json.Unmarshal(raw, &generic)
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
			"session_webhook": generic.SessionWebhook,
			"recipient":       generic.SessionWebhook,
		},
	}, true
}
