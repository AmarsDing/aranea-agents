package agentbridge

import "context"

// AgentRepo 编程工具注册表读写。
// Stability:evolving
type AgentRepo interface {
	GetByKey(ctx context.Context, workspace, key string) (*CodingAgent, error)
	List(ctx context.Context, workspace string) ([]*CodingAgent, error)
	Upsert(ctx context.Context, agent *CodingAgent) error
	UpdateProbe(ctx context.Context, id string, ok bool, errMsg string) error
}

// ProjectRepo 项目目录注册表读写。
// Stability:evolving
type ProjectRepo interface {
	GetByName(ctx context.Context, workspace, name string) (*CodingProject, error)
	// Match 按 精确 → 前缀 → 包含 顺序返回候选（消歧用）。
	Match(ctx context.Context, workspace, query string) ([]*CodingProject, error)
	List(ctx context.Context, workspace string) ([]*CodingProject, error)
	Upsert(ctx context.Context, p *CodingProject) error
	Delete(ctx context.Context, id string) error
}

// TaskRepo 任务记录读写。
// Stability:evolving
type TaskRepo interface {
	Create(ctx context.Context, t *CodingTask) error
	Get(ctx context.Context, id string) (*CodingTask, error)
	// UpdateStatus 以 CAS 方式转换状态：当前状态不等于 from 时返回冲突错误。
	UpdateStatus(ctx context.Context, id string, from, to TaskStatus, patch TaskPatch) error
	ListBySession(ctx context.Context, sessionID string, limit int) ([]*CodingTask, error)
	// ListActive 返回所有非终态任务（服务重启恢复用）。
	ListActive(ctx context.Context) ([]*CodingTask, error)
}

// ACPSession 是 biz 对 ACP 协议层的端口（service 层实现注入，便于 mock）。
// Stability:internal
type ACPSession interface {
	// Prompt 阻塞执行至 agent 完成，返回结果摘要。
	Prompt(ctx context.Context, cwd, prompt string, h EventHandler) (string, error)
	Cancel(ctx context.Context) error
	Close() error
}

// EventHandler 接收 ACP 流式事件（service 层注入实现）。
// OnPermission 可阻塞等待用户审批，但实现必须带超时。
type EventHandler interface {
	OnUpdate(kind, text string)
	OnPermission(ctx context.Context, title string, options []PermissionOption) (optionID string, err error)
}

// TaskListener 接收任务终态快照（service 层注入：事件发射/播报）。
// 实现必须快速返回（在状态机推进路径上同步调用）。
// Stability:internal
type TaskListener interface {
	OnTaskTerminal(t *CodingTask)
}

// PermissionOption 是审批可选项（映射 ACP permission options）。
type PermissionOption struct {
	OptionID string
	Name     string
	Kind     string // allow_once / allow_always / reject_once / reject_always
}
