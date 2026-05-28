package session

import (
	"database/sql"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	trpcsqlite "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

func NewSQLiteSessionService(db *sql.DB) (trpcsession.Service, error) {
	if db == nil {
		return nil, kerrors.BadRequest("SESSION", "session sqlite: db is required")
	}
	svc, err := trpcsqlite.NewService(db,
		trpcsqlite.WithTablePrefix("trpc_"),
		trpcsqlite.WithEnableAsyncPersist(false),
		trpcsqlite.WithSoftDelete(true),
	)
	if err != nil {
		return nil, kerrors.InternalServer("SESSION", "session sqlite init: "+err.Error())
	}
	return svc, nil
}
