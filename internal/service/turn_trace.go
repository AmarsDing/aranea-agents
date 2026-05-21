package service

import (
	"context"

	"aranea-agents/internal/telemetry/turntrace"

	"go.opentelemetry.io/otel/trace"
)

// startTurnSpan opens a chat.turn OTel root span (delegates to turntrace).
func startTurnSpan(ctx context.Context, name, sessionID, agentKey, runID string) (context.Context, *turntrace.Bridge, trace.Span) {
	ctx, bridge, span := turntrace.Start(ctx, turntrace.Config{
		Domain:    turntrace.DomainChat,
		SpanName:  name,
		SessionID: sessionID,
		RunID:     runID,
		AgentKey:  agentKey,
	})
	ctx = turntrace.WithBridge(ctx, bridge)
	return ctx, bridge, span
}

func endTurnSpan(bridge *turntrace.Bridge, err error) {
	if bridge != nil {
		bridge.Finish(err)
	}
}
