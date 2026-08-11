// Package acp 实现 ACP（Agent Client Protocol）客户端子集。
// 协议：JSON-RPC 2.0 over stdio NDJSON。仅实现本模块所需方法：
// initialize / session/new / session/prompt / session/cancel（出站），
// session/update / session/request_permission（入站）。
// 参考：agentclientprotocol.com（protocol version 1）。
package acp

import "encoding/json"

// ProtocolVersion 是当前协商的 ACP 协议版本。
const ProtocolVersion = 1

// ACP 方法名。
const (
	MethodInitialize        = "initialize"
	MethodSessionNew        = "session/new"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// StopReason 是 session/prompt 的结束原因。
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonCancelled = "cancelled"
)

// Permission outcome 取值。
const (
	PermissionOutcomeSelected  = "selected"
	PermissionOutcomeCancelled = "cancelled"
)

// ClientCapabilities 声明客户端能力（本实现全部关闭：文件/终端由 agent 本地执行）。
type ClientCapabilities struct {
	FS       FileSystemCapabilities `json:"fs"`
	Terminal bool                   `json:"terminal"`
}

// FileSystemCapabilities 对应 ACP fs 能力组。
type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// InitializeParams 是 initialize 请求参数。
type InitializeParams struct {
	ProtocolVersion   int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
}

// InitializeResult 是 initialize 响应。
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
}

// AgentCapabilities 只保留关心的字段。
type AgentCapabilities struct {
	LoadSession       bool `json:"loadSession,omitempty"`
	EmbeddedContext   bool `json:"-"` // 占位：暂不使用 promptCapabilities
}

// NewSessionParams 是 session/new 请求参数。
type NewSessionParams struct {
	CWD        string `json:"cwd"`
	MCPServers []any  `json:"mcpServers"`
}

// NewSessionResult 是 session/new 响应。
type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

// ContentBlock 是 ACP 内容块（只用 text）。
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PromptParams 是 session/prompt 请求参数。
type PromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// PromptResult 是 session/prompt 响应。
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// CancelNotificationParams 是 session/cancel 通知参数。
type CancelNotificationParams struct {
	SessionID string `json:"sessionId"`
}

// SessionNotification 是 session/update 通知参数。
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate 是流式更新载荷（只保留关心字段，未知字段忽略）。
type SessionUpdate struct {
	Kind    string        `json:"sessionUpdate"`     // agent_message_chunk / tool_call / plan / ...
	Content *ContentBlock `json:"content,omitempty"` // agent_message_chunk 的文本
	Title   string        `json:"title,omitempty"`   // tool_call 标题
	ToolCallID string      `json:"toolCallId,omitempty"`
}

// ToolCallInfo 是 request_permission 中的工具调用信息。
type ToolCallInfo struct {
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title"`
	Kind       string          `json:"kind,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
}

// PermissionOption 是可选项（allow_once / allow_always / reject_once / reject_always）。
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// PermissionRequestParams 是 session/request_permission 请求参数。
type PermissionRequestParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallInfo       `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionOutcome 是审批结论。
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`            // selected / cancelled
	OptionID string `json:"optionId,omitempty"` // outcome=selected 时的选项
}

// PermissionResult 是 session/request_permission 响应。
type PermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
}
