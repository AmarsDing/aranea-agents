package session

import (
	"database/sql"
	"fmt"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	trpcsqlite "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

func NewSQLiteSessionService(db *sql.DB) (trpcsession.Service, error) {
	if db == nil {
		return nil, fmt.Errorf("session sqlite: db is required")
	}
	svc, err := trpcsqlite.NewService(db,
		trpcsqlite.WithTablePrefix("trpc_"),
		trpcsqlite.WithEnableAsyncPersist(true),
		trpcsqlite.WithSoftDelete(true),
	)
	if err != nil {
		return nil, fmt.Errorf("session sqlite init: %w", err)
	}
	return svc, nil
}
