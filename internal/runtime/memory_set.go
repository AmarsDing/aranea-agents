package runtime

import (
	"aranea-agents/internal/biz"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// MemorySet groups the trpc memory.Service with L0–L4 narrow ports and recall usecases for one turn.
// Lives in runtime (not biz) because TRPC field is a trpc-agent-go type injected at Wire time.
type MemorySet struct {
	TRPC trpcmemory.Service
	// MemoryLayerPorts holds independent L0–L4 persistence ports (ISP).
	// Replaces the deprecated SessionAdminStore aggregate field.
	biz.MemoryLayerPorts
	AdminUsecase    *biz.MemoryAdminUsecase
	ActionLogWriter biz.MemoryActionLogWriter
	L2Recall        biz.MemoryL2Recaller
	L3Recall        biz.MemoryL3Recaller
	CompositeRecall biz.MemoryCompositeRecaller
	// PreferenceLister feeds the pinned preference/constraint prompt block
	// (FR-M3). Optional: nil disables pinned injection.
	PreferenceLister biz.MemoryPreferenceLister
	// ProfileCardReader feeds the resident profile card prompt block (FR-12.7).
	// Optional: nil disables profile card injection.
	ProfileCardReader biz.MemoryProfileCardReader
	// FactInjectCounter bumps injected_count for the facts actually written
	// into the prompt each turn (FR-12.6). Optional: nil disables counting.
	FactInjectCounter biz.MemoryFactInjectCounter
	// Reconsolidator triggers L4 memory reconsolidation (activation boost +
	// use_count + Hebbian reinforcement) when entities are recalled into the
	// prompt (design §15.7, FR-10.5). Optional: nil disables the trigger.
	Reconsolidator biz.L4Reconsolidator
	// AgentCaseRecaller feeds the task-experience case block (P3 M3): the
	// agent's distilled goal/approach/pitfalls from past sessions, merged
	// into the recall cue alongside L2/L3. Optional: nil skips case recall.
	AgentCaseRecaller biz.AgentCaseRecaller
}

func (s MemorySet) Available() bool {
	return s.TRPC != nil
}
