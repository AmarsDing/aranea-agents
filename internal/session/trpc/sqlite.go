package session

import (
	"database/sql"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	trpcsqlite "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

// sessionEventLimit caps the maximum number of events per session.
// Framework default is 1000; we raise to 5000 to accommodate long-running
// agent sessions that accumulate many tool call/result pairs.
const sessionEventLimit = 5000

func NewInMemorySessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

func NewSQLiteSessionService(db *sql.DB, lg loggateway.Logger, summarizerCfg *SummarizerConfig) (trpcsession.Service, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainSession, "session sqlite: db is required")
	}
	opts := []trpcsqlite.ServiceOpt{
		trpcsqlite.WithTablePrefix("trpc_"),
		trpcsqlite.WithEnableAsyncPersist(false),
		trpcsqlite.WithSoftDelete(true),
		trpcsqlite.WithSessionEventLimit(sessionEventLimit),
		trpcsqlite.WithAppendEventHook(NewAppendEventAuditHook(lg)),
		trpcsqlite.WithGetSessionHook(NewGetSessionAuditHook(lg)),
	}
	if summarizerCfg != nil {
		if s := NewDynamicSummarizer(*summarizerCfg); s != nil {
			opts = append(opts, trpcsqlite.WithSummarizer(s))
		}
	}
	svc, err := trpcsqlite.NewService(db, opts...)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSession, "session sqlite init").WithCause(err)
	}
	return svc, nil
}
