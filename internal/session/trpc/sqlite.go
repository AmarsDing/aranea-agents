package session

import (
	"database/sql"

	"aranea-agents/pkg/apierror"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	trpcsqlite "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

func NewSQLiteSessionService(db *sql.DB) (trpcsession.Service, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainSession, "session sqlite: db is required")
	}
	svc, err := trpcsqlite.NewService(db,
		trpcsqlite.WithTablePrefix("trpc_"),
		trpcsqlite.WithEnableAsyncPersist(false),
		trpcsqlite.WithSoftDelete(true),
	)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSession, "session sqlite init").WithCause(err)
	}
	return svc, nil
}
