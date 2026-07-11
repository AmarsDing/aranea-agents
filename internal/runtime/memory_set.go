package runtime

import (
	"aranea-agents/internal/biz"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// MemorySet groups the trpc memory.Service with the L0–L4 admin port and recall usecases for one turn.
// Lives in runtime (not biz) because TRPC field is a trpc-agent-go type injected at Wire time.
type MemorySet struct {
	TRPC            trpcmemory.Service
	Admin           biz.SessionAdminStore
	AdminUsecase    *biz.MemoryAdminUsecase
	ActionLogWriter biz.MemoryActionLogWriter
	L2Recall        biz.MemoryL2Recaller
	L3Recall        biz.MemoryL3Recaller
	CompositeRecall biz.MemoryCompositeRecaller
	// ContextCompressor handles L0 context-window compression. nil when the
	// compressor LLM is not configured (MEMORY_COMPRESSOR_PROVIDER/MODEL env
	// vars unset) — the compression hook becomes a no-op in that case.
	ContextCompressor biz.ContextCompressor
}

func (s MemorySet) Available() bool {
	return s.TRPC != nil
}
