package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

type mockPreviewUpdater struct {
	calls []string
	force []bool
	msgID string
}

func (m *mockPreviewUpdater) Update(_ context.Context, _, text string, force bool) error {
	m.calls = append(m.calls, text)
	m.force = append(m.force, force)
	if m.msgID == "" {
		m.msgID = "om_preview_1"
	}
	return nil
}

func (m *mockPreviewUpdater) PreviewMessageID() string { return m.msgID }

func TestTurnPreviewCoordinatorTextThenTool(t *testing.T) {
	updater := &mockPreviewUpdater{}
	lt := biz.ParseChannelLongTaskConfig(`{"config":{"im_render_mode":"transcript"}}`)
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Updater:   updater,
		Recipient: "u1",
		Platform:  "feishu",
		Policy:    biz.ParseChannelIMRenderPolicy(`{"config":{"im_render_mode":"transcript"}}`, lt),
		LtCfg:     lt,
		Lg:        loggateway.NewNoop(),
	})
	ctx := context.Background()
	stop := coord.Start(ctx, "sess-1")
	defer stop()

	coord.consume(ctx, event.Envelope{
		Type:    event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{Text: "hi", IsPartial: true},
	})
	coord.consume(ctx, event.Envelope{
		Type: event.EnvelopeTypeToolCall,
		ToolCall: &event.EnvelopeToolCall{
			ID:           "t1",
			Name:         "read_file",
			Status:       "calling",
			DisplayLabel: "读取文件",
			ActivityKind: "tool",
		},
	})

	if err := coord.Flush(ctx, true); err != nil {
		t.Fatal(err)
	}
	rendered := coord.RenderedText()
	if rendered == "" || len(updater.calls) == 0 {
		t.Fatalf("rendered=%q calls=%d", rendered, len(updater.calls))
	}
	if coord.PreviewMessageID() != "om_preview_1" {
		t.Fatalf("preview id=%q", coord.PreviewMessageID())
	}
}

func TestTurnPreviewCoordinator_heartbeatPreservesTranscript(t *testing.T) {
	updater := &mockPreviewUpdater{}
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Updater:  updater,
		Platform: "feishu",
		Policy: biz.ChannelIMRenderPolicy{
			Mode:              biz.ChannelIMRenderModeTranscript,
			HeartbeatEnabled:  true,
			HeartbeatQuietSec: 1,
			HeartbeatMessage:  "仍在处理 {{elapsed}}",
		},
		LtCfg: biz.ChannelLongTaskConfig{ProgressQuietSec: 1},
		Lg:    loggateway.NewNoop(),
	})
	ctx := context.Background()
	coord.transcript.Apply(event.Envelope{
		Type:    event.EnvelopeTypeTextDone,
		Content: &event.EnvelopeContent{Text: "正文进度"},
	})
	coord.mu.Lock()
	coord.lastEvent = time.Now().Add(-5 * time.Second)
	coord.mu.Unlock()
	coord.maybeHeartbeat(ctx)
	if len(updater.calls) == 0 {
		t.Fatal("expected heartbeat patch")
	}
	last := updater.calls[len(updater.calls)-1]
	if !strings.Contains(last, "正文进度") || !strings.Contains(last, "仍在处理") {
		t.Fatalf("heartbeat overwrote transcript: %q", last)
	}
}
