package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestClassifyChannelTurnError_taxonomy(t *testing.T) {
	if classifyChannelTurnError(turnBusyError()) != biz.ChannelTurnErrBusy {
		t.Fatal("busy")
	}
	if classifyChannelTurnError(errors.New("429 Too Many Requests")) != biz.ChannelTurnErrRateLimit {
		t.Fatal("rate limit")
	}
	if classifyChannelTurnError(errors.New("maximum context length exceeded")) != biz.ChannelTurnErrContextOverflow {
		t.Fatal("overflow")
	}
	if formatChannelTurnErrorMessage(errors.New("429 Too Many Requests")) != biz.ChannelTurnErrorRateLimitMsg {
		t.Fatal("rate limit message")
	}
	if formatChannelTurnErrorMessage(errors.New("maximum context length exceeded")) != biz.ChannelTurnErrorContextOverflowMsg {
		t.Fatal("overflow message")
	}
}

func TestTurnPreviewCoordinator_runIDFilter(t *testing.T) {
	lt := biz.ParseChannelLongTaskConfig(`{"config":{"im_render_mode":"transcript"}}`)
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Platform: "feishu",
		Policy:   biz.ParseChannelIMRenderPolicy(`{"config":{"im_render_mode":"transcript"}}`, lt),
		LtCfg:    lt,
		Lg:       loggateway.NewNoop(),
	})
	coord.SetActiveRunID("run-1")
	coord.consume(context.Background(), event.Envelope{
		Type:      event.EnvelopeTypeTextDelta,
		SessionID: "sess-1",
		Metadata:  map[string]any{"run_id": "run-2"},
		Content:   &event.EnvelopeContent{Text: "skip"},
	})
	if coord.RenderedText() != "" {
		t.Fatal("should ignore mismatched run_id")
	}
	coord.consume(context.Background(), event.Envelope{
		Type:      event.EnvelopeTypeTextDelta,
		SessionID: "sess-1",
		Metadata:  map[string]any{"run_id": "run-1"},
		Content:   &event.EnvelopeContent{Text: "keep"},
	})
	if !strings.Contains(coord.RenderedText(), "keep") {
		t.Fatalf("should consume matching run_id, got %q", coord.RenderedText())
	}
}
