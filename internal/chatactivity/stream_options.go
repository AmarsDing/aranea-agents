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
// events are not projected.
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
// the v2 projector factory needed to construct StreamConsumeOptions.
// Inject this into the team Runner via SetStreamOptsFactory to eliminate
// the team→chatactivity direct import.
//
type StreamOptsFactoryAdapter struct {
	// V2ProjectorFactory produces per-turn v2 ActivityProjector instances.
	// When nil, NewStreamConsumeOptions returns nil (v2 path disabled).
	V2ProjectorFactory *v2.ProjectorFactory
}

func (a *StreamOptsFactoryAdapter) NewStreamConsumeOptions() *chatagent.StreamConsumeOptions {
	if a == nil || a.V2ProjectorFactory == nil {
		return nil
	}
	return NewStreamConsumeOptions(a.V2ProjectorFactory.NewProjector())
}
