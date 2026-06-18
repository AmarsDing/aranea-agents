package session

import (
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// sessionEventLimit caps the maximum number of events per session.
// Framework default is 1000; we raise to 5000 to accommodate long-running
// agent sessions that accumulate many tool call/result pairs.
const sessionEventLimit = 5000

// NewInMemorySessionService returns an in-memory session service used as
// fallback when Postgres is unavailable (mainly for tests).
func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}
