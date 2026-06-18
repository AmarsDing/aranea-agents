package session

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcpostgres "trpc.group/trpc-go/trpc-agent-go/session/postgres"
)

// NewPostgresSessionService builds a Postgres-backed trpcsession.Service.
// The Postgres session service creates its own connection pool from the DSN
// (the framework does not accept an existing *sql.DB for Postgres).
func NewPostgresSessionService(dsn string, lg loggateway.Logger, summarizerCfg *SummarizerConfig) (trpcsession.Service, error) {
	if dsn == "" {
		return nil, apierror.BadRequest(apierror.DomainSession, "session postgres: dsn is required")
	}
	opts := []trpcpostgres.ServiceOpt{
		trpcpostgres.WithPostgresClientDSN(dsn),
		trpcpostgres.WithTablePrefix("trpc_"),
		trpcpostgres.WithEnableAsyncPersist(false),
		trpcpostgres.WithSoftDelete(true),
		trpcpostgres.WithSessionEventLimit(sessionEventLimit),
	}
	if summarizerCfg != nil {
		if s := NewDynamicSummarizer(*summarizerCfg); s != nil {
			opts = append(opts, trpcpostgres.WithSummarizer(s))
		}
	}
	if lg != nil {
		opts = append(opts,
			trpcpostgres.WithAppendEventHook(NewAppendEventAuditHook(lg)),
			trpcpostgres.WithGetSessionHook(NewGetSessionAuditHook(lg)),
		)
	}
	svc, err := trpcpostgres.NewService(opts...)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSession, "session postgres init").WithCause(err)
	}
	return svc, nil
}
