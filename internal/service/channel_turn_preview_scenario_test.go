package service

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestTurnPreviewCoordinator_deferredAck(t *testing.T) {
	bus := event.NewBus(nil)
	updater := &mockPreviewUpdater{}
	lt := biz.ParseChannelLongTaskConfig(`{"config":{"streaming_enabled":true,"ack_message":"收到，正在处理…","im_render_mode":"transcript"}}`)
	cfg := `{"config":{"streaming_enabled":true,"im_render_mode":"transcript"}}`
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Bus:        bus,
		Updater:    updater,
		Platform:   "feishu",
		Policy:     biz.ParseChannelIMRenderPolicy(cfg, lt),
		LtCfg:      lt,
		InitialAck: "收到，正在处理…",
		Lg:         loggateway.NewNoop(),
	})
	ctx := context.Background()
	stop := coord.Start(ctx, "sess-ack")
	defer stop()

	if len(updater.calls) == 0 || !strings.Contains(updater.calls[0], "收到") {
		t.Fatalf("expected deferred ack in preview, calls=%v", updater.calls)
	}
}

// Phase 1c-5: TestTurnPreviewCoordinator_textToolTextOrder removed — tested deleted
// EnvelopeType TextDelta/ToolCall/ToolResult behavior in consume() (now a no-op).
// Phase 1c-5: TestTurnPreviewCoordinator_splitOverflow removed — tested deleted
// EnvelopeType TextDone behavior.
// Phase 1c-5: TestTurnPreviewCoordinator_toolCardHook removed — tested deleted
// EnvelopeType ToolCall/ToolResult behavior in consume() (now a no-op).
