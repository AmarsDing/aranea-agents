package team

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"
)

// runEventHandler consumes a system.notice from the team run pipeline (BL-09).
type runEventHandler interface {
	HandleRunEvent(ctx context.Context, notice *biz.SystemNoticeEvent) (stop bool)
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

// Start subscribes once and fans SystemNotice events to all handlers.
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
				notice, ok := e.(*biz.SystemNoticeEvent)
				if !ok || notice.NoticeType == "orchestration_status" {
					continue
				}
				for _, h := range p.handlers {
					if h.HandleRunEvent(procCtx, notice) {
						cancel()
						return
					}
				}
			}
		}
	})
	return cancel
}

// orchestrationStatusHandler projects system.notice → AgentNodeState and publishes WS status.
type orchestrationStatusHandler struct {
	cfg     OrchestrationProjectorConfig
	store   *biz.OrchestrationStatusStore
	channel string
	mu      sync.Mutex
}

func (h *orchestrationStatusHandler) HandleRunEvent(ctx context.Context, notice *biz.SystemNoticeEvent) bool {
	if h == nil || h.store == nil {
		return false
	}
	h.mu.Lock()
	changed := h.store.ApplySystemNotice(notice, h.cfg.Registry)
	h.mu.Unlock()
	for _, st := range changed {
		publishOrchestrationStatus(ctx, h.cfg, h.channel, st)
	}
	return false
}
