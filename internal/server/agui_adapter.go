package server

// TECH-DEBT(P2-29): AGUIHandler 框架适配器已实现但未接入生产路径。
// 生产环境使用 internal/service/agui_compat.go 的 AGUICompatService 包装层
// （解决 Runner per-session 问题），而非此直接对接框架的适配器。保留此文件
// 作为未来框架 AG-UI Server 支持 per-session Runner 时的切换入口
// （alignment-plan.md §四 协同包 F）。

import (
	"net/http"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcagui "trpc.group/trpc-go/trpc-agent-go/server/agui"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// AGUIHandler integrates the framework's AG-UI protocol server with the
// project's Kratos-based HTTP transport. It wraps trpc-agent-go/server/agui
// and exposes a RegisterRoutes method for mounting on the Kratos HTTP mux.
//
// AG-UI provides SSE-based streaming for CopilotKit-compatible frontends.
//
// TECH-DEBT(P2-29): 未接入生产路径，见文件头说明。
type AGUIHandler struct {
	server *trpcagui.Server
	lg     loggateway.Logger
}

// AGUIConfig holds the configuration for creating an AGUIHandler.
type AGUIConfig struct {
	// BasePath is the URL prefix for AG-UI routes (default "/agui").
	BasePath string
	// Path is the chat message endpoint path (default "/").
	Path string
	// CancelEnabled enables the cancel endpoint.
	CancelEnabled bool
	// CancelPath is the cancel endpoint path (default "/cancel").
	CancelPath string
	// MessagesSnapshotEnabled enables the messages snapshot endpoint.
	MessagesSnapshotEnabled bool
	// MessagesSnapshotPath is the messages snapshot endpoint path (default "/history").
	MessagesSnapshotPath string
	// HeartbeatInterval controls how often SSE heartbeat frames are sent.
	HeartbeatInterval time.Duration
	// Timeout is the maximum execution time for a run (default 1h).
	Timeout time.Duration
	// FlushInterval controls how often buffered AG-UI events are flushed.
	FlushInterval time.Duration
	// AppName is the static app name for the AG-UI runner.
	AppName string
	// GraphNodeLifecycleActivityEnabled emits graph node lifecycle events.
	GraphNodeLifecycleActivityEnabled bool
	// ReasoningContentEnabled emits reasoning content events.
	ReasoningContentEnabled bool
	// EventSourceMetadataEnabled includes source metadata in AG-UI events.
	EventSourceMetadataEnabled bool
	// ToolCallDeltaStreamingEnabled emits partial tool-call arguments.
	ToolCallDeltaStreamingEnabled bool
}

// NewAGUIHandler creates an AGUIHandler that wraps the framework AG-UI server.
func NewAGUIHandler(r trpcrunner.Runner, sessionSvc trpcsession.Service, cfg AGUIConfig, lg loggateway.Logger) (*AGUIHandler, error) {
	opts := []trpcagui.Option{
		trpcagui.WithSessionService(sessionSvc),
	}
	if cfg.BasePath != "" {
		opts = append(opts, trpcagui.WithBasePath(cfg.BasePath))
	}
	if cfg.Path != "" {
		opts = append(opts, trpcagui.WithPath(cfg.Path))
	}
	if cfg.CancelEnabled {
		opts = append(opts, trpcagui.WithCancelEnabled(true))
		if cfg.CancelPath != "" {
			opts = append(opts, trpcagui.WithCancelPath(cfg.CancelPath))
		}
	}
	if cfg.MessagesSnapshotEnabled {
		opts = append(opts, trpcagui.WithMessagesSnapshotEnabled(true))
		if cfg.MessagesSnapshotPath != "" {
			opts = append(opts, trpcagui.WithMessagesSnapshotPath(cfg.MessagesSnapshotPath))
		}
	}
	if cfg.HeartbeatInterval > 0 {
		opts = append(opts, trpcagui.WithHeartbeatInterval(cfg.HeartbeatInterval))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, trpcagui.WithTimeout(cfg.Timeout))
	}
	if cfg.FlushInterval > 0 {
		opts = append(opts, trpcagui.WithFlushInterval(cfg.FlushInterval))
	}
	if cfg.AppName != "" {
		opts = append(opts, trpcagui.WithAppName(cfg.AppName))
	}
	if cfg.GraphNodeLifecycleActivityEnabled {
		opts = append(opts, trpcagui.WithGraphNodeLifecycleActivityEnabled(true))
	}
	if cfg.ReasoningContentEnabled {
		opts = append(opts, trpcagui.WithReasoningContentEnabled(true))
	}
	if cfg.EventSourceMetadataEnabled {
		opts = append(opts, trpcagui.WithEventSourceMetadataEnabled(true))
	}
	if cfg.ToolCallDeltaStreamingEnabled {
		opts = append(opts, trpcagui.WithToolCallDeltaStreamingEnabled(true))
	}

	srv, err := trpcagui.New(r, opts...)
	if err != nil {
		lg.Error("AG-UI server 创建失败", loggateway.StepID("agui.create_fail"), loggateway.Err(err))
		return nil, err
	}
	lg.Info("AG-UI server 创建成功", loggateway.StepID("agui.create"), loggateway.Str("path", srv.Path()))
	return &AGUIHandler{server: srv, lg: lg}, nil
}

// RegisterRoutes mounts the AG-UI handler onto the given HTTP mux.
// The framework AG-UI server provides its own http.Handler with chat,
// cancel, and history endpoints already wired.
func (h *AGUIHandler) RegisterRoutes(mux *http.ServeMux) {
	if h == nil || h.server == nil {
		return
	}
	mux.Handle(h.server.Path(), h.server.Handler())
	if h.server.BasePath() != "/" && h.server.BasePath() != "" {
		mux.Handle(h.server.BasePath(), h.server.Handler())
	}
}

// Handler returns the underlying http.Handler for custom routing.
func (h *AGUIHandler) Handler() http.Handler {
	if h == nil || h.server == nil {
		return nil
	}
	return h.server.Handler()
}

// Path returns the chat endpoint path.
func (h *AGUIHandler) Path() string {
	if h == nil || h.server == nil {
		return ""
	}
	return h.server.Path()
}
