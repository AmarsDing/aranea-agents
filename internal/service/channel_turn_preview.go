package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/preview"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const channelPreviewPatchInterval = 2 * time.Second

// streamPreviewMessageID exposes platform preview message identifiers when available.
type streamPreviewMessageID interface {
	PreviewMessageID() string
}

// TurnPreviewCoordinator projects EventBus envelopes to a single IM preview message.
type TurnPreviewCoordinator struct {
	bus       event.Bus
	updater   streamPreviewUpdater
	recipient string
	platform  string
	policy    biz.ChannelIMRenderPolicy
	ltCfg     biz.ChannelLongTaskConfig
	lg        loggateway.Logger

	transcript         *preview.Transcript
	mu                 sync.Mutex
	lastPatch          time.Time
	lastRender         string
	lastEvent          time.Time
	started            time.Time
	messageID          string
	initialAck         string
	delivery           *turnPreviewDelivery
	overflowEnqueued   int
	sentToolCardStatus map[string]string
	toolCardMessageIDs map[string]string
	cardSerial         sync.Mutex
	sessionID          string
	activeRunID        string
	cardOpts           preview.ToolCardBuildOpts
}

type turnPreviewParams struct {
	Bus        event.Bus
	Updater    streamPreviewUpdater
	Recipient  string
	Platform   string
	Policy     biz.ChannelIMRenderPolicy
	LtCfg      biz.ChannelLongTaskConfig
	InitialAck string
	Delivery   *turnPreviewDelivery
	SessionID  string
	CardOpts   preview.ToolCardBuildOpts
	Lg         loggateway.Logger
}

func newTurnPreviewCoordinator(p turnPreviewParams) *TurnPreviewCoordinator {
	c := &TurnPreviewCoordinator{
		bus:        p.Bus,
		updater:    p.Updater,
		recipient:  strings.TrimSpace(p.Recipient),
		platform:   strings.TrimSpace(p.Platform),
		policy:     p.Policy,
		ltCfg:      p.LtCfg,
		lg:         p.Lg,
		transcript: preview.NewTranscript(),
		started:    time.Now(),
		initialAck: strings.TrimSpace(p.InitialAck),
		delivery:   p.Delivery,
		sessionID:  strings.TrimSpace(p.SessionID),
		cardOpts:   p.CardOpts,
	}
	if p.Delivery != nil && p.Delivery.UpsertToolCard != nil {
		c.sentToolCardStatus = make(map[string]string)
		c.toolCardMessageIDs = make(map[string]string)
	}
	return c
}

// Start subscribes to session envelopes and optionally sends the initial ACK on the preview message.
func (c *TurnPreviewCoordinator) Start(ctx context.Context, sessionID string) context.CancelFunc {
	if c == nil {
		return func() {}
	}
	sessionID = strings.TrimSpace(sessionID)
	if c.initialAck != "" {
		c.transcript.SetSystem(c.initialAck)
		if c.updater != nil {
			if err := c.patch(ctx, preview.RenderPlainText(c.transcript, c.policy), true); err != nil {
				c.lg.Warn("initial ack patch failed", loggateway.Err(err))
			}
		}
	}
	if c.bus == nil || sessionID == "" {
		return func() {}
	}
	ch, unsub := c.bus.Subscribe(event.SubscribeOptions{
		SessionID:  sessionID,
		BufferSize: 128,
		DropPolicy: event.DropNewest,
	})
	procCtx, cancel := context.WithCancel(ctx)
	safego.Go(procCtx, "channel.turn.preview", func() {
		defer unsub()
		var heartbeatC <-chan time.Time
		var ticker *time.Ticker
		if c.policy.HeartbeatEnabled && c.updater != nil {
			quiet := c.policy.HeartbeatQuietSec
			if quiet <= 0 {
				quiet = 20
			}
			ticker = time.NewTicker(time.Duration(quiet) * time.Second)
			defer ticker.Stop()
			heartbeatC = ticker.C
		}
		for {
			select {
			case <-procCtx.Done():
				return
			case <-heartbeatC:
				c.maybeHeartbeat(procCtx)
			case env, ok := <-ch:
				if !ok {
					return
				}
				c.consume(procCtx, env)
			}
		}
	})
	return cancel
}

func (c *TurnPreviewCoordinator) maybeHeartbeat(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updater == nil {
		return
	}
	if c.transcript.HasInFlightTool() {
		return
	}
	if time.Since(c.lastEvent) < time.Duration(c.policy.HeartbeatQuietSec)*time.Second {
		return
	}
	heartbeat := biz.RenderChannelTemplate(c.policy.HeartbeatMessage, map[string]string{
		"elapsed": fmt.Sprintf("%ds", int(time.Since(c.started).Seconds())),
	})
	if strings.TrimSpace(heartbeat) == "" {
		return
	}
	rendered := preview.RenderPlainText(c.transcript, c.policy)
	text := heartbeat
	if strings.TrimSpace(rendered) != "" {
		text = preview.FormatRenderedTranscriptForIM(c.platform, rendered) + "\n\n" + heartbeat
	}
	if err := c.patchLocked(ctx, text, false); err != nil {
		c.lg.Warn("heartbeat patch failed", loggateway.Err(err))
	}
}

func (c *TurnPreviewCoordinator) SetActiveRunID(runID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.activeRunID = strings.TrimSpace(runID)
	c.mu.Unlock()
}

func (c *TurnPreviewCoordinator) envelopeMatchesRun(env event.Envelope) bool {
	c.mu.Lock()
	want := c.activeRunID
	c.mu.Unlock()
	if want == "" {
		return true
	}
	if md := env.Metadata; md != nil {
		if rid, ok := md["run_id"].(string); ok && strings.TrimSpace(rid) != "" {
			return strings.TrimSpace(rid) == want
		}
	}
	return true
}

func (c *TurnPreviewCoordinator) maybeBindRunID(env event.Envelope) {
	if c == nil || env.Metadata == nil {
		return
	}
	rid, _ := env.Metadata["run_id"].(string)
	rid = strings.TrimSpace(rid)
	if rid == "" {
		return
	}
	c.mu.Lock()
	if c.activeRunID == "" {
		c.activeRunID = rid
	}
	c.mu.Unlock()
}

func (c *TurnPreviewCoordinator) consume(ctx context.Context, env event.Envelope) {
	c.maybeBindRunID(env)
	if !c.envelopeMatchesRun(env) {
		return
	}
	switch env.Type {
	case event.EnvelopeTypeTextDelta, event.EnvelopeTypeTextDone,
		event.EnvelopeTypeToolCall, event.EnvelopeTypeToolResult,
		event.EnvelopeTypeMemberMessageStart, event.EnvelopeTypeMemberDelta,
		event.EnvelopeTypeMemberMessageDone:
		c.mu.Lock()
		c.transcript.Apply(env)
		c.lastEvent = time.Now()
		toolID := ""
		sendCard := false
		var cardSnap preview.Segment
		if env.ToolCall != nil &&
			(env.Type == event.EnvelopeTypeToolCall || env.Type == event.EnvelopeTypeToolResult) &&
			c.policy.ToolCardMode == biz.ChannelIMToolCardModeFeishuAppend {
			toolID = strings.TrimSpace(env.ToolCall.ID)
			if toolID == "" {
				toolID = strings.TrimSpace(env.ToolCall.Name)
			}
			if toolID != "" {
				for _, s := range c.transcript.Segments() {
					if s.Kind == preview.SegmentTool && s.ID == toolID {
						cardSnap = s
						sendCard = true
						break
					}
				}
			}
		}
		rendered := preview.RenderPlainText(c.transcript, c.policy)
		segmentsCount := len(c.transcript.Segments())
		c.mu.Unlock()
		if sendCard {
			tid := toolID
			snap := cardSnap
			safego.Go(ctx, "channel.tool.card", func() {
				c.cardSerial.Lock()
				defer c.cardSerial.Unlock()
				c.maybeSendToolCard(ctx, tid, snap)
			})
		}
		if c.updater != nil {
			patchText := preview.FormatRenderedTranscriptForIM(c.platform, rendered)
			if strings.TrimSpace(patchText) == "" && segmentsCount > 0 {
				patchText = channelPreviewThinkingHint
			}
			if strings.TrimSpace(patchText) != "" {
				if err := c.patch(ctx, patchText, false); err != nil {
					c.lg.Warn("envelope patch failed", loggateway.Err(err))
				}
			}
		}
	}
}

func (c *TurnPreviewCoordinator) patch(ctx context.Context, text string, force bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.patchLocked(ctx, text, force)
}

func (c *TurnPreviewCoordinator) patchLocked(ctx context.Context, text string, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" || c.updater == nil {
		return nil
	}
	limit := preview.PlatformTextLimit(c.platform)
	if c.policy.SplitOverflow && c.delivery != nil && c.delivery.EnqueueOverflow != nil && len([]rune(text)) > limit {
		return c.patchSplitLocked(ctx, text, force)
	}
	text = preview.TruncateRunes(text, limit)
	if !force && time.Since(c.lastPatch) < channelPreviewPatchInterval && text == c.lastRender {
		return nil
	}
	if !force && time.Since(c.lastPatch) < channelPreviewPatchInterval {
		return nil
	}
	if err := c.updater.Update(ctx, c.recipient, text, force); err != nil {
		recordPreviewPatch(c.platform, err)
		return err
	}
	c.lastPatch = time.Now()
	c.lastRender = text
	if id, ok := c.updater.(streamPreviewMessageID); ok {
		if v := strings.TrimSpace(id.PreviewMessageID()); v != "" {
			c.messageID = v
		}
	}
	recordPreviewPatch(c.platform, nil)
	c.lg.Info("Channel preview PATCH",
		loggateway.StepID(flowStepChannelPreview),
		loggateway.Str("platform", c.platform),
		loggateway.Int("text_len", len(text)),
	)
	return nil
}

func (c *TurnPreviewCoordinator) patchSplitLocked(ctx context.Context, text string, force bool) error {
	limit := preview.PlatformTextLimit(c.platform)
	pages := preview.SplitPages(text, limit)
	if len(pages) == 0 {
		return nil
	}
	first := pages[0]
	if !force && time.Since(c.lastPatch) < channelPreviewPatchInterval && first == c.lastRender {
		// still enqueue new overflow pages below
	} else if err := c.updater.Update(ctx, c.recipient, first, force); err != nil {
		recordPreviewPatch(c.platform, err)
		return err
	} else {
		c.lastPatch = time.Now()
		c.lastRender = first
		if id, ok := c.updater.(streamPreviewMessageID); ok {
			if v := strings.TrimSpace(id.PreviewMessageID()); v != "" {
				c.messageID = v
			}
		}
		recordPreviewPatch(c.platform, nil)
	}
	if c.delivery == nil || c.delivery.EnqueueOverflow == nil {
		return nil
	}
	for i := c.overflowEnqueued; i < len(pages)-1; i++ {
		if err := c.delivery.EnqueueOverflow(ctx, pages[i+1], i+1); err != nil {
			return err
		}
		c.overflowEnqueued = i + 1
	}
	return nil
}

// Flush forces the latest transcript to the preview message.
func (c *TurnPreviewCoordinator) Flush(ctx context.Context, force bool) error {
	c.mu.Lock()
	rendered := preview.RenderPlainText(c.transcript, c.policy)
	c.mu.Unlock()
	return c.patchDeliverable(ctx, rendered, force)
}

func (c *TurnPreviewCoordinator) patchDeliverable(ctx context.Context, text string, force bool) error {
	text = strings.TrimSpace(preview.FormatRenderedTranscriptForIM(c.platform, text))
	if text == "" {
		return nil
	}
	if !force || !c.policy.SplitOverflow || c.delivery == nil || c.delivery.EnqueueOverflow == nil {
		return c.patch(ctx, text, force)
	}
	limit := preview.PlatformTextLimit(c.platform)
	if len([]rune(text)) <= limit {
		return c.patch(ctx, text, force)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.patchSplitLocked(ctx, text, true)
}

func (c *TurnPreviewCoordinator) maybeSendToolCard(ctx context.Context, toolID string, seg preview.Segment) {
	if c.delivery == nil || c.delivery.UpsertToolCard == nil || toolID == "" {
		return
	}
	if c.policy.ToolCardMode != biz.ChannelIMToolCardModeFeishuAppend {
		return
	}
	if seg.ID == "" || seg.Kind != preview.SegmentTool {
		return
	}
	status := strings.ToLower(strings.TrimSpace(seg.Status))
	lastSent := ""
	c.mu.Lock()
	if c.sentToolCardStatus != nil {
		lastSent = c.sentToolCardStatus[toolID]
	}
	if lastSent == status && lastSent != "" {
		c.mu.Unlock()
		return
	}
	if c.sentToolCardStatus == nil {
		c.sentToolCardStatus = make(map[string]string)
	}
	c.sentToolCardStatus[toolID] = status
	existingMsgID := ""
	if c.toolCardMessageIDs != nil {
		existingMsgID = c.toolCardMessageIDs[toolID]
	}
	if c.toolCardMessageIDs == nil {
		c.toolCardMessageIDs = make(map[string]string)
	}
	opts := c.cardOpts
	opts.SessionID = c.sessionID
	c.mu.Unlock()

	cardJSON, err := preview.BuildFeishuToolCardJSON(seg, opts)
	if err != nil || strings.TrimSpace(cardJSON) == "" {
		recordToolCard(c.platform, "build", err)
		return
	}
	msgID, err := c.delivery.UpsertToolCard(ctx, toolID, existingMsgID, cardJSON)
	if err != nil {
		recordToolCard(c.platform, "send", err)
		c.lg.Warn("Channel tool card upsert failed",
			loggateway.StepID(flowStepChannelToolCard),
			loggateway.Str("platform", c.platform),
			loggateway.Str("tool_id", toolID),
			loggateway.Str("existing_message_id", existingMsgID),
			loggateway.Err(err),
		)
		return
	}
	if strings.TrimSpace(msgID) != "" {
		c.mu.Lock()
		c.toolCardMessageIDs[toolID] = strings.TrimSpace(msgID)
		c.mu.Unlock()
	}
	phase := "send"
	if existingMsgID != "" {
		phase = "update"
	}
	recordToolCard(c.platform, phase, nil)
}

func (c *TurnPreviewCoordinator) RenderedText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return preview.RenderPlainText(c.transcript, c.policy)
}

func (c *TurnPreviewCoordinator) PreviewMessageID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messageID
}

func (c *TurnPreviewCoordinator) ContentPreview(max int) string {
	text := c.RenderedText()
	if max <= 0 {
		max = 200
	}
	return truncateForLog(text, max)
}

// FlushFinalText patches the preview message with final text (avoids a second outbound when preview exists).
func (c *TurnPreviewCoordinator) FlushFinalText(ctx context.Context, text string) error {
	if c == nil || c.updater == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return c.patch(ctx, text, true)
}

func recordPreviewPatch(platform string, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	arametrics.ChannelProgressPatchTotal.WithLabelValues(strings.ToLower(strings.TrimSpace(platform)), result).Inc()
}

func recordToolCard(platform, phase string, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	arametrics.ChannelToolCardTotal.WithLabelValues(strings.ToLower(strings.TrimSpace(platform)), phase, result).Inc()
}

func (h *ChannelIngress) startTurnPreview(
	ctx context.Context,
	sessionID, platform, recipient string,
	updater streamPreviewUpdater,
	chRow biz.Channel,
	ev port.InboundEvent,
	ltCfg biz.ChannelLongTaskConfig,
) (*TurnPreviewCoordinator, context.CancelFunc) {
	policy := biz.ParseChannelIMRenderPolicy(chRow.ConfigJSON, ltCfg)
	meta := ev.OutboundMeta
	if meta == nil {
		meta = map[string]string{}
	}
	initialAck := ""
	if biz.ChannelACKDeferredToPreview(chRow.ConfigJSON, platform) {
		initialAck = strings.TrimSpace(ltCfg.AckMessage)
	}
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Bus:        h.eventBus,
		Updater:    updater,
		Recipient:  recipient,
		Platform:   platform,
		Policy:     policy,
		LtCfg:      ltCfg,
		InitialAck: initialAck,
		Delivery:   h.buildTurnPreviewDelivery(ctx, chRow, platform, recipient, ev, policy, meta),
		SessionID:  sessionID,
		CardOpts: preview.ToolCardBuildOpts{
			SessionID: sessionID,
			WebOrigin: biz.ResolveChannelWebOrigin(chRow.MetadataJSON),
		},
		Lg: h.lg,
	})
	cancel := coord.Start(ctx, sessionID)
	if h != nil && h.previewManager != nil {
		cancel = h.previewManager.Register(sessionID, cancel)
	}
	return coord, cancel
}

func (h *ChannelIngress) startTurnPreviewAccumulate(
	ctx context.Context,
	sessionID, platform string,
	configJSON string,
	ltCfg biz.ChannelLongTaskConfig,
) (*TurnPreviewCoordinator, context.CancelFunc) {
	policy := biz.ParseChannelIMRenderPolicy(configJSON, ltCfg)
	coord := newTurnPreviewCoordinator(turnPreviewParams{
		Bus:      h.eventBus,
		Platform: platform,
		Policy:   policy,
		LtCfg:    ltCfg,
		Lg:       h.lg,
	})
	cancel := coord.Start(ctx, sessionID)
	if h != nil && h.previewManager != nil {
		cancel = h.previewManager.Register(sessionID, cancel)
	}
	return coord, cancel
}
