package biz

import (
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

type RuntimeSet struct {
	TRPC  trpcmemory.Service
	Admin SessionAdminStore
}

func (s RuntimeSet) Available() bool {
	return s.TRPC != nil
}
