package chatactivity

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
)

// NewStreamConsumeOptions wires the v2 projector and optional activity bus for
// a chat turn.
//
// v2 phase: the v2 projector is the sole projection path. The v1
// ActivityProjector and the catalog ActivityMetaResolver (which was set but
// never read) have been removed. When v2Projector is nil (test scenarios),
// events are not projected. The v2 projector is a singleton shared across
// turns (per-turn state is reset via Reset + Configure before the LLM call).
func NewStreamConsumeOptions(activityBus biz.ActivityEventBus, v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	opts := &chatagent.StreamConsumeOptions{}
	if activityBus != nil {
		opts.ActivityBus = activityBus
	}
	opts.V2Projector = v2Projector
	return opts
}

// StreamOptsFactoryAdapter implements team.StreamOptsFactory by closing over
// the activity bus and v2 projector needed to construct StreamConsumeOptions.
// Inject this into the team Runner via SetStreamOptsFactory to eliminate
// the team→chatactivity direct import.
type StreamOptsFactoryAdapter struct {
	ActivityBus biz.ActivityEventBus
	// V2Projector is the singleton v2 projector. When non-nil, every chat
	// turn triggers the v2 projection path. Wired via Wire DI.
	V2Projector *v2.ActivityProjector
}

func (a *StreamOptsFactoryAdapter) NewStreamConsumeOptions() *chatagent.StreamConsumeOptions {
	return NewStreamConsumeOptions(a.ActivityBus, a.V2Projector)
}
