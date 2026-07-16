package biz

import (
	"context"
	"time"
)

// PublishSystemNoticeFromActivity publishes a SystemNoticeEvent that preserves
// the v1 ActivityEvent fields needed by legacy consumers (graph watch,
// orchestration status projector, channel ingress) via Meta:
//
//	activity_kind / activity_status / activity_event / agent_key / team_id
//
// NoticeType defaults to Activity.Stage (graph node_start/end, etc.).
func PublishSystemNoticeFromActivity(ctx context.Context, bus EventBus, aev ActivityEvent) {
	if bus == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	meta := aev.Activity.Meta
	if meta == nil {
		meta = map[string]any{}
	} else {
		cp := make(map[string]any, len(meta)+5)
		for k, v := range meta {
			cp[k] = v
		}
		meta = cp
	}
	meta["activity_kind"] = string(aev.Activity.Kind)
	meta["activity_status"] = string(aev.Activity.Status)
	meta["activity_event"] = string(aev.Event)
	if aev.Activity.AgentKey != "" {
		meta["agent_key"] = aev.Activity.AgentKey
	}
	if aev.Activity.TeamID != "" {
		meta["team_id"] = aev.Activity.TeamID
	}
	noticeType := aev.Activity.Stage
	if noticeType == "" {
		if nt := metaString(meta, "notice_type"); nt != "" {
			noticeType = nt
		} else {
			noticeType = string(aev.Activity.Kind)
		}
	}
	sessionID := aev.Activity.SpiritSessionID
	if sessionID == "" {
		sessionID = aev.Activity.SessionID
	}
	bus.Publish(ctx, NewSystemNoticeEvent(sessionID, noticeType, aev.Activity.Content, meta))
}

// ActivityEventFromSystemNotice reconstructs a synthetic ActivityEvent for
// consumers that still filter on Kind/Stage/Meta (orchestration projector,
// team graph watch, channel ingress).
func ActivityEventFromSystemNotice(n *SystemNoticeEvent) ActivityEvent {
	if n == nil {
		return ActivityEvent{}
	}
	meta := n.Meta
	if meta == nil {
		meta = map[string]any{}
	}
	kind := ActivityKindNotice
	if v, ok := meta["activity_kind"].(string); ok && v != "" {
		kind = ActivityKind(v)
	}
	status := ActivityStatusCompleted
	if v, ok := meta["activity_status"].(string); ok && v != "" {
		status = ActivityStatus(v)
	}
	eventType := ActivityEventUpdated
	if v, ok := meta["activity_event"].(string); ok && v != "" {
		eventType = ActivityEventType(v)
	}
	now := n.OccurredAt()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return ActivityEvent{
		Event: eventType,
		Activity: Activity{
			Kind:            kind,
			Status:          status,
			SessionID:       n.SpiritSessionID(),
			SpiritSessionID: n.SpiritSessionID(),
			Timestamp:       now,
			Stage:           n.NoticeType,
			Content:         n.Message,
			Meta:            meta,
			AgentKey:        metaString(meta, "agent_key"),
			TeamID:          metaString(meta, "team_id"),
		},
		Domain: ActivityDomainChat,
	}
}
