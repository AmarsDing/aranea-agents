// Package fleet 是 Harness 多任务运行台（复刻 stablyai/orca 的工作方式）：
// 每个任务一个 git worktree，worktree 里直接起编码 agent 的命令行终端，
// 人看状态灯、在 diff 上逐行批注后整批发回 agent、提交推送；
// 另有协调者编排（一个 agent 拆任务、派 worker、收结果）、自动化定时任务
// 和本机 agent 用量统计。
//
// 与 agentbridge 的关系：agentbridge 走 ACP 协议、以「任务」为单位；
// fleet 直接跑交互式 CLI、以「worktree + 终端」为单位。两者并存，
// 项目登记（coding_projects）共用。
package fleet

import "time"

// 终端状态。agent 终端在 starting/working/needs_input/done/idle 间流转，
// 进程退出后落 exited/failed；普通 shell 恒为 shell（不显示状态灯）。
const (
	StatusStarting   = "starting"
	StatusWorking    = "working"
	StatusNeedsInput = "needs_input"
	StatusDone       = "done"
	StatusIdle       = "idle"
	StatusExited     = "exited"
	StatusFailed     = "failed"
	StatusShell      = "shell"
)

// worktree 状态。
const (
	WorkspaceCreating = "creating"
	WorkspaceReady    = "ready"
	WorkspaceFailed   = "failed"
	WorkspaceDeleting = "deleting"
)

// worktree 种类：primary = 项目主检出（不可删），worktree = 运行台建的。
const (
	KindPrimary  = "primary"
	KindWorktree = "worktree"
)

// 事件种类（动态流与通知）。
const (
	EventAgentDone      = "agent_done"
	EventNeedsInput     = "needs_input"
	EventExited         = "exited"
	EventWorktreeReady  = "worktree_ready"
	EventWorktreeFailed = "worktree_failed"
	EventWorkerDone     = "worker_done"
	EventQuestion       = "question"
	EventAutomation     = "automation"
)

// Project 是登记在 coding_projects 里的一个本机仓库。
type Project struct {
	ID            string
	Name          string
	Path          string
	DefaultBranch string
	IsGit         bool
}

// Workspace 是一个 worktree（或项目主检出）。
type Workspace struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	Branch       string `json:"branch"`
	BaseRef      string `json:"base_ref"`
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	Pinned       bool   `json:"pinned,omitempty"`
	Archived     bool   `json:"archived,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	Origin       string `json:"origin,omitempty"`
	LinkedIssue  string `json:"linked_issue,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LastActiveAt string `json:"last_active_at,omitempty"`
}

// TerminalInfo 是终端的对外快照（运行态在 Session 里）。
type TerminalInfo struct {
	ID              string
	WorkspaceID     string
	Agent           string
	Title           string
	Status          string
	FullAccess      bool
	PID             int
	StartedAt       time.Time
	ExitCode        int
	LastActivityAt  time.Time
	StatusChangedAt time.Time
	Preview         string
	RunID           string
	TaskID          string
	Role            string
}

// Note 是 diff 上的一条行批注。
type Note struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Side        string `json:"side"`
	Body        string `json:"body"`
	LineText    string `json:"line_text,omitempty"`
	Resolved    bool   `json:"resolved,omitempty"`
	SentAt      string `json:"sent_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Event 是动态流里的一条记录。
type Event struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	TerminalID  string `json:"terminal_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	Read        bool   `json:"read,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Run 是一次协调者编排。
type Run struct {
	ID                    string `json:"id"`
	Objective             string `json:"objective"`
	ProjectID             string `json:"project_id"`
	WorkspaceID           string `json:"workspace_id"`
	CoordinatorTerminalID string `json:"coordinator_terminal_id"`
	Agent                 string `json:"agent"`
	FullAccess            bool   `json:"full_access,omitempty"`
	Status                string `json:"status"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

// Task 是编排里的一个子任务；dispatch 是它在某个 worker 终端上的一次尝试。
type Task struct {
	ID                string   `json:"id"`
	RunID             string   `json:"run_id"`
	Title             string   `json:"title"`
	Spec              string   `json:"spec"`
	Status            string   `json:"status"`
	Deps              []string `json:"deps,omitempty"`
	WorkerTerminalID  string   `json:"worker_terminal_id,omitempty"`
	WorkerWorkspaceID string   `json:"worker_workspace_id,omitempty"`
	WorkerAgent       string   `json:"worker_agent,omitempty"`
	DispatchID        string   `json:"dispatch_id,omitempty"`
	Outcome           string   `json:"outcome,omitempty"`
	Summary           string   `json:"summary,omitempty"`
	FilesModified     []string `json:"files_modified,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// Message 是编排收件箱里的一封信。
type Message struct {
	ID         string   `json:"id"`
	RunID      string   `json:"run_id"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Type       string   `json:"type"`
	Subject    string   `json:"subject,omitempty"`
	Body       string   `json:"body,omitempty"`
	TaskID     string   `json:"task_id,omitempty"`
	DispatchID string   `json:"dispatch_id,omitempty"`
	Options    []string `json:"options,omitempty"`
	Read       bool     `json:"read,omitempty"`
	Answer     string   `json:"answer,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

// Automation 是一条定时任务：按 Schedule 在项目上起 agent 跑 Prompt。
type Automation struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ProjectID       string `json:"project_id"`
	Agent           string `json:"agent"`
	Prompt          string `json:"prompt"`
	Schedule        string `json:"schedule"`
	WorktreeMode    string `json:"worktree_mode"`
	FullAccess      bool   `json:"full_access,omitempty"`
	Enabled         bool   `json:"enabled"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	LastStatus      string `json:"last_status,omitempty"`
	LastWorkspaceID string `json:"last_workspace_id,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// UsageWindow 是一个统计窗口（5h / 7d）。Percent<0 表示日志里没有额度信息。
type UsageWindow struct {
	Label    string
	Tokens   int64
	Percent  float64
	ResetsAt string
}

// AgentUsage 是一个 agent 的用量。
type AgentUsage struct {
	Agent     string
	Windows   []UsageWindow
	Source    string
	UpdatedAt string
	Sessions  int64

	// 最近一次 Codex 额度快照里的套餐与积分（只有 codex 有），给平台积分用，不进 /v1/fleet/usage。
	codexLimits *codexRateLimits
	codexAt     int64
}

// 存储里的记录种类。
const (
	kindWorkspace  = "workspace"
	kindNote       = "note"
	kindEvent      = "event"
	kindRun        = "run"
	kindTask       = "task"
	kindMessage    = "message"
	kindAutomation = "automation"
)

func nowString() string { return time.Now().UTC().Format(time.RFC3339) }
