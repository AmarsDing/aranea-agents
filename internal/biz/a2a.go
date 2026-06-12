package biz

import (
	"context"
	"io"
	"net/http"

	"aranea-agents/internal/biz/a2a"
)

type A2ARunnerFactory interface {
	BuildA2ARunner(ctx context.Context, agentID, publicURL string) (io.Closer, http.Handler, error)
}

type (
	A2ACapability               = a2a.Capability
	A2AAgentCard                = a2a.AgentCard
	A2AInvocation               = a2a.Invocation
	A2AAuditEntry               = a2a.AuditEntry
	A2ARemoteAgent              = a2a.RemoteAgent
	A2ARegisterRemoteAgentInput = a2a.RegisterRemoteAgentInput
	A2ARemoteCardDiscoverInput  = a2a.RemoteCardDiscoverInput
	A2AGatewayEntry             = a2a.GatewayEntry
	A2AGatewayDiscoverInput     = a2a.GatewayDiscoverInput
	A2ARepo                     = a2a.Repo
	A2ACardRepo                 = a2a.CardRepo
	A2AInvocationRepo           = a2a.InvocationRepo
	A2AAuditRepo                = a2a.AuditRepo
	A2ARemoteAgentRepo          = a2a.RemoteAgentRepo
	A2AUsecase                  = a2a.Usecase
	A2AAgentLookup              = a2a.AgentLookup
	A2AAgentMeta                = a2a.AgentMeta
	A2ARetryPolicy              = a2a.RetryPolicy
)

// Backward-compatible aliases using original biz type names.
type (
	RemoteCardDiscoverInput  = a2a.RemoteCardDiscoverInput
	RegisterRemoteAgentInput = a2a.RegisterRemoteAgentInput
	GatewayDiscoverInput     = a2a.GatewayDiscoverInput
)

const (
	A2ASourceLocal  = a2a.SourceLocal
	A2ASourceRemote = a2a.SourceRemote
)

var (
	NewA2AUsecase         = a2a.NewUsecase
	A2AAgentIDsFromCards  = a2a.AgentIDsFromCards
	AgentIDsFromCards     = a2a.AgentIDsFromCards
	NewA2AID              = a2a.NewID
	NewAgentLookupAdapter = a2a.NewAgentLookupAdapter
	A2ADefaultRetryPolicy = a2a.DefaultRetryPolicy
)
