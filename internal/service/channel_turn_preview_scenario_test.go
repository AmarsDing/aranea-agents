package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/preview"
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

func TestTurnPreviewCoordinator_textToolTextOrder(t *testing.T) {
	updater := &mockPreviewUpdater{}
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Updater:  updater,
		Platform: "feishu",
		Policy: biz.ChannelIMRenderPolicy{
			Mode:       biz.ChannelIMRenderModeTranscript,
			ToolDetail: biz.ChannelIMToolDetailLabelSummary,
		},
		Lg: loggateway.NewNoop(),
	})
	ctx := context.Background()

	coord.consume(ctx, event.Envelope{
		Type:    event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{Text: "开始", IsPartial: true},
	})
	coord.consume(ctx, event.Envelope{
		Type: event.EnvelopeTypeToolCall,
		ToolCall: &event.EnvelopeToolCall{
			ID: "t1", Name: "grep", DisplayLabel: "搜索", ActivityKind: "mcp", Status: "calling",
		},
	})
	coord.consume(ctx, event.Envelope{
		Type:     event.EnvelopeTypeToolResult,
		ToolCall: &event.EnvelopeToolCall{ID: "t1", Name: "grep", Status: "ok"},
	})
	coord.consume(ctx, event.Envelope{
		Type:    event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{Text: "完成", IsPartial: true},
	})
	_ = coord.Flush(ctx, true)

	rendered := coord.RenderedText()
	idxStart := strings.Index(rendered, "开始")
	idxTool := strings.Index(rendered, "搜索")
	idxDone := strings.Index(rendered, "完成")
	if idxStart < 0 || idxTool < 0 || idxDone < 0 || !(idxStart < idxTool && idxTool < idxDone) {
		t.Fatalf("order wrong: %q", rendered)
	}
}

func TestTurnPreviewCoordinator_splitOverflow(t *testing.T) {
	updater := &mockPreviewUpdater{}
	var overflowPages []string
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Updater:  updater,
		Platform: "feishu",
		Policy: biz.ChannelIMRenderPolicy{
			Mode:            biz.ChannelIMRenderModeTranscript,
			MaxPreviewRunes: 100000,
			SplitOverflow:   true,
		},
		Delivery: &turnPreviewDelivery{
			EnqueueOverflow: func(_ context.Context, text string, pageIndex int) error {
				overflowPages = append(overflowPages, text)
				if pageIndex < 1 {
					t.Fatalf("pageIndex=%d", pageIndex)
				}
				return nil
			},
		},
		Lg: loggateway.NewNoop(),
	})
	coord.transcript.SetSystem("ack")
	longBody := strings.Repeat("段落内容。\n\n", 3000)
	coord.transcript.Apply(event.Envelope{
		Type:    event.EnvelopeTypeTextDone,
		Content: &event.EnvelopeContent{Text: longBody},
	})

	ctx := context.Background()
	if err := coord.Flush(ctx, true); err != nil {
		t.Fatal(err)
	}
	if len(updater.calls) == 0 {
		t.Fatal("expected preview patch")
	}
	if len(overflowPages) == 0 {
		t.Fatal("expected overflow pages enqueued")
	}
	if len([]rune(updater.calls[len(updater.calls)-1])) > preview.PlatformTextLimit("feishu") {
		t.Fatal("first page exceeds platform limit")
	}
}

func TestTurnPreviewCoordinator_toolCardHook(t *testing.T) {
	type cardOp struct {
		existing string
		card     string
	}
	var ops []cardOp
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Platform: "feishu",
		Policy: biz.ChannelIMRenderPolicy{
			Mode:         biz.ChannelIMRenderModeTranscript,
			ToolCardMode: biz.ChannelIMToolCardModeFeishuAppend,
		},
		SessionID: "sess-card",
		CardOpts: preview.ToolCardBuildOpts{
			SessionID: "sess-card",
			WebOrigin: "https://app.test",
		},
		Delivery: &turnPreviewDelivery{
			UpsertToolCard: func(_ context.Context, _, existing, cardJSON string) (string, error) {
				ops = append(ops, cardOp{existing: existing, card: cardJSON})
				if existing == "" {
					return "om_card_1", nil
				}
				return existing, nil
			},
		},
		Lg: loggateway.NewNoop(),
	})
	ctx := context.Background()

	coord.consume(ctx, event.Envelope{
		Type: event.EnvelopeTypeToolCall,
		ToolCall: &event.EnvelopeToolCall{
			ID: "t1", Name: "read", DisplayLabel: "读取", ActivityKind: "mcp", Status: "calling",
		},
	})
	// Wait for the tool-call card to be sent before sending the tool-result,
	// because consume dispatches card delivery asynchronously via safego.Go.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(ops) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	coord.consume(ctx, event.Envelope{
		Type: event.EnvelopeTypeToolResult,
		ToolCall: &event.EnvelopeToolCall{
			ID: "t1", Name: "read", DisplayLabel: "读取", Summary: "ok", Status: "ok",
		},
	})

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(ops) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 card upserts (create+update), got %d", len(ops))
	}
	var hasCreate, hasUpdate bool
	for _, op := range ops {
		if op.existing == "" && strings.Contains(op.card, "进行中") {
			hasCreate = true
		}
		if op.existing != "" && strings.Contains(op.card, "✓") {
			hasUpdate = true
		}
	}
	if !hasCreate || !hasUpdate {
		t.Fatalf("ops=%+v", ops)
	}
}
