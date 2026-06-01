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

func TestTurnPreviewRegistry_replacesPreviousSessionPreview(t *testing.T) {
	reg := newTurnPreviewRegistry()
	firstStopped := false
	secondStopped := false
	coord1 := &TurnPreviewCoordinator{}
	coord2 := &TurnPreviewCoordinator{}
	_ = reg.Register("sess-1", coord1, func() { firstStopped = true })
	stop2 := reg.Register("sess-1", coord2, func() { secondStopped = true })
	if !firstStopped {
		t.Fatal("previous preview should stop when replaced")
	}
	stop2()
	if !secondStopped {
		t.Fatal("second stop should run")
	}
}

func TestTurnPreviewRegistry_SetRunID(t *testing.T) {
	reg := newTurnPreviewRegistry()
	coord := &TurnPreviewCoordinator{}
	reg.Register("sess-1", coord, func() {})
	reg.SetRunID("sess-1", "run-a")
	if got := reg.ActiveRunID("sess-1"); got != "run-a" {
		t.Fatalf("runID=%q", got)
	}
	coord.mu.Lock()
	got := coord.activeRunID
	coord.mu.Unlock()
	if got != "run-a" {
		t.Fatalf("coord runID=%q", got)
	}
}

func TestClassifyChannelTurnError_taxonomy(t *testing.T) {
	if classifyChannelTurnError(turnBusyError()) != channelTurnErrBusy {
		t.Fatal("busy")
	}
	if classifyChannelTurnError(errors.New("429 Too Many Requests")) != channelTurnErrRateLimit {
		t.Fatal("rate limit")
	}
	if classifyChannelTurnError(errors.New("maximum context length exceeded")) != channelTurnErrContextOverflow {
		t.Fatal("overflow")
	}
	if formatChannelTurnErrorMessage(errors.New("429 Too Many Requests")) != channelTurnErrorRateLimitMsg {
		t.Fatal("rate limit message")
	}
	if formatChannelTurnErrorMessage(errors.New("maximum context length exceeded")) != channelTurnErrorContextOverflow {
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
