package service

import (
	"context"
	"net/http"
	"sync"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	trpcopenai "trpc.group/trpc-go/trpc-agent-go/server/openai"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// OpenAISessionCompatService wraps the framework's OpenAI-compatible API
// server with session persistence, using lazy initialization. The
// trpc-agent-go Runner is per-session, so the underlying framework server
// is created on the first request via OpenAIRunnerBuilder.
//
// This mirrors OpenAICompatService: the service depends on the narrow
// OpenAIRunnerBuilder interface rather than *ChatService directly, and
// constructs the framework server inline (not via internal/server adapters)
// to avoid an import cycle (internal/server already imports internal/service
// for the ServiceRegistry).
type OpenAISessionCompatService struct {
	chat       OpenAIRunnerBuilder
	sessionSvc trpcsession.Service
	cfg        *conf.Server
	lg         loggateway.Logger

	mu     sync.RWMutex
	server *trpcopenai.Server
	closer func()
}

// NewOpenAISessionCompatService creates a new OpenAISessionCompatService.
// If c is nil the service is disabled (Enabled() returns false).
func NewOpenAISessionCompatService(chat OpenAIRunnerBuilder, sessionSvc trpcsession.Service, c *conf.Server, lg loggateway.Logger) *OpenAISessionCompatService {
	return &OpenAISessionCompatService{chat: chat, sessionSvc: sessionSvc, cfg: c, lg: lg}
}

// Enabled returns true when the service has configuration and can serve requests.
func (s *OpenAISessionCompatService) Enabled() bool {
	return s != nil && s.cfg != nil
}

// Path returns the chat completions endpoint path used for route registration.
func (s *OpenAISessionCompatService) Path() string {
	return "/v1/chat/completions"
}

// Handler returns the http.Handler for OpenAI session-compatible requests,
// lazily initializing the underlying framework server on the first call.
// Double-check locking ensures the server is created only once even under
// concurrent access.
func (s *OpenAISessionCompatService) Handler(ctx context.Context) (http.Handler, error) {
	if err := s.ensureServer(ctx); err != nil {
		return nil, err
	}
	return s.server.Handler(), nil
}

// Close releases resources owned by the service. It is safe to call
// multiple times.
func (s *OpenAISessionCompatService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
	if s.server != nil {
		_ = s.server.Close()
		s.server = nil
	}
	return nil
}

// ensureServer lazily creates the trpcopenai.Server on first use. The runner
// is obtained from OpenAIRunnerBuilder (which delegates to *ChatService)
// using the default agent key, matching the OpenAICompatService pattern.
// Unlike OpenAICompatService, this service wires the session service for
// conversation persistence across turns.
func (s *OpenAISessionCompatService) ensureServer(ctx context.Context) error {
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

	srv, err := trpcopenai.New(
		trpcopenai.WithRunner(runner),
		trpcopenai.WithSessionService(s.sessionSvc),
		trpcopenai.WithBasePath("/v1"),
		trpcopenai.WithPath("/chat/completions"),
		trpcopenai.WithModelName("gpt-3.5-turbo"),
		trpcopenai.WithAppName("openai-session"),
	)
	if err != nil {
		closeRunner()
		s.lg.Error("OpenAI Session server 创建失败",
			loggateway.StepID("openai_session.create_fail"),
			loggateway.Err(err))
		return err
	}
	s.lg.Info("OpenAI Session server 创建成功",
		loggateway.StepID("openai_session.create"),
		loggateway.Str("path", srv.Path()),
		loggateway.Str("base_path", srv.BasePath()))

	s.server = srv
	s.closer = closeRunner
	return nil
}
