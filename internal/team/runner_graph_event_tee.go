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
func teeGraphStageNotices(
	in <-chan *trpcevent.Event,
	bus biz.EventBus,
	sessionID, spiritSessionID, graphID, execID string,
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
			if !isGraphStageNoticeSource(ev) {
				continue
			}
			aev := bridge.ConvertEvent(ev)
			if aev == nil {
				continue
			}
			bus.Publish(context.Background(), graphtrpc.ActivityEventToSystemNotice(*aev))
		}
	})
	return out
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
