package server

// TECH-DEBT(P2-30): A2AExtensionAdapter 框架适配器已实现但未接入生产路径。
// 生产环境使用 internal/service/a2a_extension_compat.go 的
// A2AExtensionCompatService 包装层（懒加载 + AgentCard 构建），而非此直接
// 对接框架的适配器。保留此文件作为未来框架 A2A Server 支持懒加载时的切换入口
// （alignment-plan.md §四 协同包 F）。

import (
	"context"
	"net/http"

	"aranea-agents/pkg/loggateway"

	a2ago "trpc.group/trpc-go/trpc-a2a-go/protocol"
	a2aserver "trpc.group/trpc-go/trpc-a2a-go/server"
	"trpc.group/trpc-go/trpc-a2a-go/taskmanager"
	trpca2a "trpc.group/trpc-go/trpc-agent-go/server/a2a"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// A2AExtensionAdapter enables the framework's A2A extension points for
// message audit, filter, and error customization. It wraps the framework's
// A2A server and provides hooks for project-specific processing.
//
// TECH-DEBT(P2-30): 未接入生产路径，见文件头说明。
type A2AExtensionAdapter struct {
	server *a2aserver.A2AServer
	lg     loggateway.Logger
}

// A2AExtensionConfig holds the configuration for creating an A2AExtensionAdapter.
type A2AExtensionConfig struct {
	// Host is the public URL for the agent card (e.g. "http://localhost:8080").
	Host string
	// EnableStreaming controls whether the A2A server supports streaming.
	EnableStreaming bool
	// UserIDHeader is the HTTP header for extracting user IDs (default "X-User-ID").
	UserIDHeader string
	// DebugLogging enables verbose A2A protocol logging.
	DebugLogging bool
	// ADKCompatibility enables ADK-compatible metadata keys.
	ADKCompatibility bool
	// StructuredTaskErrors enables structured error propagation.
	StructuredTaskErrors bool
}

// MessageAuditHook is called before an A2A message is processed.
// Return an error to reject the message, or nil to allow it.
type MessageAuditHook func(ctx context.Context, msg *a2ago.Message) error

// MessageFilterHook is called after conversion but before sending.
// Return nil to drop the message.
type MessageFilterHook func(ctx context.Context, result a2ago.UnaryMessageResult) a2ago.UnaryMessageResult

// NewA2AExtensionAdapter creates an A2AExtensionAdapter that wires the
// framework's A2A server with project-specific extension hooks.
func NewA2AExtensionAdapter(
	r trpcrunner.Runner,
	sessionSvc trpcsession.Service,
	agentCard a2aserver.AgentCard,
	cfg A2AExtensionConfig,
	auditHook MessageAuditHook,
	filterHook MessageFilterHook,
	lg loggateway.Logger,
) (*A2AExtensionAdapter, error) {
	opts := []trpca2a.Option{
		trpca2a.WithRunner(r),
		trpca2a.WithAgentCard(agentCard),
		trpca2a.WithSessionService(sessionSvc),
	}
	if cfg.Host != "" {
		opts = append(opts, trpca2a.WithHost(cfg.Host))
	}
	if cfg.UserIDHeader != "" {
		opts = append(opts, trpca2a.WithUserIDHeader(cfg.UserIDHeader))
	}
	if cfg.DebugLogging {
		opts = append(opts, trpca2a.WithDebugLogging(true))
	}
	if !cfg.ADKCompatibility {
		opts = append(opts, trpca2a.WithADKCompatibility(false))
	}
	if cfg.StructuredTaskErrors {
		opts = append(opts, trpca2a.WithStructuredTaskErrors(true))
	}

	// Wire audit hook as a ProcessMessageHook if provided.
	if auditHook != nil {
		opts = append(opts, trpca2a.WithProcessMessageHook(
			makeAuditProcessMessageHook(auditHook, lg),
		))
	}

	// Wire filter hook as a ResponseRewriter if provided.
	if filterHook != nil {
		opts = append(opts, trpca2a.WithResponseRewriter(
			&messageFilterRewriter{filterHook: filterHook},
		))
	}

	srv, err := trpca2a.New(opts...)
	if err != nil {
		lg.Error("A2A 扩展适配器创建失败", loggateway.StepID("a2a.create_fail"), loggateway.Err(err))
		return nil, err
	}
	lg.Info("A2A 扩展适配器创建成功", loggateway.StepID("a2a.create"))
	return &A2AExtensionAdapter{server: srv, lg: lg}, nil
}

// Handler returns the http.Handler for the A2A server.
func (a *A2AExtensionAdapter) Handler() http.Handler {
	if a == nil || a.server == nil {
		return nil
	}
	return a.server.Handler()
}

// RegisterRoutes mounts the A2A handler onto the given HTTP mux.
func (a *A2AExtensionAdapter) RegisterRoutes(mux *http.ServeMux, path string) {
	if a == nil || a.server == nil || mux == nil {
		return
	}
	mux.Handle(path, a.server.Handler())
}

// Server returns the underlying A2A server for advanced configuration.
func (a *A2AExtensionAdapter) Server() *a2aserver.A2AServer {
	if a == nil {
		return nil
	}
	return a.server
}

// makeAuditProcessMessageHook wraps a MessageAuditHook as a framework
// ProcessMessageHook. The hook inspects incoming messages before they
// reach the agent runner.
func makeAuditProcessMessageHook(auditHook MessageAuditHook, lg loggateway.Logger) trpca2a.ProcessMessageHook {
	return func(next taskmanager.MessageProcessor) taskmanager.MessageProcessor {
		return &auditProcessor{inner: next, auditHook: auditHook, lg: lg}
	}
}

// auditProcessor wraps a message processor with an audit hook.
type auditProcessor struct {
	inner     taskmanager.MessageProcessor
	auditHook MessageAuditHook
	lg        loggateway.Logger
}

// ProcessMessage implements taskmanager.MessageProcessor with audit.
func (p *auditProcessor) ProcessMessage(
	ctx context.Context,
	msg a2ago.Message,
	opts taskmanager.ProcessOptions,
	handler taskmanager.TaskHandler,
) (*taskmanager.MessageProcessingResult, error) {
	if p.auditHook != nil {
		if err := p.auditHook(ctx, &msg); err != nil {
			p.lg.Warn("A2A 消息审计拒绝", loggateway.StepID("a2a.audit_reject"), loggateway.Err(err))
			return nil, err
		}
	}
	return p.inner.ProcessMessage(ctx, msg, opts, handler)
}

// messageFilterRewriter adapts a MessageFilterHook to the framework's
// ResponseRewriter interface.
type messageFilterRewriter struct {
	filterHook MessageFilterHook
}

func (r *messageFilterRewriter) RewriteUnary(ctx context.Context, result a2ago.UnaryMessageResult) a2ago.UnaryMessageResult {
	if r.filterHook == nil {
		return result
	}
	return r.filterHook(ctx, result)
}

func (r *messageFilterRewriter) RewriteStreaming(ctx context.Context, result a2ago.StreamingMessageResult) a2ago.StreamingMessageResult {
	// Streaming results use a different type; pass through without filtering
	// since the filter hook is typed for unary results. Streaming filtering
	// can be added later if needed.
	return result
}
