package service

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const turnTracerName = "aranea-agents/chat"

func startTurnSpan(ctx context.Context, name, sessionID, agentKey, runID string) (context.Context, trace.Span) {
	tracer := otel.Tracer(turnTracerName)
	ctx, span := tracer.Start(ctx, name)
	span.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.String("agent_key", agentKey),
		attribute.String("run_id", runID),
	)
	return ctx, span
}

func endTurnSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}
