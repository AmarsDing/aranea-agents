package biz

import "aranea-agents/internal/biz/a2a"

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
	A2AUsecase                  = a2a.Usecase
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
	NewA2AUsecase        = a2a.NewUsecase
	A2AAgentIDsFromCards = a2a.AgentIDsFromCards
	AgentIDsFromCards    = a2a.AgentIDsFromCards
)
