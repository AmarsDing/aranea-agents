package lark

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// FeishuTextSender performs Feishu/Lark outbound IM text delivery (receive_id_type=open_id).
// It bundles tenant token fetch + send; used by HTTP webhook ingress after agent reply.
//
// Implements aranea-agents/internal/channel.OutboundText (compile-time assertion in ../contract_test.go).
type FeishuTextSender struct {
	Region        string
	AppID         string
	AppSecret     string
	ReceiveIDType string
	HTTP          *http.Client
}

// ID implements channel.Identified.
func (s *FeishuTextSender) ID() string { return "feishu" }

// SendText posts a text DM to recipient (OpenID). If recipient or text is empty after trim, SendText returns nil (no-op).
func (s *FeishuTextSender) SendText(ctx context.Context, recipientOpenID, text string) error {
	recipientOpenID = strings.TrimSpace(recipientOpenID)
	text = strings.TrimSpace(text)
	if recipientOpenID == "" || text == "" {
		return nil
	}
	client := s.HTTP
	if client == nil {
		client = DefaultHTTPClient()
	}
	region := strings.TrimSpace(strings.ToLower(s.Region))
	if region == "" {
		region = RegionFeishu
	}
	appID := strings.TrimSpace(s.AppID)
	secret := strings.TrimSpace(s.AppSecret)
	if appID == "" || secret == "" {
		return fmt.Errorf("feishu outbound: app_id and app_secret required")
	}
	tok, _, err := FetchTenantAccessToken(ctx, client, region, appID, secret)
	if err != nil {
		return err
	}
	return SendTextMessage(ctx, client, region, tok, recipientOpenID, s.effectiveReceiveIDType(), text)
}

func (s *FeishuTextSender) effectiveReceiveIDType() string {
	return ReceiveIDTypeFromMeta(map[string]string{"receive_id_type": s.ReceiveIDType})
}
