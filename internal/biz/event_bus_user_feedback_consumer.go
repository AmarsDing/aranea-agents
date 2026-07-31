package biz

import (
	"context"
	"strings"
)

// userFeedbackConsumer records message feedback to monitor and enqueues
// preference memory extraction.
//
// Publishes/consumes system.notice with NoticeType=user_feedback.
type userFeedbackConsumer struct {
	bus       EventBus
	monitor   *MonitorUsecase
	memWorker *TurnMemoryWorker
	logger    SessionLogWriter
	flowLog   FlowLogWriter
}

// flowLog is the user-visible flow log (流程日志) port for monitor persist
// failures; nil-safe (tests may pass nil).
func newUserFeedbackConsumer(bus EventBus, monitor *MonitorUsecase, memWorker *TurnMemoryWorker, logger SessionLogWriter, flowLog FlowLogWriter) *userFeedbackConsumer {
	if bus == nil {
		return nil
	}
	return &userFeedbackConsumer{bus: bus, monitor: monitor, memWorker: memWorker, logger: logger, flowLog: flowLog}
}

func (c *userFeedbackConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runSystemNoticeConsumerWithOpts(ctx, "event-bus-user-feedback", c.bus,
		func(n *SystemNoticeEvent) bool {
			return n != nil && n.NoticeType == "user_feedback"
		},
		c.handle,
		offerOption[*SystemNoticeEvent]{FallbackSync: true, FallbackFn: c.handle},
		c.logger,
	)
}

func (c *userFeedbackConsumer) handle(ctx context.Context, n *SystemNoticeEvent) {
	if c == nil || n == nil {
		return
	}
	sessionID := strings.TrimSpace(n.SpiritSessionID())
	messageID := metaString(n.Meta, "message_id")
	rating := metaString(n.Meta, "rating")
	comment := metaString(n.Meta, "comment")
	if sessionID == "" || messageID == "" || rating == "" {
		return
	}
	if err := RecordUserFeedbackMonitor(ctx, c.monitor, sessionID, messageID, rating, comment); err != nil {
		if c.logger != nil {
			c.logger.LogSessionWarn(ctx, sessionID, "event_bus.feedback.monitor", "反馈监控事件写入失败",
				LogPair{Key: "message_id", Value: messageID}, LogPair{Key: "error", Value: err})
		}
		if c.flowLog != nil {
			c.flowLog.LogFlowError(ctx, sessionID, "event_bus.monitor.persist", "监控事件持久化失败",
				LogPair{Key: "message_id", Value: messageID}, LogPair{Key: "error", Value: err.Error()})
		}
	}
	if c.memWorker != nil {
		c.memWorker.OnUserFeedback(ctx, sessionID, messageID, rating, comment)
	}
}
