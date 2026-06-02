package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
)

// toolCallConsumer persists tool_result envelopes to tool_invocations (source=event_bus).
type toolCallConsumer struct {
	bus    contract.Bus
	tools  *ToolUsecase
	logger SessionLogWriter
}

func newToolCallConsumer(bus contract.Bus, tools *ToolUsecase, logger SessionLogWriter) *toolCallConsumer {
	if tools == nil {
		return nil
	}
	return &toolCallConsumer{bus: bus, tools: tools, logger: logger}
}

func (c *toolCallConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumerWithOpts(ctx, "event-bus-tool-call", c.bus, contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{contract.EnvelopeTypeToolResult},
		BufferSize: 256,
		Reliable:   true,
	}, c.handle, OfferOption{
		FallbackSync: true,
		FallbackFn:   c.handle,
	}, c.logger)
}

func (c *toolCallConsumer) handle(ctx context.Context, env contract.Envelope) {
	if c == nil || c.tools == nil || env.ToolCall == nil {
		return
	}
	tc := env.ToolCall
	toolKey := strings.TrimSpace(tc.Name)
	if toolKey == "" {
		return
	}
	toolCallID := strings.TrimSpace(tc.ID)
	if toolCallID == "" {
		return
	}
	status := strings.TrimSpace(tc.Status)
	if status == "" {
		if strings.TrimSpace(tc.ResultJSON) != "" {
			status = "success"
		} else {
			status = "calling"
		}
	}
	if status == "calling" {
		return
	}
	if status == "error" || status == "failed" {
		status = "error"
	}
	ended := strings.TrimSpace(tc.FinishedAt)
	if ended == "" {
		ended = env.Timestamp
	}
	started := strings.TrimSpace(tc.StartedAt)
	if started == "" {
		started = ended
	}
	durationMS := int(tc.DurationMS)
	if durationMS <= 0 && started != "" && ended != "" {
		if t0, err0 := time.Parse(time.RFC3339, started); err0 == nil {
			if t1, err1 := time.Parse(time.RFC3339, ended); err1 == nil {
				durationMS = int(t1.Sub(t0).Milliseconds())
			}
		}
	}
	errCode := strings.TrimSpace(tc.ErrorCode)
	errMsg := ""
	if env.Error != nil {
		errMsg = strings.TrimSpace(env.Error.Message)
		if errCode == "" {
			errCode = strings.TrimSpace(env.Error.Type)
		}
	}
	write := ToolInvocationWrite{
		ToolKey:       toolKey,
		AgentKey:      coalesceNonEmpty(tc.AgentKey, env.Author),
		AgentID:       strings.TrimSpace(tc.AgentID),
		SessionID:     strings.TrimSpace(env.SessionID),
		Status:        status,
		DurationMS:    durationMS,
		StartedAt:     started,
		EndedAt:       ended,
		InputPreview:  strings.TrimSpace(tc.ArgumentsJSON),
		OutputPreview: strings.TrimSpace(tc.ResultJSON),
		ErrorCode:     errCode,
		ErrorMessage:  errMsg,
		Source:        ToolInvocationSourceEventBus,
		ToolCallID:    toolCallID,
	}
	if err := c.tools.RecordToolInvocation(ctx, write); err != nil {
		if c.logger != nil {
			c.logger.LogSessionWarn(ctx, env.SessionID, "system.tool.record_fail", "工具调用记录失败（EventBus）",
				LogPair{Key: "tool", Value: toolKey}, LogPair{Key: "error", Value: err})
		}
	}
}

func coalesceNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
