package session

import (
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}
