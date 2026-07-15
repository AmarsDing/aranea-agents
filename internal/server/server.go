package server

import (
	"github.com/google/wire"
)

// ProviderSet is server providers.
// NewWSServerFromInfra is provided via cmd/admin provideWSServer so the durable
// event outbox can be attached for B-06 last_event_id replay.
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewServiceRegistry, NewSessionAuthorizer)
