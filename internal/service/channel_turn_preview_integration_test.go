package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestTurnPreviewCoordinator_eventBusWithHeartbeat(t *testing.T) {
	bus := event.NewBus()
	updater := &mockPreviewUpdater{}
	lt := biz.ParseChannelLongTaskConfig(`{"config":{"im_render_mode":"transcript","progress_quiet_sec":3600}}`)
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Bus:       bus,
		Updater:   updater,
		Platform:  "feishu",
		Policy:    biz.ParseChannelIMRenderPolicy(`{"config":{"im_render_mode":"transcript","progress_quiet_sec":3600}}`, lt),
		LtCfg:     lt,
		InitialAck: "收到",
	})
	ctx := context.Background()
	stop := coord.Start(ctx, "sess-bus")
	defer stop()

	bus.Publish(ctx, event.Envelope{
		SessionID: "sess-bus",
		Type:      event.EnvelopeTypeTextDelta,
		Content:   &event.EnvelopeContent{Text: "hello", IsPartial: true},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(coord.RenderedText(), "hello") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event bus not consumed: rendered=%q calls=%d", coord.RenderedText(), len(updater.calls))
}
