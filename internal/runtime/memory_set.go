package runtime

import (
	"aranea-agents/internal/biz"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// MemorySet groups the trpc memory.Service with the L0–L4 admin port for one turn.
// Lives in runtime (not biz) because TRPC field is a trpc-agent-go type injected at Wire time.
type MemorySet struct {
	TRPC  trpcmemory.Service
	Admin biz.SessionAdminStore
}

func (s MemorySet) Available() bool {
	return s.TRPC != nil
}
