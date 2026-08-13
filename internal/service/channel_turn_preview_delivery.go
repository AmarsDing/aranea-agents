package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

// turnPreviewDelivery optional side effects during IM preview (overflow pages, tool cards).
type turnPreviewDelivery struct {
	EnqueueOverflow func(ctx context.Context, text string, pageIndex int) error
	UpsertToolCard  func(ctx context.Context, toolID, existingMessageID, cardJSON string) (messageID string, err error)
}

func (h *ChannelIngress) buildTurnPreviewDelivery(
	ctx context.Context,
	chRow biz.Channel,
	platform, recipient string,
	ev port.InboundEvent,
	policy biz.ChannelIMRenderPolicy,
	meta map[string]string,
) *turnPreviewDelivery {
	if h == nil {
		return nil
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	var d turnPreviewDelivery

	if policy.SplitOverflow {
		idemBase := ackIdempotencyKey(platform, ev, "preview")
		d.EnqueueOverflow = func(ctx context.Context, text string, pageIndex int) error {
			return h.enqueueOutboundTranscript(ctx, chRow, platform, recipient, text, meta,
				fmt.Sprintf("%s:overflow:%d", idemBase, pageIndex), true)
		}
	}

	if policy.ToolCardMode == biz.ChannelIMToolCardModeFeishuAppend &&
		(platform == "feishu" || platform == "lark") {
		creds, err := h.channels.ListCredentialsRaw(ctx, chRow.ID)
		if err != nil {
			h.lg.Warn("Channel tool card delivery skipped: credentials list failed",
				loggateway.StepID(flowStepChannelToolCard),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Str("platform", platform),
				loggateway.Err(err),
			)
			if d.EnqueueOverflow != nil {
				return &d
			}
			return nil
		}
		region, appID, err := feishuAppAndRegion(chRow.ConfigJSON)
		if err != nil {
			h.lg.Warn("Channel tool card delivery skipped: feishu config",
				loggateway.StepID(flowStepChannelToolCard),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Str("platform", platform),
				loggateway.Err(err),
			)
			if d.EnqueueOverflow != nil {
				return &d
			}
			return nil
		}
		sec, err := resolveCredentialPlain(ctx, h.channels, creds, "app_secret")
		if err != nil {
			h.lg.Warn("Channel tool card delivery skipped: app_secret",
				loggateway.StepID(flowStepChannelToolCard),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Str("platform", platform),
				loggateway.Err(err),
			)
			if d.EnqueueOverflow != nil {
				return &d
			}
			return nil
		}
		sender := &lark.CardSender{
			Region:        region,
			AppID:         appID,
			AppSecret:     sec,
			ReceiveIDType: lark.ReceiveIDTypeFromMeta(meta),
			HTTP:          h.http,
		}
		d.UpsertToolCard = func(ctx context.Context, _, existingMessageID, cardJSON string) (string, error) {
			return sender.UpsertToolCard(ctx, recipient, existingMessageID, cardJSON)
		}
	}

	if d.EnqueueOverflow == nil && d.UpsertToolCard == nil {
		return nil
	}
	return &d
}
