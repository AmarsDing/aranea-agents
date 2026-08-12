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
	CreatedAt      string
	UpdatedAt      string
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
}

// ProjectCandidate 是项目名消歧候选。
type ProjectCandidate struct {
	ID          string
	Name        string
	Path        string
	Description string
}
