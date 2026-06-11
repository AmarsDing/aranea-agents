package qq

import (
	"context"
	"strings"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/token"
)

// TextSender delivers plain text via QQ Bot OpenAPI v2.
type TextSender struct {
	AppID     string
	AppSecret string
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "qq" }

// SendText sends to a group or C2C user. extra may contain group_id.
func (s *TextSender) SendText(ctx context.Context, recipient, text string, extra map[string]string) error {
	recipient = strings.TrimSpace(recipient)
	text = strings.TrimSpace(text)
	if recipient == "" {
		return errRecipientRequired
	}
	if text == "" {
		return nil
	}
	appID := strings.TrimSpace(s.AppID)
	appSecret := strings.TrimSpace(s.AppSecret)
	if appID == "" || appSecret == "" {
		return errAppCredentialsRequired
	}
	ts := token.NewQQBotTokenSource(&token.QQBotCredentials{AppID: appID, AppSecret: appSecret})
	api := botgo.NewOpenAPI(appID, ts).WithTimeout(15 * time.Second)
	msg := dto.MessageToCreate{Content: text, MsgType: dto.TextMsg}
	if groupID := strings.TrimSpace(extra["group_id"]); groupID != "" {
		_, err := api.PostGroupMessage(ctx, groupID, msg)
		return err
	}
	_, err := api.PostC2CMessage(ctx, recipient, msg)
	return err
}
