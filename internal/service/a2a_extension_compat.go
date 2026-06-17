package service

import (
	"context"
	"net/http"
	"sync"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
	trpca2a "trpc.group/trpc-go/trpc-agent-go/server/a2a"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// A2AExtensionCompatService wraps the framework's A2A extension server with
// lazy initialization. The trpc-agent-go Runner is per-session, so the
// underlying A2A server is created on the first request via
// OpenAIRunnerBuilder.
//
// This mirrors OpenAICompatService: the service depends on the narrow
// OpenAIRunnerBuilder interface rather than *ChatService directly, and
// constructs the framework server inline (not via internal/server adapters)
// to avoid an import cycle (internal/server already imports internal/service
// for the ServiceRegistry).
type A2AExtensionCompatService struct {
	chat       OpenAIRunnerBuilder
	sessionSvc trpcsession.Service
	cfg        *conf.Server
	lg         loggateway.Logger

	mu     sync.RWMutex
	server *a2aserver.A2AServer
	closer func()
}

// NewA2AExtensionCompatService creates a new A2AExtensionCompatService.
// If c is nil the service is disabled (Enabled() returns false).
func NewA2AExtensionCompatService(chat OpenAIRunnerBuilder, sessionSvc trpcsession.Service, c *conf.Server, lg loggateway.Logger) *A2AExtensionCompatService {
	return &A2AExtensionCompatService{chat: chat, sessionSvc: sessionSvc, cfg: c, lg: lg}
}

// Enabled returns true when the service has configuration and can serve requests.
func (s *A2AExtensionCompatService) Enabled() bool {
	return s != nil && s.cfg != nil
}

// Path returns the A2A extension base path used for route registration.
func (s *A2AExtensionCompatService) Path() string {
	return "/a2a"
}

// Handler returns the http.Handler for A2A extension requests, lazily
// initializing the underlying framework server on the first call.
// Double-check locking ensures the server is created only once even under
// concurrent access.
func (s *A2AExtensionCompatService) Handler(ctx context.Context) (http.Handler, error) {
	if err := s.ensureServer(ctx); err != nil {
		return nil, err
	}
	return s.server.Handler(), nil
}

// Close releases resources owned by the service. It is safe to call
// multiple times.
func (s *A2AExtensionCompatService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
	s.server = nil
	return nil
}

// ensureServer lazily creates the A2A server on first use. The runner is
// obtained from OpenAIRunnerBuilder (which delegates to *ChatService) using
// the default agent key, matching the OpenAICompatService pattern. A minimal
// default AgentCard is built from the server config.
func (s *A2AExtensionCompatService) ensureServer(ctx context.Context) error {
	s.mu.RLock()
	if s.server != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}

	runner, closeRunner, err := s.chat.BuildOpenAIRunner(ctx, "")
	if err != nil {
		return err
	}

	host := "http://localhost:8080"
	if s.cfg != nil {
		if url := s.cfg.GetA2APublicBaseUrl(); url != "" {
			host = url
		}
	}

	agentCard := defaultA2AAgentCard(host)
	opts := []trpca2a.Option{
		trpca2a.WithRunner(runner),
		trpca2a.WithAgentCard(agentCard),
		trpca2a.WithSessionService(s.sessionSvc),
		trpca2a.WithHost(host),
		trpca2a.WithUserIDHeader("X-User-ID"),
	}

	srv, err := trpca2a.New(opts...)
	if err != nil {
		closeRunner()
		s.lg.Error("A2A Extension server 创建失败",
			loggateway.StepID("a2a_extension.create_fail"),
			loggateway.Err(err))
		return err
	}
	s.lg.Info("A2A Extension server 创建成功",
		loggateway.StepID("a2a_extension.create"),
		loggateway.Str("host", host))

	s.server = srv
	s.closer = closeRunner
	return nil
}

// defaultA2AAgentCard builds a minimal AgentCard from the given host URL.
// The card advertises streaming support and text input/output modes.
func defaultA2AAgentCard(host string) a2aserver.AgentCard {
	streaming := true
	pushNotifications := false
	stateTransitionHistory := false
	return a2aserver.AgentCard{
		Name:        "aranea-agents",
		Description: "Aranea Agents A2A Extension endpoint",
		URL:         host + "/a2a",
		Version:     "1.0.0",
		Capabilities: a2aserver.AgentCapabilities{
			Streaming:              &streaming,
			PushNotifications:      &pushNotifications,
			StateTransitionHistory: &stateTransitionHistory,
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
	}
}
