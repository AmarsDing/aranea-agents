package service

import (
	"context"
	"net/http"
	"sync"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	trpcagui "trpc.group/trpc-go/trpc-agent-go/server/agui"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// AGUICompatService wraps the framework's AG-UI protocol server with lazy
// initialization. The trpc-agent-go Runner is per-session, so the underlying
// AGUIHandler is created on the first request via OpenAIRunnerBuilder.
//
// This mirrors OpenAICompatService: the service depends on the narrow
// OpenAIRunnerBuilder interface rather than *ChatService directly, and
// constructs the framework server inline (not via internal/server adapters)
// to avoid an import cycle (internal/server already imports internal/service
// for the ServiceRegistry).
type AGUICompatService struct {
	chat       OpenAIRunnerBuilder
	sessionSvc trpcsession.Service
	cfg        *conf.Server
	lg         loggateway.Logger

	mu     sync.RWMutex
	server *trpcagui.Server
	closer func()
}

// NewAGUICompatService creates a new AGUICompatService.
// If c is nil the service is disabled (Enabled() returns false).
func NewAGUICompatService(chat OpenAIRunnerBuilder, sessionSvc trpcsession.Service, c *conf.Server, lg loggateway.Logger) *AGUICompatService {
	return &AGUICompatService{chat: chat, sessionSvc: sessionSvc, cfg: c, lg: lg}
}

// Enabled returns true when the service has configuration and can serve requests.
func (s *AGUICompatService) Enabled() bool {
	return s != nil && s.cfg != nil
}

// Path returns the AG-UI base path used for route registration.
func (s *AGUICompatService) Path() string {
	return "/agui"
}

// Handler returns the http.Handler for AG-UI requests, lazily initializing
// the underlying framework server on the first call. Double-check locking
// ensures the server is created only once even under concurrent access.
func (s *AGUICompatService) Handler(ctx context.Context) (http.Handler, error) {
	if err := s.ensureServer(ctx); err != nil {
		return nil, err
	}
	return s.server.Handler(), nil
}

// Close releases resources owned by the service. It is safe to call
// multiple times.
func (s *AGUICompatService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
	s.server = nil
	return nil
}

// ensureServer lazily creates the trpcagui.Server on first use. The runner
// is obtained from OpenAIRunnerBuilder (which delegates to *ChatService)
// using the default agent key, matching the OpenAICompatService pattern.
func (s *AGUICompatService) ensureServer(ctx context.Context) error {
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

	opts := []trpcagui.Option{
		trpcagui.WithSessionService(s.sessionSvc),
		trpcagui.WithBasePath("/agui"),
		trpcagui.WithPath("/"),
		trpcagui.WithAppName("aranea-agents"),
	}

	srv, err := trpcagui.New(runner, opts...)
	if err != nil {
		closeRunner()
		s.lg.Error("AG-UI server 创建失败",
			loggateway.StepID("agui.create_fail"),
			loggateway.Err(err))
		return err
	}
	s.lg.Info("AG-UI server 创建成功",
		loggateway.StepID("agui.create"),
		loggateway.Str("path", srv.Path()))

	s.server = srv
	s.closer = closeRunner
	return nil
}
