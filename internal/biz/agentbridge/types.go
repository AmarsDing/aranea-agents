// Package agentbridge 是编程 Agent 桥接的领域层：
// 精灵助手经 ACP 协议驱动外部编程 CLI（Claude Code / Codex / CodeBuddy）
// 在本机注册项目目录执行任务，支持审批中继与结果回收。
package agentbridge

// TaskStatus 是任务状态机枚举（task_state_machine.go 定义合法转换）。
type TaskStatus string

const (
	StatusDispatched       TaskStatus = "dispatched"
	StatusRunning          TaskStatus = "running"
	StatusAwaitingApproval TaskStatus = "awaiting_approval"
	StatusCancelling       TaskStatus = "cancelling"
	StatusDone             TaskStatus = "done"
	StatusFailed           TaskStatus = "failed"
	StatusCancelled        TaskStatus = "cancelled"
)

// IsTerminal 报告状态是否为终态。
func (s TaskStatus) IsTerminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusCancelled
}

// CodingAgent 是一个已注册的外部编程 CLI Agent（ACP stdio 子进程）。
type CodingAgent struct {
	ID             string
	Workspace      string
	AgentKey       string // claude_code / codex / codebuddy
	DisplayName    string
	Command        string
	Args           []string
	Env            map[string]string
	Enabled        bool
	LastProbeOK    bool
	LastProbeError string
	// LivenessOverride M4 P3-6：按 key 覆盖 liveness 阈值（JSON，空=用全局默认）
	LivenessOverride map[string]any
	// 03 P0：注册表驱动前端渲染——图标（静态资源路径或 URL，空=前端首字母占位）、
	// 能力声明（read_workspace/write_workspace/exec_command）、默认自治级别（readonly/workspace/full）。
	IconURL         string
	Capabilities    []string
	DefaultAutonomy string
	CreatedAt       string
	UpdatedAt       string
}

// CodingProject 是一个可派发任务的本机项目目录（cwd 白名单）。
type CodingProject struct {
	ID          string
	Workspace   string
	Name        string
	Path        string
	Description string
	CreatedAt   string
	UpdatedAt   string
}

// CodingTask 是一次外部编程 agent 任务执行记录。
type CodingTask struct {
	ID            string
	Workspace     string
	SessionID     string // 发起任务的精灵会话（审批卡片路由目标）
	AgentID       string
	ProjectID     string
	Prompt        string
	Status        TaskStatus
	ACPSessionID  string
	Summary       string
	Error         string
	ProgressCount int
	// ChildPID M4 P0-S4：子进程 PID（Windows 为进程组根，重启收割用）
	ChildPID int
	// M4 活跃性监管（设计「活跃性监管」§B）
	LastActiveAt  string // RFC3339；为空表示尚无活动
	LastExcerpt   string
	HeartbeatMiss int
	LivenessState string // healthy / silent / suspect / lost
	// 03 P0：发起来源（chat=会话内 LLM 派发 / workbench=看板表单直发）与验收结果。
	Origin        string
	ReviewStatus  string // "" / approved / rejected / archived
	ReviewComment string
	ReviewedAt    string
	CreatedAt     string
	UpdatedAt     string
	CompletedAt   string
}

// TaskPatch 是 UpdateStatus CAS 时携带的字段补丁（空值字段不更新）。
type TaskPatch struct {
	ACPSessionID  *string
	Summary       *string
	Error         *string
	CompletedAt   *string
	ProgressCount *int
	ChildPID      *int    // M4 P0-2b：Spawn 后写回子进程 PID
	LastActiveAt  *string // M4：liveness 字段（同态 CAS 更新）
	LastExcerpt   *string
	HeartbeatMiss *int
	LivenessState *string
}

// ProjectCandidate 是项目名消歧候选。
type ProjectCandidate struct {
	ID          string
	Name        string
	Path        string
	Description string
}

// CodingTaskEvent 统一步骤事件（03 P0 ④：所有工具运行输出归一化为同一套
// 步骤结构并持久化，支持刷新后回看与回放；借鉴 vibe-kanban NormalizedEntry
// crates/executors/src/logs/mod.rs，裁剪为 seq 游标 + kind + title + text）。
type CodingTaskEvent struct {
	ID        string
	TaskID    string
	Seq       int64  // 任务内单调序号（回放/增量拉取游标）
	Kind      string // agent_message_chunk / tool_call / plan / permission / error / heartbeat
	Title     string // 步骤标题（tool_call 的工具名等）
	Text      string // 步骤详情（截断存储）
	CreatedAt string
}
