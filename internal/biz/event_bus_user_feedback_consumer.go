package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/event/contract"
)

// userFeedbackConsumer records message feedback to monitor and enqueues preference memory extraction.
type userFeedbackConsumer struct {
	bus       contract.Bus
	monitor   *MonitorUsecase
	memWorker *TurnMemoryWorker
	logger    SessionLogWriter
}

func newUserFeedbackConsumer(bus contract.Bus, monitor *MonitorUsecase, memWorker *TurnMemoryWorker, logger SessionLogWriter) *userFeedbackConsumer {
	if bus == nil {
		return nil
	}
	return &userFeedbackConsumer{bus: bus, monitor: monitor, memWorker: memWorker, logger: logger}
}

func (c *userFeedbackConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumerWithOpts(ctx, "event-bus-user-feedback", c.bus, contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{contract.EnvelopeTypeUserFeedback},
		BufferSize: 64,
		Reliable:   true,
	}, c.handle, OfferOption{FallbackSync: true, FallbackFn: c.handle}, c.logger)
}

func (c *userFeedbackConsumer) handle(ctx context.Context, env contract.Envelope) {
	if c == nil {
		return
	}
	sessionID := strings.TrimSpace(env.SessionID)
	messageID := metaString(env.Metadata, "message_id")
	rating := metaString(env.Metadata, "rating")
	comment := metaString(env.Metadata, "comment")
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
