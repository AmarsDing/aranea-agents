package team

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"
)

// runEventHandler consumes a projected ActivityEvent from the team run pipeline (BL-09).
type runEventHandler interface {
	HandleRunEvent(ctx context.Context, aev biz.ActivityEvent) (stop bool)
}

// teamRunPipeline fans out one EventBus subscription to multiple handlers.
// Status projector uses this path; Graph step finalize watch remains a separate
// subscription because it owns HITL SLA / finalize lifecycle.
type teamRunPipeline struct {
	handlers []runEventHandler
}

func newTeamRunPipeline(handlers ...runEventHandler) *teamRunPipeline {
	out := make([]runEventHandler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			out = append(out, h)
		}
	}
	return &teamRunPipeline{handlers: out}
}

// Start subscribes once and fans SystemNotice → ActivityEvent to all handlers.
func (p *teamRunPipeline) Start(ctx context.Context, bus biz.EventBus, spiritSessionID, sessionID string) context.CancelFunc {
	if p == nil || bus == nil || len(p.handlers) == 0 {
		return func() {}
	}
	procCtx, cancel := context.WithCancel(ctx)
	opts := biz.EventSubscribeOptions{SpiritSessionID: spiritSessionID}
	if opts.SpiritSessionID == "" {
		opts.SpiritSessionID = sessionID
	}
	ch, unsub := bus.Subscribe(opts)
	safego.Go(procCtx, "team.run.pipeline", func() {
		defer unsub()
		for {
			select {
			case <-procCtx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				aev, ok := activityEventFromBusEvent(e)
				if !ok {
					continue
				}
				for _, h := range p.handlers {
					if h.HandleRunEvent(procCtx, aev) {
						cancel()
						return
					}
				}
			}
		}
	})
	return cancel
}

func activityEventFromBusEvent(e biz.Event) (biz.ActivityEvent, bool) {
	switch ev := e.(type) {
	case *biz.SystemNoticeEvent:
		if ev.NoticeType == "orchestration_status" {
			return biz.ActivityEvent{}, false
		}
		return biz.ActivityEventFromSystemNotice(ev), true
	default:
		return biz.ActivityEvent{}, false
	}
}

// orchestrationStatusHandler projects ActivityEvent → AgentNodeState and publishes WS status.
type orchestrationStatusHandler struct {
	cfg     OrchestrationProjectorConfig
	store   *biz.OrchestrationStatusStore
	channel string
	mu      sync.Mutex
}

func (h *orchestrationStatusHandler) HandleRunEvent(ctx context.Context, aev biz.ActivityEvent) bool {
	if h == nil || h.store == nil {
		return false
	}
	h.mu.Lock()
	changed := h.store.ApplyActivityEvent(aev, h.cfg.Registry)
	h.mu.Unlock()
	for _, st := range changed {
		publishOrchestrationStatus(ctx, h.cfg, h.channel, st)
	}
	return false
}
