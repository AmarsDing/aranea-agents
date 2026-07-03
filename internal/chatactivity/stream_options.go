package chatactivity

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
)

// NewStreamConsumeOptions wires the v2 projector for a chat turn.
//
// v2 phase: the v2 projector is the sole projection path. The v1
// ActivityProjector and the catalog ActivityMetaResolver (which was set but
// never read) have been removed. When v2Projector is nil (test scenarios),
// events are not projected. The v2 projector is a singleton shared across
// turns (per-turn state is reset via Reset + Configure before the LLM call).
//
// Phase 3b-D Tier 4: the v1 ActivityBus parameter has been removed —
// opts.ActivityBus was set but never read by the framework or any agent
// code (all chat events now flow through the v2 EventBus + WSV2Subscriber).
func NewStreamConsumeOptions(v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	opts := &chatagent.StreamConsumeOptions{}
	opts.V2Projector = v2Projector
	return opts
}

// StreamOptsFactoryAdapter implements team.StreamOptsFactory by closing over
// the v2 projector needed to construct StreamConsumeOptions.
// Inject this into the team Runner via SetStreamOptsFactory to eliminate
// the team→chatactivity direct import.
type StreamOptsFactoryAdapter struct {
	// V2Projector is the singleton v2 projector. When non-nil, every chat
	// turn triggers the v2 projection path. Wired via Wire DI.
	V2Projector *v2.ActivityProjector
}

func (a *StreamOptsFactoryAdapter) NewStreamConsumeOptions() *chatagent.StreamConsumeOptions {
	return NewStreamConsumeOptions(a.V2Projector)
}
