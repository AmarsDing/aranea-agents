package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func inboundAccessContextFromEvent(ev port.InboundEvent) biz.InboundAccessContext {
	meta := ev.OutboundMeta
	if meta == nil {
		meta = map[string]string{}
	}
	userIDs := uniqueNonEmptyStrings(
		ev.PeerID,
		meta[port.MetaSenderOpenID],
		meta[port.MetaSenderUserID],
		meta["user_id"],
	)
	groupID := strings.TrimSpace(meta[port.MetaChatID])
	isGroup := strings.EqualFold(strings.TrimSpace(meta[port.MetaChatType]), "group") ||
		strings.EqualFold(strings.TrimSpace(meta["conversation_type"]), "group")
	if isGroup && groupID == "" {
		groupID = strings.TrimSpace(ev.PeerID)
	}
	mentioned := metaBool(meta[port.MetaMentioned]) || metaBool(meta["bot_mentioned"])
	if isGroup && strings.TrimSpace(meta["mentions"]) != "" {
		mentioned = true
	}
	return biz.InboundAccessContext{
		UserIDs:   userIDs,
		GroupID:   groupID,
		IsGroup:   isGroup,
		Mentioned: mentioned,
	}
}

func (h *ChannelIngress) checkInboundAccess(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) (allowed bool, reason string, err error) {
	policy, err := biz.ParseChannelAccessPolicy(chRow.ConfigJSON)
	if err != nil {
		return false, "", err
	}
	allowed, reason = policy.Allows(inboundAccessContextFromEvent(ev))
	return allowed, reason, nil
}

func (h *ChannelIngress) rejectInboundAccess(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, reason string) error {
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = biz.ChannelTypeFromConfig(chRow.ConfigJSON)
	}
	recipient := strings.TrimSpace(ev.OutboundMeta[port.MetaRecipient])
	if recipient == "" {
		recipient = ev.PeerID
	}
	idempotency := strings.TrimSpace(ev.IdempotencyKey)
	if idempotency == "" {
		idempotency = platform + ":" + ev.PeerID
	}
	h.recordDelivery(ctx, chRow.ID, "access_denied", map[string]any{
		"peer_id": ev.PeerID,
		"reason":  reason,
	}, reason)
	text := channelAccessDeniedDefault
	if strings.TrimSpace(reason) != "" {
		text = channelAccessDeniedWithReason + reason
	}
	return h.enqueueOutboundReply(ctx, chRow, platform, recipient, text, ev.OutboundMeta, idempotency+":deny")
}

func metaBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func uniqueNonEmptyStrings(parts ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
