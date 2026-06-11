package lark

import (
	"context"
	"net/http"
	"strings"

	"aranea-agents/internal/channel/port"
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
	return s.SendTextWithMeta(ctx, recipientOpenID, text, nil)
}

// SendTextWithMeta posts text with optional thread reply metadata (F-06b).
func (s *FeishuTextSender) SendTextWithMeta(ctx context.Context, recipientOpenID, text string, meta map[string]string) error {
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
		return errAppCredentialsRequired
	}
	tok, _, err := FetchTenantAccessToken(ctx, client, region, appID, secret)
	if err != nil {
		return err
	}
	receiveType := s.effectiveReceiveIDType()
	receiveID := recipientOpenID
	if meta != nil && strings.EqualFold(strings.TrimSpace(meta[port.MetaReplyInThread]), "true") {
		if tid := strings.TrimSpace(meta[port.MetaThreadID]); tid != "" {
			receiveID = tid
			receiveType = "thread_id"
		}
	}
	return SendTextMessage(ctx, client, region, tok, receiveID, receiveType, text)
}

func (s *FeishuTextSender) effectiveReceiveIDType() string {
	return ReceiveIDTypeFromMeta(map[string]string{port.MetaReceiveIDType: s.ReceiveIDType})
}
