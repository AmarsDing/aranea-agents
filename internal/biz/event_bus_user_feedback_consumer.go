package biz

import (
	"context"
	"strings"
)

// userFeedbackConsumer records message feedback to monitor and enqueues
// preference memory extraction.
//
// Phase 5 Blocker B: migrated from legacy Envelope-based SessionBus to
// ActivityEventBus. The user_feedback publisher (service.SubmitMessageFeedback)
// emits ActivityEvent{Kind:ActivityKindNotice, Meta:{"notice_type":"user_feedback",
// "message_id","rating","comment"}}. This consumer filters at the bus level
// by Kind==notice and Meta["notice_type"]=="user_feedback".
type userFeedbackConsumer struct {
	bus       ActivityEventBus
	monitor   *MonitorUsecase
	memWorker *TurnMemoryWorker
	logger    SessionLogWriter
}

func newUserFeedbackConsumer(bus ActivityEventBus, monitor *MonitorUsecase, memWorker *TurnMemoryWorker, logger SessionLogWriter) *userFeedbackConsumer {
	if bus == nil {
		return nil
	}
	return &userFeedbackConsumer{bus: bus, monitor: monitor, memWorker: memWorker, logger: logger}
}

func (c *userFeedbackConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runActivityConsumerWithOpts(ctx, "event-bus-user-feedback", c.bus, ActivityEventSubscribeOptions{
		BufferSize: 64,
		GlobalMode: true,
		Filter: func(ev ActivityEvent) bool {
			return ev.Activity.Kind == ActivityKindNotice &&
				metaString(ev.Activity.Meta, "notice_type") == "user_feedback"
		},
	}, c.handle, offerOption[ActivityEvent]{FallbackSync: true, FallbackFn: c.handle}, c.logger)
}

func (c *userFeedbackConsumer) handle(ctx context.Context, ev ActivityEvent) {
	if c == nil {
		return
	}
	sessionID := strings.TrimSpace(ev.Activity.SessionID)
	messageID := metaString(ev.Activity.Meta, "message_id")
	rating := metaString(ev.Activity.Meta, "rating")
	comment := metaString(ev.Activity.Meta, "comment")
	if sessionID == "" || messageID == "" || rating == "" {
		return
	}
	if err := RecordUserFeedbackMonitor(ctx, c.monitor, sessionID, messageID, rating, comment); err != nil && c.logger != nil {
		c.logger.LogSessionWarn(ctx, sessionID, "event_bus.feedback.monitor", "反馈监控事件写入失败",
			LogPair{Key: "message_id", Value: messageID}, LogPair{Key: "error", Value: err})
	}
	if c.memWorker != nil {
		c.memWorker.OnUserFeedback(ctx, sessionID, messageID, rating, comment)
	}
}
