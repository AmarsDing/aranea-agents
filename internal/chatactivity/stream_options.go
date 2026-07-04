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
// 2026-07-04 问题 4 修复：v2Projector 由调用方通过 V2ProjectorFactory.NewProjector()
// 创建的 per-turn 实例传入，不再使用全局单例。每个 turn（spirit + 每个 team
// member）持有独立 Projector 实例，避免并发场景下的状态互相清空。
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
// 2026-07-04 问题 4 修复：V2ProjectorFactory（替代原单例 V2Projector）。
// 每次 NewStreamConsumeOptions() 调用都会通过 factory.NewProjector() 创建
// 独立 Projector 实例，确保并发 team turn 之间状态隔离。
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
