// Package clientbridge implements the backend half of the client tool
// bridge: Agent-facing tools (client_open_app / client_open_url) do not
// execute locally — the bridge registers a pending invocation, routes a
// client_tool.invoke downstream message to the desktop companion over the
// session WebSocket, and waits for the client_tool.result uplink (or a
// timeout) before returning the tool result to the Turn.
//
// See docs/development/74-voice-companion.design.md §6.
package clientbridge

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Step IDs for flow logs (流程日志). Registered in
// internal/event/flow_log.go stepTitleRegistry + 52-flow-logger §5.1.
const (
	StepInvoke  = "client_tool.invoke"
	StepResult  = "client_tool.result"
	StepTimeout = "client_tool.timeout"
)

// Audit actions recorded per invocation lifecycle.
const (
	AuditActionInvoke  = "invoke"
	AuditActionResult  = "result"
	AuditActionOffline = "offline"
	AuditActionTimeout = "timeout"
)

// Structured error codes surfaced to the Agent so it can paraphrase the
// failure to the user (e.g. "桌面客户端未连接").
const (
	ErrCodeOffline = "DESKTOP_CLIENT_OFFLINE"
	ErrCodeTimeout = "CLIENT_TOOL_TIMEOUT"
)

// DefaultTimeout bounds a pending invocation when Deps.Timeout is unset.
// Design §6.2: 30s.
const DefaultTimeout = 30 * time.Second

// LogPair aliases biz.LogPair so this package's ports speak the shared
// structured-logging vocabulary without re-defining it.
type LogPair = biz.LogPair

// FlowLogWriter aliases the biz flow-log port (nil-safe; tests may pass nil).
type FlowLogWriter = biz.FlowLogWriter

// Error is the structured bridge error. Code is one of ErrCode*.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// InvokeRequest is one Agent tool call to be executed on the client.
type InvokeRequest struct {
	SessionID string
	UserID    string
	Tool      string // full runtime tool name, e.g. client_open_app
	Args      []byte // raw JSON arguments, forwarded verbatim
}

// InvokeMessage is the downstream payload routed to the client connection.
type InvokeMessage struct {
	InvocationID string
	SessionID    string
	UserID       string
	Tool         string
	Args         []byte
}

// InvokeResult is the resolved client execution outcome. A failed client
// execution is reported here (OK=false + Error), not as a Go error; Go
// errors are reserved for bridge-level failures (offline/timeout).
type InvokeResult struct {
	OK     bool
	Output string
	Error  string
}

// AuditEntry is one audit record for the invocation lifecycle.
type AuditEntry struct {
	Action       string // AuditAction*
	InvocationID string
	SessionID    string
	UserID       string
	Tool         string
	Detail       string
	CreatedAt    time.Time
}

// AuditRecorder persists audit entries. Implemented in the service/data
// layer; nil-safe (audit is best-effort beside the invocation path).
type AuditRecorder interface {
	RecordClientToolAudit(ctx context.Context, e AuditEntry)
}

// Router delivers the invoke message to an eligible client connection of the
// session and reports whether at least one connection accepted it.
// Implemented by the server WS layer (capabilities-filtered fan-out).
type Router interface {
	RouteClientToolInvoke(sessionID string, msg InvokeMessage) bool
}

// Deps wires the bridge's collaborators.
type Deps struct {
	Timeout time.Duration // per-invocation result wait; <=0 → DefaultTimeout
	Audit   AuditRecorder // nil → audit skipped
	Flow    FlowLogWriter // nil → flow logs skipped
	LG      loggateway.Logger
}

// pending tracks one in-flight invocation awaiting the client result.
type pending struct {
	sessionID string
	userID    string
	tool      string
	resultCh  chan InvokeResult // buffered(1); ResolveResult never blocks
}

// Bridge is the client tool invocation coordinator. Concurrency-safe.
type Bridge struct {
	timeout time.Duration
	audit   AuditRecorder
	flow    FlowLogWriter
	lg      loggateway.Logger

	mu      sync.Mutex
	router  Router // set post-construction by the server layer
	pending map[string]*pending
}

// NewBridge creates a Bridge. LG nil falls back to a noop logger.
func NewBridge(d Deps) *Bridge {
	lg := d.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Bridge{
		timeout: timeout,
		audit:   d.Audit,
		flow:    d.Flow,
		lg:      lg.With(loggateway.Domain("client_tool")),
		pending: make(map[string]*pending),
	}
}

// SetRouter installs the downstream router. Called once by the server layer
// during startup wiring; invocations before that fail offline.
func (b *Bridge) SetRouter(r Router) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.router = r
}

// PendingCount reports the number of in-flight invocations (observability +
// tests).
func (b *Bridge) PendingCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// Invoke registers the invocation, routes it to the client, and blocks until
// the client result, the per-invocation timeout, or ctx cancellation.
func (b *Bridge) Invoke(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
	if req.SessionID == "" {
		return InvokeResult{}, errors.New("clientbridge: session id is required")
	}
	if req.Tool == "" {
		return InvokeResult{}, errors.New("clientbridge: tool is required")
	}

	b.mu.Lock()
	router := b.router
	b.mu.Unlock()
	if router == nil {
		b.recordAudit(ctx, AuditActionOffline, "", req, "no router installed")
		b.logFlowError(ctx, req.SessionID, StepInvoke, "桌面客户端未连接")
		return InvokeResult{}, &Error{Code: ErrCodeOffline, Message: "client tool router is not installed"}
	}

	invID := uuid.NewString()
	p := &pending{
		sessionID: req.SessionID,
		userID:    req.UserID,
		tool:      req.Tool,
		resultCh:  make(chan InvokeResult, 1),
	}
	b.mu.Lock()
	b.pending[invID] = p
	b.mu.Unlock()

	delivered := router.RouteClientToolInvoke(req.SessionID, InvokeMessage{
		InvocationID: invID,
		SessionID:    req.SessionID,
		UserID:       req.UserID,
		Tool:         req.Tool,
		Args:         req.Args,
	})
	if !delivered {
		b.removePending(invID)
		b.recordAudit(ctx, AuditActionOffline, invID, req, "no eligible client connection")
		b.logFlowError(ctx, req.SessionID, StepInvoke, "桌面客户端未连接")
		b.lg.Warn("client tool invoke offline: no eligible connection",
			loggateway.SessionID(req.SessionID), loggateway.Str("tool", req.Tool))
		return InvokeResult{}, &Error{Code: ErrCodeOffline, Message: "no eligible desktop companion connection for session"}
	}

	b.recordAudit(ctx, AuditActionInvoke, invID, req, "")
	b.logFlowStart(ctx, req.SessionID, StepInvoke, "调用客户端工具")

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case res := <-p.resultCh:
		b.recordAudit(ctx, AuditActionResult, invID, req, resultDetail(res))
		b.logFlowDone(ctx, req.SessionID, StepResult, "客户端工具执行完成")
		return res, nil
	case <-timer.C:
		b.removePending(invID)
		b.recordAudit(ctx, AuditActionTimeout, invID, req, b.timeout.String())
		b.logFlowError(ctx, req.SessionID, StepTimeout, "客户端工具执行超时")
		b.lg.Warn("client tool invoke timeout",
			loggateway.SessionID(req.SessionID), loggateway.Str("tool", req.Tool),
			loggateway.Str("invocation_id", invID))
		return InvokeResult{}, &Error{Code: ErrCodeTimeout, Message: "client tool result not received within " + b.timeout.String()}
	case <-ctx.Done():
		b.removePending(invID)
		return InvokeResult{}, ctx.Err()
	}
}

// ResolveResult completes a pending invocation from a client_tool.result
// uplink. Returns false when the invocation is unknown (never registered,
// already resolved, or already timed out) so the server can drop the frame.
func (b *Bridge) ResolveResult(invocationID string, ok bool, output, errText string) bool {
	b.mu.Lock()
	p, found := b.pending[invocationID]
	if found {
		delete(b.pending, invocationID)
	}
	b.mu.Unlock()
	if !found {
		return false
	}
	p.resultCh <- InvokeResult{OK: ok, Output: output, Error: errText}
	return true
}

// removePending deletes the invocation without resolving it (timeout/cancel
// paths; the waiter has already given up).
func (b *Bridge) removePending(invocationID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, invocationID)
}

// recordAudit writes one audit entry when an audit recorder is configured.
func (b *Bridge) recordAudit(ctx context.Context, action, invocationID string, req InvokeRequest, detail string) {
	if b.audit == nil {
		return
	}
	b.audit.RecordClientToolAudit(ctx, AuditEntry{
		Action:       action,
		InvocationID: invocationID,
		SessionID:    req.SessionID,
		UserID:       req.UserID,
		Tool:         req.Tool,
		Detail:       detail,
		CreatedAt:    time.Now(),
	})
}

func (b *Bridge) logFlowStart(ctx context.Context, sessionID, stepID, message string) {
	if b.flow != nil {
		b.flow.LogFlowStart(ctx, sessionID, stepID, message)
	}
}

func (b *Bridge) logFlowDone(ctx context.Context, sessionID, stepID, message string) {
	if b.flow != nil {
		b.flow.LogFlowDone(ctx, sessionID, stepID, message)
	}
}

func (b *Bridge) logFlowError(ctx context.Context, sessionID, stepID, message string) {
	if b.flow != nil {
		b.flow.LogFlowError(ctx, sessionID, stepID, message)
	}
}

func resultDetail(res InvokeResult) string {
	if res.OK {
		return "ok"
	}
	return res.Error
}
