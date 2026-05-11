package agent

import (
	"context"
	"errors"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// TRPCDefaultAppName keeps the tRPC-Agent-Go session namespace aligned with the
// existing runtime app name while the project migrates off the ADK-shaped path.
const TRPCDefaultAppName = "aranea"

// TRPCRunnerDeps contains product-owned services that are injected into a
// tRPC-Agent-Go Runner. Keep this struct thin: platform policy should stay in
// biz/data, while execution semantics remain in pkg/trpc-agent-go.
type TRPCRunnerDeps struct {
	AppName        string
	SessionService trpcsession.Service
}

// NewTRPCRunner constructs the standard Runner for Aranea Agent turns.
//
// Callers may pass a product session service when bridging persisted sessions.
// If omitted, an in-memory service is used, which is useful for tests and
// request-scoped dry runs.
func NewTRPCRunner(root trpcagent.Agent, deps TRPCRunnerDeps, opts ...trpcrunner.Option) (trpcrunner.Runner, error) {
	if root == nil {
		return nil, errors.New("trpc runtime: root agent is nil")
	}
	appName := strings.TrimSpace(deps.AppName)
	if appName == "" {
		appName = TRPCDefaultAppName
	}
	if deps.SessionService != nil {
		opts = append([]trpcrunner.Option{trpcrunner.WithSessionService(deps.SessionService)}, opts...)
	}
	return trpcrunner.NewRunner(appName, root, opts...), nil
}

// RunTRPCUserTurn executes one user turn and returns the framework event stream.
// Service-layer callers are responsible for projecting these events to Kratos
// responses, SSE, usage, and message persistence.
func RunTRPCUserTurn(
	ctx context.Context,
	r trpcrunner.Runner,
	userID string,
	sessionID string,
	content string,
	opts ...trpcagent.RunOption,
) (<-chan *trpcevent.Event, error) {
	if r == nil {
		return nil, errors.New("trpc runtime: runner is nil")
	}
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	if userID == "" {
		return nil, errors.New("trpc runtime: user id is required")
	}
	if sessionID == "" {
		return nil, errors.New("trpc runtime: session id is required")
	}
	return r.Run(ctx, userID, sessionID, trpcmodel.NewUserMessage(content), opts...)
}

// NewInMemoryTRPCSessionService exposes the framework in-memory session service
// for tests and temporary migration adapters.
func NewInMemoryTRPCSessionService() trpcsession.Service {
	return trpcinmemory.NewSessionService()
}

