package team

import (
	"context"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// teeGraphStageNotices taps the team run's framework event stream and
// republishes graph node lifecycle events as graph_stage system notices on
// the event bus.
//
// Background (F-C): the team graph path runs a GraphAgent as the trpc runner
// root (BuildTeamGraphRoot), bypassing trpcGraphRuntime.Run — the only place
// an EventBridge converts framework graph events into graph_stage notices.
// Without this tee the coordinator's StartGraphStepWatch (BL-03) never
// receives node_start/node_end, so PublishTeamStepStarted/PersistGraphRunStep
// never fire and per-member team_run_steps are never persisted (only the
// single anchor fallback step lands).
//
// Conversion reuses graphtrpc.EventBridge.ConvertEvent +
// ActivityEventToSystemNotice (same mapping as the standalone graph path).
// High-frequency events (pregel steps, state updates, channel updates,
// custom node events) are filtered to keep bus traffic bounded (限流红线).
// Events are always forwarded downstream unchanged; notices are published
// bus-only (no persist), matching the standalone graph path behavior.
//
// Y3 (HITL pause race): an interrupt-carrying Pregel step — the only
// reachable HITL carrier (checkpoint interrupt events require
// StreamModeCheckpoints) — must NOT be dropped by the filter, and the pause
// mark must happen SYNCHRONOUSLY in this tee goroutine via onInterrupt,
// before the stream ends. The async bus handoff (notice → coordinator watch
// goroutine → MarkTeamGraphInterrupt) races with run finalization: once
// ConsumeWithFirstByteGuard returns, the runner calls
// DeferTeamRunSuccessIfHITL and would read a stale Running status, completing
// a run that actually paused. The inline mark lands first; the watch-path
// mark stays as an idempotent fallback (MarkTeamGraphInterrupt early-returns
// when already waiting_human). onInterrupt may be nil (tests/passthrough).
func teeGraphStageNotices(
	in <-chan *trpcevent.Event,
	bus biz.EventBus,
	sessionID, spiritSessionID, graphID, execID string,
	onInterrupt func(nodeID, lineageID string),
	lg loggateway.Logger,
) <-chan *trpcevent.Event {
	if in == nil || bus == nil || execID == "" {
		return in
	}
	bridge := graphtrpc.NewEventBridge(nil, nil, sessionID, spiritSessionID, graphID, execID, lg)
	out := make(chan *trpcevent.Event, 64)
	safego.Go(appctx.Ctx(), "team.graph.stage_notice_tee", func() {
		defer close(out)
		for ev := range in {
			out <- ev
			if !isGraphStageNoticeSource(ev) && !isPregelInterruptCarrier(ev, lg) {
				continue
			}
			aev := bridge.ConvertEvent(ev)
			if aev == nil {
				continue
			}
			if onInterrupt != nil {
				if key := metaString(aev.Activity.Meta, "interrupt_key"); key != "" {
					onInterrupt(
						metaString(aev.Activity.Meta, "node_id"),
						metaString(aev.Activity.Meta, "lineage_id"),
					)
				}
			}
			bus.Publish(context.Background(), graphtrpc.ActivityEventToSystemNotice(*aev))
		}
	})
	return out
}

// isPregelInterruptCarrier reports whether the event is a Pregel step
// carrying an HITL interrupt (interrupt key + node id). Plain pregel steps
// stay filtered by the caller (high-frequency progress, no domain meaning).
func isPregelInterruptCarrier(ev *trpcevent.Event, lg loggateway.Logger) bool {
	if ev == nil || ev.Response == nil || ev.Response.Object != trpcgraph.ObjectTypeGraphPregelStep {
		return false
	}
	meta := graphtrpc.ExtractPregelMeta(ev, lg)
	return meta.InterruptKey != "" && meta.NodeID != ""
}

// isGraphStageNoticeSource reports whether the framework event carries graph
// node lifecycle worth republishing as a graph_stage notice. Everything else
// (pregel steps, state updates, channel updates, custom node events,
// non-terminal execution events) is high-frequency or non-actionable for the
// step watch / status projector and stays local to the stream.
func isGraphStageNoticeSource(ev *trpcevent.Event) bool {
	if ev == nil || ev.Response == nil {
		return false
	}
	switch ev.Object {
	case trpcgraph.ObjectTypeGraphNodeStart,
		trpcgraph.ObjectTypeGraphNodeComplete,
		trpcgraph.ObjectTypeGraphNodeError,
		trpcgraph.ObjectTypeGraphCheckpointInterrupt,
		trpcgraph.ObjectTypeGraphExecution:
		return true
	}
	return false
}
