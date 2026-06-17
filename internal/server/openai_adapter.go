package server

import (
	"net/http"

	"aranea-agents/pkg/loggateway"

	trpcopenai "trpc.group/trpc-go/trpc-agent-go/server/openai"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// OpenAISessionAdapter integrates the framework's OpenAI-compatible API server
// with the project's session persistence layer. It enables the framework's
// OpenAI complete options (session persistence, streaming, history) while
// mapping project session management to the framework's session.Service.
type OpenAISessionAdapter struct {
	server *trpcopenai.Server
	lg     loggateway.Logger
}

// OpenAISessionConfig holds the configuration for creating an OpenAISessionAdapter.
type OpenAISessionConfig struct {
	// BasePath is the URL prefix for OpenAI routes (default "/v1").
	BasePath string
	// Path is the chat completions endpoint path (default "/chat/completions").
	Path string
	// ModelName is the model name returned in responses (default "gpt-3.5-turbo").
	ModelName string
	// AppName is the app name for the runner (default "openai-server").
	AppName string
}

// NewOpenAISessionAdapter creates an OpenAISessionAdapter that wires the
// framework's OpenAI server with the project's session service for
// conversation persistence across turns.
func NewOpenAISessionAdapter(
	r trpcrunner.Runner,
	sessionSvc trpcsession.Service,
	cfg OpenAISessionConfig,
	lg loggateway.Logger,
) (*OpenAISessionAdapter, error) {
	opts := []trpcopenai.Option{
		trpcopenai.WithRunner(r),
		trpcopenai.WithSessionService(sessionSvc),
	}
	if cfg.BasePath != "" {
		opts = append(opts, trpcopenai.WithBasePath(cfg.BasePath))
	}
	if cfg.Path != "" {
		opts = append(opts, trpcopenai.WithPath(cfg.Path))
	}
	if cfg.ModelName != "" {
		opts = append(opts, trpcopenai.WithModelName(cfg.ModelName))
	}
	if cfg.AppName != "" {
		opts = append(opts, trpcopenai.WithAppName(cfg.AppName))
	}

	srv, err := trpcopenai.New(opts...)
	if err != nil {
		lg.Error("OpenAI 适配器创建失败", loggateway.StepID("openai.create_fail"), loggateway.Err(err))
		return nil, err
	}
	lg.Info("OpenAI 适配器创建成功",
		loggateway.StepID("openai.create"),
		loggateway.Str("path", srv.Path()),
		loggateway.Str("base_path", srv.BasePath()))
	return &OpenAISessionAdapter{server: srv, lg: lg}, nil
}

// RegisterRoutes mounts the OpenAI handler onto the given HTTP mux.
// The framework OpenAI server provides /v1/chat/completions with both
// streaming and non-streaming support, backed by the project's session
// service for conversation persistence.
func (a *OpenAISessionAdapter) RegisterRoutes(mux *http.ServeMux) {
	if a == nil || a.server == nil || mux == nil {
		return
	}
	mux.Handle(a.server.Path(), a.server.Handler())
}

// Handler returns the underlying http.Handler for custom routing.
func (a *OpenAISessionAdapter) Handler() http.Handler {
	if a == nil || a.server == nil {
		return nil
	}
	return a.server.Handler()
}

// Path returns the chat completions endpoint path.
func (a *OpenAISessionAdapter) Path() string {
	if a == nil || a.server == nil {
		return ""
	}
	return a.server.Path()
}

// Close releases resources owned by the adapter. It is safe to call
// multiple times.
func (a *OpenAISessionAdapter) Close() error {
	if a == nil || a.server == nil {
		return nil
	}
	return a.server.Close()
}
