package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/dingtalk"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/internal/channel/telegram"
	"aranea-agents/internal/channel/wecom"

	"github.com/google/uuid"
)

func (h *ChannelIngress) runChatTurn(
	ctx context.Context,
	chRow biz.Channel,
	titlePrefix string,
	peerKey string,
	peerID string,
	content string,
) (reply string, err error) {
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return "", err
	}
	ownerType, agentID, teamID, err := biz.ResolveChannelTarget(ctx, h.agents, h.teams, routing, peerID)
	if err != nil {
		return "", err
	}

	bind, err := h.peers.GetByChannelAndPeer(ctx, chRow.ID, peerKey)
	var sessionID string
	switch {
	case err == nil && strings.TrimSpace(bind.SessionID) != "":
		sessionID = bind.SessionID
	case err != nil && err != sql.ErrNoRows:
		return "", err
	default:
		title := titlePrefix + ":" + strings.TrimSpace(chRow.Key) + ":" + peerKey
		created, cerr := h.sessions.Create(ctx, biz.Session{
			OwnerType: ownerType,
			AgentID:   agentID,
			TeamID:    teamID,
			Title:     title,
		})
		if cerr != nil {
			return "", cerr
		}
		sessionID = created.ID
		if _, cerr = h.peers.Create(ctx, biz.ChannelPeerSession{
			ID:        uuid.NewString(),
			ChannelID: chRow.ID,
			PeerKey:   peerKey,
			SessionID: sessionID,
		}); cerr != nil {
			return "", cerr
		}
	}

	req := &chatv1.SendChatMessageRequest{SessionId: sessionID, Content: content}
	if ownerType == "team" && teamID != "" {
		tid := teamID
		req.TeamId = &tid
	}
	_, asst, err := h.chat.RunNativeTurnUnary(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(asst.ContentMarkdown), nil
}

func (h *ChannelIngress) enqueueOutboundReply(
	ctx context.Context,
	chRow biz.Channel,
	platform string,
	recipient string,
	reply string,
	extra map[string]string,
	idempotencyKey string,
) error {
	if strings.TrimSpace(reply) == "" {
		return nil
	}
	_, err := h.channels.EnqueueOutboundDelivery(ctx, chRow.ID, biz.ChannelOutboundPayload{
		Platform:       platform,
		Recipient:      recipient,
		Text:           reply,
		IdempotencyKey: idempotencyKey,
		Extra:          extra,
	})
	return err
}

// ChannelDeliveryWorker drains pending outbound channel deliveries.
type ChannelDeliveryWorker struct {
	channels *biz.ChannelUsecase
	ingress  *ChannelIngress
}

func NewChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *ChannelIngress) *ChannelDeliveryWorker {
	return &ChannelDeliveryWorker{channels: channels, ingress: ingress}
}

func (w *ChannelDeliveryWorker) ProcessPending(ctx context.Context, limit int) error {
	if w == nil || w.channels == nil || w.ingress == nil {
		return nil
	}
	items, err := w.channels.ListPendingOutboundDeliveries(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range items {
		var payload biz.ChannelOutboundPayload
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			_ = w.channels.MarkOutboundAttempt(ctx, row, err)
			continue
		}
		if payload.Kind != "" && payload.Kind != "outbound_text" {
			_ = w.channels.MarkOutboundAttempt(ctx, row, fmt.Errorf("unsupported delivery kind %q", payload.Kind))
			continue
		}
		sendErr := w.ingress.sendOutboundPayload(ctx, row.ChannelID, payload)
		_ = w.channels.MarkOutboundAttempt(ctx, row, sendErr)
	}
	return nil
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
	switch strings.ToLower(strings.TrimSpace(payload.Platform)) {
	case "feishu":
		region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
		if err != nil {
			return err
		}
		sec, err := resolveCredentialPlain(ctx, creds, "app_secret")
		if err != nil {
			return err
		}
		return (&lark.FeishuTextSender{
			Region: region, AppID: appID, AppSecret: sec, HTTP: h.http,
		}).SendText(ctx, payload.Recipient, payload.Text)
	case "dingtalk":
		secret, _ := resolveCredentialPlain(ctx, creds, "secret")
		webhookURL, _ := resolveCredentialPlain(ctx, creds, "webhook_url")
		target := payload.Extra["session_webhook"]
		return (&dingtalk.TextSender{WebhookURL: webhookURL, Secret: secret, HTTP: h.http}).SendText(ctx, target, payload.Text)
	case "wecom", "wecom-app":
		webhookURL, _ := resolveCredentialPlain(ctx, creds, "webhook_url")
		target := payload.Extra["response_url"]
		return (&wecom.TextSender{WebhookURL: webhookURL, HTTP: h.http}).SendText(ctx, target, payload.Text)
	case "slack":
		token, err := resolveCredentialPlain(ctx, creds, "bot_token")
		if err != nil {
			return err
		}
		return (&slack.TextSender{BotToken: token, HTTP: h.http}).SendText(ctx, payload.Recipient, payload.Text)
	case "telegram":
		token, err := resolveCredentialPlain(ctx, creds, "bot_token")
		if err != nil {
			return err
		}
		return (&telegram.TextSender{BotToken: token, HTTP: h.http}).SendText(ctx, payload.Recipient, payload.Text)
	default:
		return fmt.Errorf("unsupported outbound platform %q", payload.Platform)
	}
}

func telegramChatRecipient(chatID int64) string {
	return strconv.FormatInt(chatID, 10)
}
