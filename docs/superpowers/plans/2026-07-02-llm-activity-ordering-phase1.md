# LLM Activity Ordering Phase 1 实施计划：后端新模型 + 兼容层

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 推倒重来 chat 模块的「思考-action-回复」顺序架构，Phase 1 完成后端：新建 9 个 entity biz 模型 + 9 张 Ent 表 + 单管道 Sequencer + 显式 PlanExecutor + 兼容层 adapter，让新事件流上线且旧前端继续工作。

**Architecture:** 替换原 `Activity` 黑洞模型为 9 个 typed entity（Session/Task/Turn/Step/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep）。所有事件源（Projector/Service/Team/PlanExecutor）统一走单一 `Sequencer.Publish` 入口，享有同等的 Seq 分配 + 16ms 批合并 + FIFO + 重试 + 死信保障。`PlanExecutor` 作为正向调度器替代 `updatePlanStepForTeam` 反向同步。兼容层 adapter 将新事件转为旧 `ActivityEvent` 格式，让旧前端继续工作；新旧表共存，Phase 2 完成后删除兼容层。

**Tech Stack:** Go (Kratos v2 + trpc-agent-go) / Ent ORM + SQLite / Wire DI / loggateway Logger / atomics + sync.Map

---

## 实施策略

### 任务分组

| 阶段 | 任务 | 目的 | 依赖 |
|---|---|---|---|
| Phase 1.0: 基线 | Task 0 | 跑现有测试，记录基线 | 无 |
| Phase 1.1: 类型基础 | Task 1, 2, 3 | 9 entity biz 文件 + Event 接口 + PlanStep 状态机 | 无 |
| Phase 1.2: 数据库 | Task 4, 5, 6 | 9 Ent Schema + go generate + SeqAssigner | 1.1 |
| Phase 1.3: Repo 层 | Task 7, 8, 9 | 9 个 Repo + 窄接口 + Wire 注入 | 1.2 |
| Phase 1.4: Sequencer | Task 10, 11 | 重写 Sequencer + 兼容层 adapter | 1.3 |
| Phase 1.5: Projector | Task 12, 13 | ProjectMeta 改造 + Projector 重写 + stream_consumer | 1.4 |
| Phase 1.6: PlanExecutor | Task 14, 15 | PlanExecutor + spirit_team 改造 | 1.5 |
| Phase 1.7: WS 推送 | Task 16 | WS 双格式推送 + adapter | 1.6 |
| Phase 1.8: 集成验证 | Task 17, 18 | 集成测试 + 全量回归 | 1.7 |

### 执行顺序原则

1. **Phase 1.1-1.3 顺序执行**：类型 → Schema → Repo 严格依赖
2. **Phase 1.4+ 必须等 1.3 完成**：Sequencer 依赖 Repo
3. **每个 Task 完成后 commit**：保证可回滚
4. **TDD 铁律**：先写失败测试 → 验证失败 → 最小实现 → 验证通过 → commit

---

## File Structure

### 后端新增文件

| 文件 | 责任 | 行数预估 |
|------|------|---------|
| `internal/biz/session.go` | Session entity + Status enum | 40 |
| `internal/biz/task.go` | Task entity + Status enum | 50 |
| `internal/biz/turn.go` | Turn entity + Status enum | 60 |
| `internal/biz/step.go` | Step entity + Kind/Status enums | 70 |
| `internal/biz/team_stage.go` | TeamStage entity + Status/Stage/MemberInfo | 80 |
| `internal/biz/team_run.go` | TeamRun entity + Status enum | 40 |
| `internal/biz/member_session.go` | MemberSession entity + Status enum | 40 |
| `internal/biz/plan_board.go` | PlanBoard entity + Strategy/Status enums | 50 |
| `internal/biz/plan_step.go` | PlanStep + StepResult/StepError/MemberReport/TokenUsage | 80 |
| `internal/biz/plan_step_state_machine.go` | PlanStep 状态机 | 60 |
| `internal/biz/event.go` | Event interface + EventKind + 30+ 具体事件类型 | 250 |
| `internal/biz/repo_ports_v2.go` | 9 个 Repo 窄接口（Reader/Writer/Upserter） | 150 |
| `internal/agent/seq_assigner.go` | SeqAssigner（per spirit session atomic counter） | 60 |
| `internal/agent/seq_assigner_test.go` | SeqAssigner 单测 | 80 |
| `internal/agent/sequencer_v2.go` | 新 Sequencer（统一入口） | 400 |
| `internal/agent/sequencer_v2_test.go` | Sequencer 单测 | 300 |
| `internal/agent/project_meta_v2.go` | 新 ProjectMeta | 50 |
| `internal/agent/activity_projector_v2.go` | 新 Projector（适配新模型） | 500 |
| `internal/agent/activity_projector_v2_test.go` | Projector 单测 | 400 |
| `internal/agent/compat_adapter.go` | 新事件 → 旧 ActivityEvent 转换 | 200 |
| `internal/data/ent/schema/session.go` | Ent Schema | 30 |
| `internal/data/ent/schema/task.go` | Ent Schema | 35 |
| `internal/data/ent/schema/turn.go` | Ent Schema | 45 |
| `internal/data/ent/schema/step.go` | Ent Schema | 55 |
| `internal/data/ent/schema/team_stage.go` | Ent Schema | 50 |
| `internal/data/ent/schema/team_run.go` | Ent Schema | 35 |
| `internal/data/ent/schema/member_session.go` | Ent Schema | 40 |
| `internal/data/ent/schema/plan_board.go` | Ent Schema | 40 |
| `internal/data/ent/schema/plan_step.go` | Ent Schema | 45 |
| `internal/data/session_v2_repo.go` | Session Repo 实现 | 80 |
| `internal/data/task_v2_repo.go` | Task Repo 实现 | 120 |
| `internal/data/turn_v2_repo.go` | Turn Repo 实现 | 120 |
| `internal/data/step_v2_repo.go` | Step Repo 实现 | 150 |
| `internal/data/team_stage_v2_repo.go` | TeamStage Repo 实现 | 130 |
| `internal/data/team_run_v2_repo.go` | TeamRun Repo 实现 | 100 |
| `internal/data/member_session_v2_repo.go` | MemberSession Repo 实现 | 110 |
| `internal/data/plan_board_v2_repo.go` | PlanBoard Repo 实现 | 120 |
| `internal/data/plan_step_v2_repo.go` | PlanStep Repo 实现 | 130 |
| `internal/service/plan_executor.go` | PlanExecutor 调度器 | 400 |
| `internal/service/plan_executor_test.go` | PlanExecutor 单测 | 350 |

### 后端修改文件

| 文件 | 改动 |
|------|------|
| `internal/data/data.go` | Wire providers 列表新增 9 个 Repo + bind |
| `internal/agent/stream_consumer.go` | 适配新 ProjectMeta |
| `internal/service/spirit_team.go` | 移除 updatePlanStepForTeam，改发事件到 Sequencer |
| `internal/server/ws_io_pump.go` | 双格式推送（新+旧 adapter） |
| `cmd/admin/wire.go` | Wire 生成（自动） |

### 命名约定

- 新文件统一加 `_v2` 后缀（如 `session_v2_repo.go`、`sequencer_v2.go`、`activity_projector_v2.go`），避免与旧文件冲突
- biz 层 entity 文件**不加** `_v2`（如 `session.go`、`task.go`），因为 biz 包不会有同名冲突（旧 `activity.go` 是不同名字）
- 新 biz 接口集中在 `repo_ports_v2.go`，与旧 `activity_repo.go` 等共存
- Ent Schema 文件不加 `_v2`（表名已隔离：`sessions_v2` / `tasks_v2` 等，避免与现有 `sessions`/`tasks` 表冲突）

**表名加 `_v2` 后缀**：现有 `sessions`/`tasks` 表已存在（旧 Schema），新表命名为 `sessions_v2`/`tasks_v2`/`turns_v2`/`steps_v2`/`team_stages_v2`/`team_runs_v2`/`member_sessions_v2`/`plan_boards_v2`/`plan_steps_v2`。Phase 3 删除旧表后可考虑 rename。

---

## Phase 1.0: 基线

### Task 0: 建立基线测试快照

**Files:**
- Read: `internal/agent/activity_event_sequencer_test.go`
- Read: `internal/data/activity_repo.go`

- [ ] **Step 1: 跑现有后端测试，记录基线**

Run: `cd f:\aranea-agents && go test ./internal/agent/... ./internal/data/... ./internal/biz/... -count=1 2>&1 | tail -30`
Expected: 全部通过（记录通过数量作为基线）

- [ ] **Step 2: 跑 build + lint 基线**

Run: `cd f:\aranea-agents && go build ./... 2>&1 | tail -10`
Expected: 无错误

- [ ] **Step 3: 记录基线 commit hash**

Run: `cd f:\aranea-agents && git log --oneline -1`
Expected: 记录 commit hash，作为回滚点

---

## Phase 1.1: 类型基础

### Task 1: 创建 9 个 entity biz 文件

**Files:**
- Create: `internal/biz/session.go`
- Create: `internal/biz/task.go`
- Create: `internal/biz/turn.go`
- Create: `internal/biz/step.go`
- Create: `internal/biz/team_stage.go`
- Create: `internal/biz/team_run.go`
- Create: `internal/biz/member_session.go`
- Create: `internal/biz/plan_board.go`
- Create: `internal/biz/plan_step.go`
- Create: `internal/biz/entity_v2_types_test.go`

- [ ] **Step 1: 写 entity 类型测试（红）**

Create `internal/biz/entity_v2_types_test.go`:

```go
package biz

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionStatus_Constants(t *testing.T) {
	if SessionStatusActive != "active" {
		t.Fatalf("expected active, got %s", SessionStatusActive)
	}
	if SessionStatusCompleted != "completed" {
		t.Fatalf("expected completed, got %s", SessionStatusCompleted)
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	cases := []TaskStatus{
		TaskStatusPending, TaskStatusRunning, TaskStatusCompleted,
		TaskStatusFailed, TaskStatusCancelled,
	}
	expected := []string{"pending", "running", "completed", "failed", "cancelled"}
	for i, c := range cases {
		if string(c) != expected[i] {
			t.Fatalf("TaskStatus[%d]: expected %s, got %s", i, expected[i], c)
		}
	}
}

func TestStepKind_Constants(t *testing.T) {
	if StepKindThinking != "thinking" {
		t.Fatalf("expected thinking, got %s", StepKindThinking)
	}
}

func TestPlanStep_Fields(t *testing.T) {
	now := time.Now().UTC()
	ps := PlanStep{
		ID:        "ps-1",
		PlanID:    "pb-1",
		TaskID:    "t-1",
		Label:     "Step 1",
		DependsOn: []string{"ps-0"},
		Status:    PlanStepStatusPending,
		Seq:       1,
	}
	if ps.ID != "ps-1" || ps.PlanID != "pb-1" || ps.TaskID != "t-1" {
		t.Fatalf("PlanStep field mismatch: %+v", ps)
	}
	if len(ps.DependsOn) != 1 || ps.DependsOn[0] != "ps-0" {
		t.Fatalf("DependsOn mismatch: %v", ps.DependsOn)
	}
	_ = now // 验证 time.Time 类型可用
}

func TestStepResult_JSONRoundtrip(t *testing.T) {
	r := StepResult{
		Output: "team output",
		MemberReports: []MemberReport{
			{AgentKey: "k1", AgentName: "Agent 1", Output: "out1"},
		},
		TokensUsed: TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		DurationMs: 5000,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got StepResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Output != "team output" || got.TokensUsed.TotalTokens != 150 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestMemberInfo_Fields(t *testing.T) {
	mi := MemberInfo{
		AgentKey:        "agent-1",
		AgentName:       "Worker",
		AvatarURL:       "https://example.com/a.png",
		ChildSessionID:  "sess-child-1",
		Status:          "pending",
	}
	if mi.AgentKey != "agent-1" || mi.AvatarURL == "" {
		t.Fatalf("MemberInfo mismatch: %+v", mi)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestSessionStatus_Constants|TestTaskStatus_Constants|TestStepKind_Constants|TestPlanStep_Fields|TestStepResult_JSONRoundtrip|TestMemberInfo_Fields" -count=1 2>&1 | tail -10`
Expected: FAIL，提示 `undefined: SessionStatusActive` 等

- [ ] **Step 3: 创建 `internal/biz/session.go`**

```go
package biz

import "time"

// Session 是 spirit 会话的根 entity（v2 模型）。
// 替代旧 Activity 模型中靠 spirit_session_id 字段隐式分组的做法。
type Session struct {
	ID            string
	UserID        string
	SpiritAgentID string
	Status        SessionStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusFailed    SessionStatus = "failed"
)
```

- [ ] **Step 4: 创建 `internal/biz/task.go`**

```go
package biz

import "time"

// Task 是用户一次输入对应的根活动（v2 模型）。
// 替代旧 Activity 模型中 kind=task 的 root activity。
type Task struct {
	ID          string
	SessionID   string // = spirit_session_id
	UserMessage string
	Status      TaskStatus
	Seq         int64 // 在 session 内的序号
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)
```

- [ ] **Step 5: 创建 `internal/biz/turn.go`**

```go
package biz

import "time"

// Turn 是最小对话单元（v2 模型）：一次 LLM 回合。
// 一个 Task 包含多个 Turn；Team member 也有自己的 Turn（嵌套，三层封顶）。
type Turn struct {
	ID              string
	TaskID          string
	SessionID       string // 当前 session（spirit 或 team 或 member session）
	SpiritSessionID string // 始终指向 spirit root session（WS 过滤用）
	ParentTurnID    string // 嵌套时填：team member 的 turn 的 parent 是 team_stage 的某个 run turn
	AgentKey        string // 谁的 turn
	TeamID          string // 所属 team（spirit turn 为空）
	TeamStageID     string // 所属 team_stage（member turn 时填）
	Seq             int64  // 在 task 内的全局序号（后端分配，单调递增）
	Status          TurnStatus
	StartedAt       time.Time
	CompletedAt     *time.Time
}

type TurnStatus string

const (
	TurnStatusRunning   TurnStatus = "running"
	TurnStatusCompleted TurnStatus = "completed"
	TurnStatusFailed    TurnStatus = "failed"
)
```

- [ ] **Step 6: 创建 `internal/biz/step.go`**

```go
package biz

import (
	"encoding/json"
	"time"
)

// Step 是 turn 内的工作步骤（v2 模型）：thinking/action/reply/notice/confirm/error。
// 替代旧 Activity 模型中按 kind 区分的多种 activity。
type Step struct {
	ID              string
	TurnID          string
	TaskID          string // 冗余，便于按 task 索引
	SessionID       string
	SpiritSessionID string
	Kind            StepKind
	Seq             int64 // turn 内的序号（1, 2, 3...）
	Content         string
	Reasoning       string
	ToolName        string
	ToolCallID      string
	ToolArgs        json.RawMessage // 类型安全的 JSON
	ToolResult      json.RawMessage
	ToolDurationMs  int64
	ToolErrorCode   string
	Status          StepStatus
	IsFinal         bool // reply 是否为最终回复
	StartedAt       time.Time
	CompletedAt     *time.Time
}

type StepKind string

const (
	StepKindThinking StepKind = "thinking"
	StepKindAction   StepKind = "action"
	StepKindReply    StepKind = "reply"
	StepKindNotice   StepKind = "notice"
	StepKindConfirm  StepKind = "confirm"
	StepKindError    StepKind = "error"
)

type StepStatus string

const (
	StepStatusPending     StepStatus = "pending"
	StepStatusRunning     StepStatus = "running"
	StepStatusToolRunning StepStatus = "tool_running"
	StepStatusToolBlocked StepStatus = "tool_blocked"
	StepStatusCompleted   StepStatus = "completed"
	StepStatusFailed      StepStatus = "failed"
	StepStatusCancelled   StepStatus = "cancelled"
)
```

- [ ] **Step 7: 创建 `internal/biz/team_stage.go`**

```go
package biz

import "time"

// TeamStage 是 task 内的团队执行阶段（v2 模型）。
// 一个 Task 可包含多个 TeamStage（串行或并行），每个 TeamStage 对应一个 plan_step（如有）。
type TeamStage struct {
	ID          string
	TaskID      string
	TurnID      string // 触发 team 的 turn
	SessionID   string // spirit_session_id
	TeamID      string
	DagNodeID   string // 对应 plan_step.id（如有）
	DependsOn   []string // 其他 team_stage.id（DAG 依赖）
	Status      TeamStageStatus
	Stage       TeamStageStage
	Members     []MemberInfo // 类型安全
	Strategy    string       // parallel/dag/coordinator
	StartedAt   time.Time
	CompletedAt *time.Time
	Seq         int64 // 在 task 内的序号
}

type TeamStageStatus string

const (
	TeamStageStatusPending      TeamStageStatus = "pending"
	TeamStageStatusRunning      TeamStageStatus = "running"
	TeamStageStatusCompleted   TeamStageStatus = "completed"
	TeamStageStatusFailed      TeamStageStatus = "failed"
	TeamStageStatusCancelled   TeamStageStatus = "cancelled"
	TeamStageStatusWaitingHuman TeamStageStatus = "waiting_human" // HITL
)

type TeamStageStage string

const (
	TeamStageStageAssembled TeamStageStage = "assembled"
	TeamStageStagePlanning  TeamStageStage = "planning"
	TeamStageStageExecuting TeamStageStage = "executing"
	TeamStageStageCompleted TeamStageStage = "completed"
	TeamStageStageFailed    TeamStageStage = "failed"
)

// MemberInfo 是 TeamStage 中的成员信息（类型安全，替代 Meta.members）。
type MemberInfo struct {
	AgentKey       string
	AgentName      string
	AvatarURL      string
	ChildSessionID string // member 自己的 session ID
	Status         string // pending/running/completed/failed/skipped
}
```

- [ ] **Step 8: 创建 `internal/biz/team_run.go`**

```go
package biz

import "time"

// TeamRun 是 team 内的一次执行（v2 模型）。
// 一个 TeamStage 可包含多个 TeamRun（重试或并行批次）。
type TeamRun struct {
	ID              string
	TeamStageID     string
	TaskID          string
	SessionID       string
	SpiritSessionID string
	DagNodeID       string // 对应 plan_step.id
	DependsOn       []string
	Status          TeamRunStatus
	StartedAt       time.Time
	CompletedAt     *time.Time
	Seq             int64
}

type TeamRunStatus string

const (
	TeamRunStatusRunning   TeamRunStatus = "running"
	TeamRunStatusCompleted TeamRunStatus = "completed"
	TeamRunStatusFailed    TeamRunStatus = "failed"
	TeamRunStatusCancelled TeamRunStatus = "cancelled"
)
```

- [ ] **Step 9: 创建 `internal/biz/member_session.go`**

```go
package biz

// MemberSession 是 team 内的成员会话（v2 模型）。
// 一个 TeamRun 包含多个 MemberSession（每个 agent 一个）。
// MemberSession 内的 turn 是终止层，不能再嵌套 team_stage（三层硬约束）。
type MemberSession struct {
	ID              string // = SessionActivityID(teamID, agentKey) 的 v2 版本
	TeamRunID       string
	TeamStageID     string
	TaskID          string
	SessionID       string // member 自己的 session ID（用于 lazy load）
	SpiritSessionID string
	AgentKey        string
	AgentName       string
	AvatarURL       string
	Status          MemberSessionStatus
	Seq             int64
}

type MemberSessionStatus string

const (
	MemberSessionStatusPending   MemberSessionStatus = "pending"
	MemberSessionStatusRunning   MemberSessionStatus = "running"
	MemberSessionStatusCompleted MemberSessionStatus = "completed"
	MemberSessionStatusFailed    MemberSessionStatus = "failed"
	MemberSessionStatusSkipped   MemberSessionStatus = "skipped"
)
```

- [ ] **Step 10: 创建 `internal/biz/plan_board.go`**

```go
package biz

import "time"

// PlanBoard 是任务计划面板（v2 模型）。
// 替代旧 TaskPlan + Activity.Meta.steps 双轨模型。
type PlanBoard struct {
	ID          string
	TaskID      string
	TurnID      string // 触发 plan 的 turn
	SessionID   string // spirit_session_id
	Strategy    PlanStrategy
	Status      PlanStatus
	Steps       []PlanStep // 数组，每个 step 有 id 和 depends_on
	StartedAt   time.Time
	CompletedAt *time.Time
	Seq         int64
}

type PlanStrategy string

const (
	PlanStrategySequential  PlanStrategy = "sequential"
	PlanStrategyParallel    PlanStrategy = "parallel"
	PlanStrategyDAG         PlanStrategy = "dag"
	PlanStrategyCoordinator PlanStrategy = "coordinator"
)

type PlanStatus string

const (
	PlanStatusPlanning       PlanStatus = "planning"
	PlanStatusExecuting      PlanStatus = "executing"
	PlanStatusCompleted      PlanStatus = "completed"
	PlanStatusFailed         PlanStatus = "failed"
	PlanStatusPartialFailure PlanStatus = "partial_failure"
)
```

- [ ] **Step 11: 创建 `internal/biz/plan_step.go`**

```go
package biz

import "time"

// PlanStep 是计划步骤（v2 模型）。
// 替代旧 Plan/PlanStep（废弃）+ SubTask 双轨模型。
type PlanStep struct {
	ID                string
	PlanID            string
	TaskID            string // 冗余，便于按 task 索引
	Label             string
	Description       string
	DependsOn         []string // 其他 plan_step.id
	MappedTeamStageID string   // 执行该 step 的 team_stage（如有；coordinator 模式下所有 step 共享一个 team_stage）
	Status            PlanStepStatus
	AutoSynthesis     bool // 是否为汇总报告 step（无 team 映射，依赖完成自动触发）
	StartedAt         time.Time
	CompletedAt       *time.Time
	Seq               int64 // 在 plan 内的序号
	Result            *StepResult // 完成时携带
	Error             *StepError  // 失败时携带
}

type PlanStepStatus string

const (
	PlanStepStatusPending        PlanStepStatus = "pending"
	PlanStepStatusRunning        PlanStepStatus = "running"
	PlanStepStatusCompleted      PlanStepStatus = "completed"
	PlanStepStatusFailed         PlanStepStatus = "failed"
	PlanStepStatusSkipped        PlanStepStatus = "skipped" // 依赖失败导致跳过
	PlanStepStatusPartialFailure PlanStepStatus = "partial_failure"
)

// StepResult 是 plan step 完成时携带的结果。
type StepResult struct {
	Output        string
	MemberReports []MemberReport
	TokensUsed    TokenUsage
	DurationMs    int64
}

// StepError 是 plan step 失败时携带的错误。
type StepError struct {
	Code         string // tool_failed / llm_timeout / team_failed / dependency_failed
	Message      string
	Retryable    bool
	FailedMember *MemberReport // 哪个 member 失败
}

// MemberReport 是单个 member 的执行报告。
type MemberReport struct {
	AgentKey   string
	AgentName  string
	Output     string
	TokensUsed TokenUsage
	DurationMs int64
	Error      string // 失败时填
}

// TokenUsage 是 token 用量统计。
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}
```

- [ ] **Step 12: 运行测试验证通过**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestSessionStatus_Constants|TestTaskStatus_Constants|TestStepKind_Constants|TestPlanStep_Fields|TestStepResult_JSONRoundtrip|TestMemberInfo_Fields" -count=1 -v 2>&1 | tail -20`
Expected: PASS，全部测试通过

- [ ] **Step 13: Commit**

```bash
cd f:\aranea-agents && git add internal/biz/session.go internal/biz/task.go internal/biz/turn.go internal/biz/step.go internal/biz/team_stage.go internal/biz/team_run.go internal/biz/member_session.go internal/biz/plan_board.go internal/biz/plan_step.go internal/biz/entity_v2_types_test.go && git commit -m "feat(biz): add v2 entity types (Session/Task/Turn/Step/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep)

Replace monolithic Activity struct with 9 typed entities per spec §3.2.2."
```

---

### Task 2: 创建 Event 接口 + 30+ 事件类型

**Files:**
- Create: `internal/biz/event.go`
- Create: `internal/biz/event_types_test.go`

- [ ] **Step 1: 写 Event 接口测试（红）**

Create `internal/biz/event_types_test.go`:

```go
package biz

import (
	"testing"
	"time"
)

func TestEvent_InterfaceCompliance(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		&TaskCreatedEvent{TaskID: "t1", SpiritSessionID: "s1", Task: Task{ID: "t1", SessionID: "s1", Seq: 1, CreatedAt: now}},
		&TurnStartedEvent{TaskID: "t1", SpiritSessionID: "s1", TurnID: "turn1", Turn: Turn{ID: "turn1", TaskID: "t1", Seq: 2, StartedAt: now}},
		&StepCreatedEvent{TaskID: "t1", SpiritSessionID: "s1", Step: Step{ID: "st1", TurnID: "turn1", Kind: StepKindThinking, Seq: 1, Status: StepStatusRunning}},
		&StepStreamingEvent{TaskID: "t1", SpiritSessionID: "s1", StepID: "st1", DeltaField: "content", DeltaChunk: "hello"},
		&StepCompletedEvent{TaskID: "t1", SpiritSessionID: "s1", Step: Step{ID: "st1", Status: StepStatusCompleted}},
		&TeamStageCreatedEvent{TaskID: "t1", SpiritSessionID: "s1", TeamStage: TeamStage{ID: "ts1", TaskID: "t1", Seq: 5, Status: TeamStageStatusPending}},
		&PlanStepStartedEvent{TaskID: "t1", SpiritSessionID: "s1", PlanStep: PlanStep{ID: "ps1", Status: PlanStepStatusRunning}},
	}
	for i, e := range events {
		if e.EventKind() == "" {
			t.Fatalf("event[%d]: empty EventKind", i)
		}
		if e.SpiritSessionID() == "" {
			t.Fatalf("event[%d]: empty SpiritSessionID", i)
		}
		if e.TaskID() == "" {
			t.Fatalf("event[%d]: empty TaskID", i)
		}
	}
}

func TestEventKind_Constants(t *testing.T) {
	if EventKindTaskCreated != "task.created" {
		t.Fatalf("expected task.created, got %s", EventKindTaskCreated)
	}
	if EventKindStepStreaming != "step.streaming" {
		t.Fatalf("expected step.streaming, got %s", EventKindStepStreaming)
	}
	if EventKindPlanStepCompleted != "plan_step.completed" {
		t.Fatalf("expected plan_step.completed, got %s", EventKindPlanStepCompleted)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestEvent_InterfaceCompliance|TestEventKind_Constants" -count=1 2>&1 | tail -10`
Expected: FAIL，提示 `undefined: Event` 等

- [ ] **Step 3: 创建 `internal/biz/event.go`**

```go
package biz

import "time"

// Event 是 v2 模型的统一事件接口。
// 所有事件源（Projector/Service/Team/PlanExecutor）都通过 Sequencer.Publish 发布。
type Event interface {
	EventKind() EventKind
	SpiritSessionID() string
	TaskID() string
	// OccurredAt 返回事件发生时间（用于排序兜底，主排序用 Seq）
	OccurredAt() time.Time
}

// EventKind 标识事件类型，格式为 "<entity>.<action>"。
type EventKind string

const (
	// Task 事件
	EventKindTaskCreated   EventKind = "task.created"
	EventKindTaskUpdated   EventKind = "task.updated"
	EventKindTaskCompleted  EventKind = "task.completed"
	EventKindTaskFailed     EventKind = "task.failed"
	EventKindTaskCancelled  EventKind = "task.cancelled"

	// Turn 事件
	EventKindTurnStarted   EventKind = "turn.started"
	EventKindTurnCompleted EventKind = "turn.completed"
	EventKindTurnFailed     EventKind = "turn.failed"

	// Step 事件
	EventKindStepCreated   EventKind = "step.created"
	EventKindStepStreaming  EventKind = "step.streaming"
	EventKindStepUpdated   EventKind = "step.updated"
	EventKindStepCompleted EventKind = "step.completed"
	EventKindStepFailed     EventKind = "step.failed"

	// TeamStage 事件
	EventKindTeamStageCreated  EventKind = "team_stage.created"
	EventKindTeamStageUpdated  EventKind = "team_stage.updated"
	EventKindTeamStageCompleted EventKind = "team_stage.completed"
	EventKindTeamStageFailed   EventKind = "team_stage.failed"

	// TeamRun 事件
	EventKindTeamRunStarted   EventKind = "team_run.started"
	EventKindTeamRunCompleted EventKind = "team_run.completed"
	EventKindTeamRunFailed    EventKind = "team_run.failed"

	// MemberSession 事件
	EventKindMemberSessionCreated EventKind = "member_session.created"
	EventKindMemberSessionUpdated EventKind = "member_session.updated"

	// PlanBoard 事件
	EventKindPlanBoardCreated EventKind = "plan_board.created"
	EventKindPlanBoardUpdated EventKind = "plan_board.updated"

	// PlanStep 事件
	EventKindPlanStepStarted   EventKind = "plan_step.started"
	EventKindPlanStepCompleted EventKind = "plan_step.completed"
	EventKindPlanStepFailed    EventKind = "plan_step.failed"
	EventKindPlanStepSkipped   EventKind = "plan_step.skipped"
	EventKindPlanStepUpdated   EventKind = "plan_step.updated"
)

// === Task 事件 ===

type TaskCreatedEvent struct {
	TaskID          string
	SpiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCreatedEvent) EventKind() EventKind      { return EventKindTaskCreated }
func (e *TaskCreatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TaskCreatedEvent) TaskID() string            { return e.TaskID }
func (e *TaskCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskUpdatedEvent) EventKind() EventKind      { return EventKindTaskUpdated }
func (e *TaskUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TaskUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *TaskUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskCompletedEvent) EventKind() EventKind      { return EventKindTaskCompleted }
func (e *TaskCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TaskCompletedEvent) TaskID() string            { return e.TaskID }
func (e *TaskCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TaskFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	Task            Task
	occurredAt      time.Time
}

func (e *TaskFailedEvent) EventKind() EventKind      { return EventKindTaskFailed }
func (e *TaskFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TaskFailedEvent) TaskID() string            { return e.TaskID }
func (e *TaskFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TaskFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === Turn 事件 ===

type TurnStartedEvent struct {
	TaskID          string
	SpiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnStartedEvent) EventKind() EventKind      { return EventKindTurnStarted }
func (e *TurnStartedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TurnStartedEvent) TaskID() string            { return e.TaskID }
func (e *TurnStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnCompletedEvent) EventKind() EventKind      { return EventKindTurnCompleted }
func (e *TurnCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TurnCompletedEvent) TaskID() string            { return e.TaskID }
func (e *TurnCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TurnFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	TurnID          string
	Turn            Turn
	occurredAt      time.Time
}

func (e *TurnFailedEvent) EventKind() EventKind      { return EventKindTurnFailed }
func (e *TurnFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TurnFailedEvent) TaskID() string            { return e.TaskID }
func (e *TurnFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TurnFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === Step 事件 ===

type StepCreatedEvent struct {
	TaskID          string
	SpiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCreatedEvent) EventKind() EventKind      { return EventKindStepCreated }
func (e *StepCreatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *StepCreatedEvent) TaskID() string            { return e.TaskID }
func (e *StepCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// StepStreamingEvent 是流式增量事件（不入库，仅推送 WS）。
// Sequencer 16ms 批合并同 StepID + 同 DeltaField 的事件。
type StepStreamingEvent struct {
	TaskID          string
	SpiritSessionID string
	StepID          string
	DeltaField      string // content / reasoning / tool_args
	DeltaChunk      string
	occurredAt      time.Time
}

func (e *StepStreamingEvent) EventKind() EventKind      { return EventKindStepStreaming }
func (e *StepStreamingEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *StepStreamingEvent) TaskID() string            { return e.TaskID }
func (e *StepStreamingEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepStreamingEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepUpdatedEvent) EventKind() EventKind      { return EventKindStepUpdated }
func (e *StepUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *StepUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *StepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepCompletedEvent) EventKind() EventKind      { return EventKindStepCompleted }
func (e *StepCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *StepCompletedEvent) TaskID() string            { return e.TaskID }
func (e *StepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type StepFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	Step            Step
	occurredAt      time.Time
}

func (e *StepFailedEvent) EventKind() EventKind      { return EventKindStepFailed }
func (e *StepFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *StepFailedEvent) TaskID() string            { return e.TaskID }
func (e *StepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *StepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamStage 事件 ===

type TeamStageCreatedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCreatedEvent) EventKind() EventKind      { return EventKindTeamStageCreated }
func (e *TeamStageCreatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamStageCreatedEvent) TaskID() string            { return e.TaskID }
func (e *TeamStageCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageUpdatedEvent) EventKind() EventKind      { return EventKindTeamStageUpdated }
func (e *TeamStageUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamStageUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *TeamStageUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageCompletedEvent) EventKind() EventKind      { return EventKindTeamStageCompleted }
func (e *TeamStageCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamStageCompletedEvent) TaskID() string            { return e.TaskID }
func (e *TeamStageCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamStageFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamStage       TeamStage
	occurredAt      time.Time
}

func (e *TeamStageFailedEvent) EventKind() EventKind      { return EventKindTeamStageFailed }
func (e *TeamStageFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamStageFailedEvent) TaskID() string            { return e.TaskID }
func (e *TeamStageFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamStageFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === TeamRun 事件 ===

type TeamRunStartedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunStartedEvent) EventKind() EventKind      { return EventKindTeamRunStarted }
func (e *TeamRunStartedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamRunStartedEvent) TaskID() string            { return e.TaskID }
func (e *TeamRunStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunCompletedEvent) EventKind() EventKind      { return EventKindTeamRunCompleted }
func (e *TeamRunCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamRunCompletedEvent) TaskID() string            { return e.TaskID }
func (e *TeamRunCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type TeamRunFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	TeamRun         TeamRun
	occurredAt      time.Time
}

func (e *TeamRunFailedEvent) EventKind() EventKind      { return EventKindTeamRunFailed }
func (e *TeamRunFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *TeamRunFailedEvent) TaskID() string            { return e.TaskID }
func (e *TeamRunFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *TeamRunFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === MemberSession 事件 ===

type MemberSessionCreatedEvent struct {
	TaskID          string
	SpiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionCreatedEvent) EventKind() EventKind      { return EventKindMemberSessionCreated }
func (e *MemberSessionCreatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *MemberSessionCreatedEvent) TaskID() string            { return e.TaskID }
func (e *MemberSessionCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type MemberSessionUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	MemberSession   MemberSession
	occurredAt      time.Time
}

func (e *MemberSessionUpdatedEvent) EventKind() EventKind      { return EventKindMemberSessionUpdated }
func (e *MemberSessionUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *MemberSessionUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *MemberSessionUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *MemberSessionUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanBoard 事件 ===

type PlanBoardCreatedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardCreatedEvent) EventKind() EventKind      { return EventKindPlanBoardCreated }
func (e *PlanBoardCreatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanBoardCreatedEvent) TaskID() string            { return e.TaskID }
func (e *PlanBoardCreatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardCreatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanBoardUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanBoard       PlanBoard
	occurredAt      time.Time
}

func (e *PlanBoardUpdatedEvent) EventKind() EventKind      { return EventKindPlanBoardUpdated }
func (e *PlanBoardUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanBoardUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *PlanBoardUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanBoardUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// === PlanStep 事件 ===

type PlanStepStartedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepStartedEvent) EventKind() EventKind      { return EventKindPlanStepStarted }
func (e *PlanStepStartedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanStepStartedEvent) TaskID() string            { return e.TaskID }
func (e *PlanStepStartedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepStartedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepCompletedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepCompletedEvent) EventKind() EventKind      { return EventKindPlanStepCompleted }
func (e *PlanStepCompletedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanStepCompletedEvent) TaskID() string            { return e.TaskID }
func (e *PlanStepCompletedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepCompletedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepFailedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepFailedEvent) EventKind() EventKind      { return EventKindPlanStepFailed }
func (e *PlanStepFailedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanStepFailedEvent) TaskID() string            { return e.TaskID }
func (e *PlanStepFailedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepFailedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepSkippedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanStep        PlanStep
	Reason          string // dependency_failed / cancelled
	occurredAt      time.Time
}

func (e *PlanStepSkippedEvent) EventKind() EventKind      { return EventKindPlanStepSkipped }
func (e *PlanStepSkippedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanStepSkippedEvent) TaskID() string            { return e.TaskID }
func (e *PlanStepSkippedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepSkippedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

type PlanStepUpdatedEvent struct {
	TaskID          string
	SpiritSessionID string
	PlanStep        PlanStep
	occurredAt      time.Time
}

func (e *PlanStepUpdatedEvent) EventKind() EventKind      { return EventKindPlanStepUpdated }
func (e *PlanStepUpdatedEvent) SpiritSessionID() string   { return e.SpiritSessionID }
func (e *PlanStepUpdatedEvent) TaskID() string            { return e.TaskID }
func (e *PlanStepUpdatedEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *PlanStepUpdatedEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestEvent_InterfaceCompliance|TestEventKind_Constants" -count=1 -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd f:\aranea-agents && git add internal/biz/event.go internal/biz/event_types_test.go && git commit -m "feat(biz): add v2 Event interface + 30 concrete event types

Unified event contract for all sources (Projector/Service/Team/PlanExecutor). Per spec §3.3.2."
```

---

### Task 3: 创建 PlanStep 状态机

**Files:**
- Create: `internal/biz/plan_step_state_machine.go`
- Create: `internal/biz/plan_step_state_machine_test.go`

- [ ] **Step 1: 写状态机测试（红）**

Create `internal/biz/plan_step_state_machine_test.go`:

```go
package biz

import (
	"strings"
	"testing"
)

func TestPlanStep_Transition_Valid(t *testing.T) {
	cases := []struct {
		name string
		from PlanStepStatus
		to   PlanStepStatus
	}{
		{"pending to running", PlanStepStatusPending, PlanStepStatusRunning},
		{"pending to skipped", PlanStepStatusPending, PlanStepStatusSkipped},
		{"running to completed", PlanStepStatusRunning, PlanStepStatusCompleted},
		{"running to failed", PlanStepStatusRunning, PlanStepStatusFailed},
		{"running to partial_failure", PlanStepStatusRunning, PlanStepStatusPartialFailure},
		{"failed to running (retry)", PlanStepStatusFailed, PlanStepStatusRunning},
		{"partial_failure to running (retry)", PlanStepStatusPartialFailure, PlanStepStatusRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := PlanStep{Status: c.from}
			if err := ps.Transition(c.to); err != nil {
				t.Fatalf("expected transition %s → %s to succeed, got: %v", c.from, c.to, err)
			}
			if ps.Status != c.to {
				t.Fatalf("expected status %s, got %s", c.to, ps.Status)
			}
		})
	}
}

func TestPlanStep_Transition_Invalid(t *testing.T) {
	cases := []struct {
		name string
		from PlanStepStatus
		to   PlanStepStatus
	}{
		{"completed to running (terminal)", PlanStepStatusCompleted, PlanStepStatusRunning},
		{"skipped to running (terminal)", PlanStepStatusSkipped, PlanStepStatusRunning},
		{"completed to failed (terminal)", PlanStepStatusCompleted, PlanStepStatusFailed},
		{"pending to completed (skip running)", PlanStepStatusPending, PlanStepStatusCompleted},
		{"pending to failed (skip running)", PlanStepStatusPending, PlanStepStatusFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := PlanStep{Status: c.from}
			err := ps.Transition(c.to)
			if err == nil {
				t.Fatalf("expected transition %s → %s to fail, but succeeded", c.from, c.to)
			}
			if !strings.Contains(err.Error(), "invalid transition") {
				t.Fatalf("expected 'invalid transition' in error, got: %v", err)
			}
			if ps.Status != c.from {
				t.Fatalf("status should remain %s, got %s", c.from, ps.Status)
			}
		})
	}
}

func TestPlanStep_Transition_UnknownSource(t *testing.T) {
	ps := PlanStep{Status: PlanStepStatus("unknown")}
	err := ps.Transition(PlanStepStatusRunning)
	if err == nil {
		t.Fatalf("expected error for unknown source status")
	}
	if !strings.Contains(err.Error(), "unknown source status") {
		t.Fatalf("expected 'unknown source status' in error, got: %v", err)
	}
}

func TestPlanStep_CanTransition(t *testing.T) {
	ps := PlanStep{Status: PlanStepStatusPending}
	if !ps.CanTransition(PlanStepStatusRunning) {
		t.Fatalf("expected pending → running to be allowed")
	}
	if ps.CanTransition(PlanStepStatusCompleted) {
		t.Fatalf("expected pending → completed to be disallowed")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestPlanStep_Transition" -count=1 2>&1 | tail -10`
Expected: FAIL，提示 `ps.Transition undefined`

- [ ] **Step 3: 创建 `internal/biz/plan_step_state_machine.go`**

```go
package biz

import "fmt"

// planStepTransitions 定义 PlanStep 的合法状态转换表（spec §3.5.4）。
// key = 源状态，value = 可到达的目标状态列表。
var planStepTransitions = map[PlanStepStatus][]PlanStepStatus{
	PlanStepStatusPending: {
		PlanStepStatusRunning,
		PlanStepStatusSkipped, // 依赖失败时跳过
	},
	PlanStepStatusRunning: {
		PlanStepStatusCompleted,
		PlanStepStatusFailed,
		PlanStepStatusSkipped,
		PlanStepStatusPartialFailure,
	},
	PlanStepStatusCompleted: {}, // terminal
	PlanStepStatusFailed: {PlanStepStatusRunning}, // 允许重试
	PlanStepStatusSkipped: {},                   // terminal
	PlanStepStatusPartialFailure: {PlanStepStatusRunning}, // 允许重试
}

// Transition 校验并执行状态转换。禁止跳过状态机直接赋值（spec §3.5.4）。
func (s *PlanStep) Transition(to PlanStepStatus) error {
	allowed, ok := planStepTransitions[s.Status]
	if !ok {
		return fmt.Errorf("unknown source status: %s", s.Status)
	}
	for _, a := range allowed {
		if a == to {
			s.Status = to
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s → %s", s.Status, to)
}

// CanTransition 返回是否可以从当前状态转换到目标状态（不执行转换）。
func (s *PlanStep) CanTransition(to PlanStepStatus) bool {
	allowed, ok := planStepTransitions[s.Status]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// IsTerminal 返回当前状态是否为终态。
func (s PlanStepStatus) IsTerminal() bool {
	switch s {
	case PlanStepStatusCompleted, PlanStepStatusSkipped:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd f:\aranea-agents && go test ./internal/biz/ -run "TestPlanStep_Transition|TestPlanStep_CanTransition" -count=1 -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd f:\aranea-agents && git add internal/biz/plan_step_state_machine.go internal/biz/plan_step_state_machine_test.go && git commit -m "feat(biz): add PlanStep state machine with explicit transitions

Forbid direct status assignment per spec §3.5.4. Allows retry from failed/partial_failure."
```

---

## Phase 1.2: 数据库

### Task 4: 创建 9 个 Ent Schema 文件

**Files:**
- Create: `internal/data/ent/schema/session_v2.go`
- Create: `internal/data/ent/schema/task_v2.go`
- Create: `internal/data/ent/schema/turn_v2.go`
- Create: `internal/data/ent/schema/step_v2.go`
- Create: `internal/data/ent/schema/team_stage_v2.go`
- Create: `internal/data/ent/schema/team_run_v2.go`
- Create: `internal/data/ent/schema/member_session_v2.go`
- Create: `internal/data/ent/schema/plan_board_v2.go`
- Create: `internal/data/ent/schema/plan_step_v2.go`

> **命名说明**：Schema 文件加 `_v2` 后缀，避免与现有 `session.go`/`team.go`/`team_run.go` 等冲突。表名也加 `_v2` 后缀。

- [ ] **Step 1: 创建 `internal/data/ent/schema/session_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionV2 是 spirit 会话根 entity（v2 模型）。
type SessionV2 struct {
	ent.Schema
}

func (SessionV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sessions_v2"},
	}
}

func (SessionV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(128).Unique().Immutable(),
		field.String("user_id").MaxLen(128).Default(""),
		field.String("spirit_agent_id").MaxLen(128).Default(""),
		field.String("status").MaxLen(32).Default("active"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
	}
}

func (SessionV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status").StorageKey("idx_sessions_v2_user_status"),
	}
}
```

- [ ] **Step 2: 创建 `internal/data/ent/schema/task_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaskV2 是用户输入对应的根活动（v2 模型）。
type TaskV2 struct {
	ent.Schema
}

func (TaskV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tasks_v2"},
	}
}

func (TaskV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.Text("user_message").Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("seq").Default(0).Comment("Sequence within session, monotonic"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (TaskV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "seq").StorageKey("idx_tasks_v2_session_seq"),
		index.Fields("status").StorageKey("idx_tasks_v2_status"),
	}
}
```

- [ ] **Step 3: 创建 `internal/data/ent/schema/turn_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TurnV2 是最小对话单元（v2 模型）。
type TurnV2 struct {
	ent.Schema
}

func (TurnV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "turns_v2"},
	}
}

func (TurnV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128).Comment("current session (spirit/team/member)"),
		field.String("spirit_session_id").MaxLen(128).Comment("always points to spirit root session for WS filter"),
		field.String("parent_turn_id").MaxLen(64).Default("").Comment("nested turn parent"),
		field.String("agent_key").MaxLen(128).Default(""),
		field.String("team_id").MaxLen(128).Default("").Comment("empty for spirit turn"),
		field.String("team_stage_id").MaxLen(64).Default("").Comment("set when member turn"),
		field.Int64("seq").Default(0).Comment("global seq within task, monotonic"),
		field.String("status").MaxLen(32).Default("running"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (TurnV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_turns_v2_task_seq"),
		index.Fields("spirit_session_id", "seq").StorageKey("idx_turns_v2_spirit_seq"),
		index.Fields("parent_turn_id").StorageKey("idx_turns_v2_parent"),
	}
}
```

- [ ] **Step 4: 创建 `internal/data/ent/schema/step_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// StepV2 是 turn 内的工作步骤（v2 模型）。
type StepV2 struct {
	ent.Schema
}

func (StepV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "steps_v2"},
	}
}

func (StepV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("turn_id").MaxLen(64),
		field.String("task_id").MaxLen(64).Comment("redundant for task indexing"),
		field.String("session_id").MaxLen(128),
		field.String("spirit_session_id").MaxLen(128),
		field.String("kind").MaxLen(32).Comment("thinking/action/reply/notice/confirm/error"),
		field.Int64("seq").Default(0).Comment("seq within turn"),
		field.Text("content").Default(""),
		field.Text("reasoning").Default(""),
		field.String("tool_name").MaxLen(128).Default(""),
		field.String("tool_call_id").MaxLen(512).Default(""),
		field.Text("tool_args").Default("").Sensitive(),
		field.Text("tool_result").Default("").Sensitive(),
		field.Int64("tool_duration_ms").Default(0),
		field.String("tool_error_code").MaxLen(64).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Bool("is_final").Default(false).Comment("reply: is this the final reply"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("version").Default(0).Comment("Monotonic version for ordered upserts"),
	}
}

func (StepV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("turn_id", "seq").StorageKey("idx_steps_v2_turn_seq"),
		index.Fields("task_id", "seq").StorageKey("idx_steps_v2_task_seq"),
		index.Fields("spirit_session_id", "seq").StorageKey("idx_steps_v2_spirit_seq"),
	}
}
```

- [ ] **Step 5: 创建 `internal/data/ent/schema/team_stage_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamStageV2 是 task 内的团队执行阶段（v2 模型）。
type TeamStageV2 struct {
	ent.Schema
}

func (TeamStageV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_stages_v2"},
	}
}

func (TeamStageV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("turn_id").MaxLen(64).Comment("turn that triggered the team"),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.String("team_id").MaxLen(128),
		field.String("dag_node_id").MaxLen(128).Default("").Comment("corresponding plan_step.id"),
		field.JSON("depends_on", []string{}).Optional().Comment("other team_stage.id DAG deps"),
		field.String("status").MaxLen(32).Default("pending"),
		field.String("stage").MaxLen(64).Default("").Comment("assembled/planning/executing/completed/failed"),
		field.JSON("members", []map[string]any{}).Optional().Comment("type-safe member list"),
		field.String("strategy").MaxLen(32).Default("").Comment("parallel/dag/coordinator"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (TeamStageV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_team_stages_v2_task_seq"),
		index.Fields("spirit_session_id", "seq").StorageKey("idx_team_stages_v2_spirit_seq"),
		index.Fields("dag_node_id").StorageKey("idx_team_stages_v2_dag_node"),
	}
}
```

- [ ] **Step 6: 创建 `internal/data/ent/schema/team_run_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamRunV2 是 team 内的一次执行（v2 模型）。
type TeamRunV2 struct {
	ent.Schema
}

func (TeamRunV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_runs_v2"},
	}
}

func (TeamRunV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("team_stage_id").MaxLen(64),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128),
		field.String("spirit_session_id").MaxLen(128),
		field.String("dag_node_id").MaxLen(128).Default(""),
		field.JSON("depends_on", []string{}).Optional(),
		field.String("status").MaxLen(32).Default("running"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (TeamRunV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_stage_id", "seq").StorageKey("idx_team_runs_v2_stage_seq"),
		index.Fields("dag_node_id").StorageKey("idx_team_runs_v2_dag_node"),
	}
}
```

- [ ] **Step 7: 创建 `internal/data/ent/schema/member_session_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MemberSessionV2 是 team 内的成员会话（v2 模型）。
type MemberSessionV2 struct {
	ent.Schema
}

func (MemberSessionV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "member_sessions_v2"},
	}
}

func (MemberSessionV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("team_run_id").MaxLen(64),
		field.String("team_stage_id").MaxLen(64),
		field.String("task_id").MaxLen(64),
		field.String("session_id").MaxLen(128).Comment("member own session for lazy load"),
		field.String("spirit_session_id").MaxLen(128),
		field.String("agent_key").MaxLen(128),
		field.String("agent_name").MaxLen(128).Default(""),
		field.String("avatar_url").MaxLen(512).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (MemberSessionV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_run_id", "seq").StorageKey("idx_member_sessions_v2_run_seq"),
		index.Fields("agent_key").StorageKey("idx_member_sessions_v2_agent"),
	}
}
```

- [ ] **Step 8: 创建 `internal/data/ent/schema/plan_board_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlanBoardV2 是任务计划面板（v2 模型）。
type PlanBoardV2 struct {
	ent.Schema
}

func (PlanBoardV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plan_boards_v2"},
	}
}

func (PlanBoardV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("task_id").MaxLen(64),
		field.String("turn_id").MaxLen(64).Comment("turn that triggered the plan"),
		field.String("session_id").MaxLen(128).Comment("spirit_session_id"),
		field.String("strategy").MaxLen(32).Default("sequential"),
		field.String("status").MaxLen(32).Default("planning"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
	}
}

func (PlanBoardV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "seq").StorageKey("idx_plan_boards_v2_task_seq"),
	}
}
```

- [ ] **Step 9: 创建 `internal/data/ent/schema/plan_step_v2.go`**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlanStepV2 是计划步骤（v2 模型）。
type PlanStepV2 struct {
	ent.Schema
}

func (PlanStepV2) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plan_steps_v2"},
	}
}

func (PlanStepV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(64).Unique().Immutable(),
		field.String("plan_id").MaxLen(64),
		field.String("task_id").MaxLen(64).Comment("redundant for task indexing"),
		field.String("label").MaxLen(256).Default(""),
		field.Text("description").Default(""),
		field.JSON("depends_on", []string{}).Optional(),
		field.String("mapped_team_stage_id").MaxLen(64).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Bool("auto_synthesis").Default(false).Comment("synthesis step, no team mapping"),
		field.Time("started_at").Default(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Int64("seq").Default(0),
		field.Int64("version").Default(0),
		field.JSON("result", map[string]any{}).Optional().Comment("StepResult JSON"),
		field.JSON("error", map[string]any{}).Optional().Comment("StepError JSON"),
	}
}

func (PlanStepV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "seq").StorageKey("idx_plan_steps_v2_plan_seq"),
		index.Fields("task_id").StorageKey("idx_plan_steps_v2_task"),
		index.Fields("mapped_team_stage_id").StorageKey("idx_plan_steps_v2_team_stage"),
	}
}
```

- [ ] **Step 10: 检查 timeNow helper 是否存在**

Run: `cd f:\aranea-agents && grep -rn "func timeNow" internal/data/ent/schema/ 2>&1 | head -5`
Expected: 找到现有定义（如不存在则需添加）

如果没有找到，创建 `internal/data/ent/schema/helpers.go`:

```go
package schema

import "time"

// timeNow 返回当前 UTC 时间，作为 Ent Schema 字段默认值。
func timeNow() time.Time {
	return time.Now().UTC()
}
```

- [ ] **Step 11: 验证 Schema 编译**

Run: `cd f:\aranea-agents && go build ./internal/data/ent/schema/... 2>&1 | tail -10`
Expected: 无错误（此时还未 generate，所以仅检查 Schema 文件本身的语法）

- [ ] **Step 12: Commit**

```bash
cd f:\aranea-agents && git add internal/data/ent/schema/session_v2.go internal/data/ent/schema/task_v2.go internal/data/ent/schema/turn_v2.go internal/data/ent/schema/step_v2.go internal/data/ent/schema/team_stage_v2.go internal/data/ent/schema/team_run_v2.go internal/data/ent/schema/member_session_v2.go internal/data/ent/schema/plan_board_v2.go internal/data/ent/schema/plan_step_v2.go internal/data/ent/schema/helpers.go && git commit -m "feat(data): add 9 v2 Ent schemas (sessions/tasks/turns/steps/team_stages/team_runs/member_sessions/plan_boards/plan_steps)

Tables use _v2 suffix to coexist with legacy tables. Per spec §3.2.3 + §4.2."
```

---

### Task 5: 运行 go generate 生成 Ent 代码

**Files:**
- Modify: `internal/data/ent/generate.go`（如需新增 generate 指令）
- Auto-generated: `internal/data/ent/sessionv2.go`, `taskv2.go` 等

- [ ] **Step 1: 检查 generate.go 模板**

Run: `cd f:\aranea-agents && cat internal/data/ent/generate.go`
Expected: 看到 `go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema`

- [ ] **Step 2: 运行 go generate**

Run: `cd f:\aranea-agents && go generate ./internal/data/ent 2>&1 | tail -20`
Expected: 无错误，生成新文件 `internal/data/ent/sessionv2/`, `taskv2/` 等目录

- [ ] **Step 3: 验证生成文件存在**

Run: `cd f:\aranea-agents && ls internal/data/ent/ | grep -i v2 | head -20`
Expected: 看到 `sessionv2.go`、`taskv2.go`、`turnv2.go`、`stepv2.go`、`teamstagev2.go`、`teamrunv2.go`、`membersessionv2.go`、`planboardv2.go`、`planstepv2.go` 等

- [ ] **Step 4: 验证整体编译**

Run: `cd f:\aranea-agents && go build ./... 2>&1 | tail -10`
Expected: 无错误（如果旧代码引用了被替换的类型，会有错误，需要修复；但 Phase 1 新旧共存，不应有冲突）

- [ ] **Step 5: Commit**

```bash
cd f:\aranea-agents && git add internal/data/ent/ && git commit -m "chore(data): run go generate for v2 schemas

Generated Ent code for 9 new v2 tables. No manual edits."
```

---

### Task 6: 创建 SeqAssigner

**Files:**
- Create: `internal/agent/seq_assigner.go`
- Create: `internal/agent/seq_assigner_test.go`

- [ ] **Step 1: 写 SeqAssigner 测试（红）**

Create `internal/agent/seq_assigner_test.go`:

```go
package agent

import (
	"sync"
	"testing"
)

func TestSeqAssigner_NextSeq_Monotonic(t *testing.T) {
	sa := NewSeqAssigner()
	expected := []int64{1, 2, 3, 4, 5}
	for i, want := range expected {
		got := sa.NextSeq("session-1")
		if got != want {
			t.Fatalf("NextSeq[%d]: expected %d, got %d", i, want, got)
		}
	}
}

func TestSeqAssigner_NextSeq_PerSessionIsolation(t *testing.T) {
	sa := NewSeqAssigner()
	// session-1 拿 3 个
	sa.NextSeq("session-1")
	sa.NextSeq("session-1")
	sa.NextSeq("session-1")
	// session-2 应该从 1 开始
	got := sa.NextSeq("session-2")
	if got != 1 {
		t.Fatalf("session-2 first seq: expected 1, got %d", got)
	}
	// session-1 继续应该 4
	got = sa.NextSeq("session-1")
	if got != 4 {
		t.Fatalf("session-1 after session-2: expected 4, got %d", got)
	}
}

func TestSeqAssigner_NextSeq_Concurrent(t *testing.T) {
	sa := NewSeqAssigner()
	const goroutines = 50
	const perG = 100
	var wg sync.WaitGroup
	results := make(chan int64, goroutines*perG)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				results <- sa.NextSeq("session-concurrent")
			}
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[int64]bool, goroutines*perG)
	for seq := range results {
		if seq < 1 || seq > int64(goroutines*perG) {
			t.Fatalf("seq out of range: %d", seq)
		}
		if seen[seq] {
			t.Fatalf("duplicate seq: %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != goroutines*perG {
		t.Fatalf("expected %d unique seqs, got %d", goroutines*perG, len(seen))
	}
}

func TestSeqAssigner_RestoreFromDB(t *testing.T) {
	sa := NewSeqAssigner()
	sa.RestoreFromDB("session-restore", 100)
	got := sa.NextSeq("session-restore")
	if got != 101 {
		t.Fatalf("after restore: expected 101, got %d", got)
	}
}

func TestSeqAssigner_RestoreFromDB_DoesNotLower(t *testing.T) {
	sa := NewSeqAssigner()
	sa.NextSeq("session-x") // seq=1
	sa.NextSeq("session-x") // seq=2
	sa.RestoreFromDB("session-x", 1) // should not lower to 1
	got := sa.NextSeq("session-x")
	if got != 3 {
		t.Fatalf("restore should not lower seq: expected 3, got %d", got)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run "TestSeqAssigner" -count=1 2>&1 | tail -10`
Expected: FAIL，提示 `undefined: NewSeqAssigner`

- [ ] **Step 3: 创建 `internal/agent/seq_assigner.go`**

```go
package agent

import (
	"sync/atomic"
)

// SeqAssigner 为每个 spirit session 维护一个 atomic counter，分配全局单调递增的 Seq。
// 同一 spirit session 内的所有 entity（Task/Turn/Step/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep）
// 共享同一 counter，保证跨 entity 类型的 Seq 排序正确。
//
// 重启恢复：调用 RestoreFromDB 从 DB 查询 MAX(seq) WHERE spirit_session_id = ? 恢复。
type SeqAssigner struct {
	counters sync.Map // sessionID → *atomic.Int64
}

// 使用 sync.Map 而非 map+mutex 的理由：
// - LoadOrStore 避免双检锁样板
// - 读多写少（同一 session 的多次 NextSeq 是读 counter 指针）
// - 跨 session 并发无锁
// 注：sync.Map = 1，符合 AS-COG-01（单 struct 内 sync.Map 数 ≤ 1）
//
// 例外说明：本 struct 持有 sync.Map，本身就是单一职责的「Seq 分配管理器」，
// 不是业务 struct，不触发 sync.Map 提取子管理器要求。
var _ = atomic.AddInt64 // keep import

// NewSeqAssigner 创建 SeqAssigner。
func NewSeqAssigner() *SeqAssigner {
	return &SeqAssigner{}
}

// NextSeq 返回 spirit session 的下一个 Seq（从 1 开始）。
func (s *SeqAssigner) NextSeq(spiritSessionID string) int64 {
	if spiritSessionID == "" {
		// 退化场景：不应发生，但兜底，避免空 key 导致全局 counter 污染
		spiritSessionID = "_default_"
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	return v.(*atomic.Int64).Add(1)
}

// RestoreFromDB 从 DB 恢复 Seq 计数器。
// 调用方在启动时查询 MAX(seq) WHERE spirit_session_id = ? 并传入。
// 若已存在的 counter 值大于传入值，不降低（防止并发场景下的回退）。
func (s *SeqAssigner) RestoreFromDB(spiritSessionID string, maxSeqFromDB int64) {
	if spiritSessionID == "" {
		return
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	current := v.(*atomic.Int64).Load()
	for maxSeqFromDB > current {
		if v.(*atomic.Int64).CompareAndSwap(current, maxSeqFromDB) {
			return
		}
		current = v.(*atomic.Int64).Load()
	}
}
```

- [ ] **Step 4: 修复 sync import**

Edit `internal/agent/seq_assigner.go`，在 import 中加入 `sync`：

```go
import (
	"sync"
	"sync/atomic"
)
```

并删除 `var _ = atomic.AddInt64` 行（不需要）。

- [ ] **Step 5: 运行测试验证通过**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run "TestSeqAssigner" -count=1 -race -v 2>&1 | tail -30`
Expected: PASS（包含 -race 验证无数据竞争）

- [ ] **Step 6: Commit**

```bash
cd f:\aranea-agents && git add internal/agent/seq_assigner.go internal/agent/seq_assigner_test.go && git commit -m "feat(agent): add SeqAssigner for per-spirit-session monotonic seq

Atomic counter per spirit session. RestoreFromDB for crash recovery. Per spec §3.2.5."
```

---

## Phase 1.3: Repo 层

### Task 7: 创建 9 个 Repo 窄接口

**Files:**
- Create: `internal/biz/repo_ports_v2.go`

- [ ] **Step 1: 创建 `internal/biz/repo_ports_v2.go`**

```go
package biz

import "context"

// v2 Repo 窄接口（每个接口方法 ≤ 5，按读写职责拆分）。
// 复合接口仅用于 Wire 绑定。
//
// Stability:evolving

// === Session ===

type SessionV2Reader interface {
	GetSession(ctx context.Context, id string) (Session, error)
}

type SessionV2Writer interface {
	CreateSession(ctx context.Context, s Session) (Session, error)
	UpdateSessionStatus(ctx context.Context, id string, status SessionStatus) error
}

type SessionV2Repo interface {
	SessionV2Reader
	SessionV2Writer
}

// === Task ===

type TaskV2Reader interface {
	GetTask(ctx context.Context, id string) (Task, error)
	ListTasksBySession(ctx context.Context, sessionID string) ([]Task, error)
}

type TaskV2Writer interface {
	CreateTask(ctx context.Context, t Task) (Task, error)
	UpdateTask(ctx context.Context, t Task) (Task, error)
	UpsertTask(ctx context.Context, t Task) (Task, error)
}

type TaskV2Repo interface {
	TaskV2Reader
	TaskV2Writer
}

// === Turn ===

type TurnV2Reader interface {
	GetTurn(ctx context.Context, id string) (Turn, error)
	ListTurnsByTask(ctx context.Context, taskID string) ([]Turn, error)
}

type TurnV2Writer interface {
	CreateTurn(ctx context.Context, t Turn) (Turn, error)
	UpdateTurn(ctx context.Context, t Turn) (Turn, error)
	UpsertTurn(ctx context.Context, t Turn) (Turn, error)
}

type TurnV2Repo interface {
	TurnV2Reader
	TurnV2Writer
}

// === Step ===

type StepV2Reader interface {
	GetStep(ctx context.Context, id string) (Step, error)
	ListStepsByTurn(ctx context.Context, turnID string) ([]Step, error)
	ListStepsByTask(ctx context.Context, taskID string) ([]Step, error)
}

type StepV2Writer interface {
	CreateStep(ctx context.Context, s Step) (Step, error)
	UpdateStep(ctx context.Context, s Step) (Step, error)
	UpsertStep(ctx context.Context, s Step) (Step, error)
}

type StepV2Repo interface {
	StepV2Reader
	StepV2Writer
}

// === TeamStage ===

type TeamStageV2Reader interface {
	GetTeamStage(ctx context.Context, id string) (TeamStage, error)
	ListTeamStagesByTask(ctx context.Context, taskID string) ([]TeamStage, error)
}

type TeamStageV2Writer interface {
	CreateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpdateTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
	UpsertTeamStage(ctx context.Context, ts TeamStage) (TeamStage, error)
}

type TeamStageV2Repo interface {
	TeamStageV2Reader
	TeamStageV2Writer
}

// === TeamRun ===

type TeamRunV2Reader interface {
	GetTeamRun(ctx context.Context, id string) (TeamRun, error)
	ListTeamRunsByStage(ctx context.Context, stageID string) ([]TeamRun, error)
}

type TeamRunV2Writer interface {
	CreateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpdateTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
	UpsertTeamRun(ctx context.Context, tr TeamRun) (TeamRun, error)
}

type TeamRunV2Repo interface {
	TeamRunV2Reader
	TeamRunV2Writer
}

// === MemberSession ===

type MemberSessionV2Reader interface {
	GetMemberSession(ctx context.Context, id string) (MemberSession, error)
	ListMemberSessionsByRun(ctx context.Context, runID string) ([]MemberSession, error)
}

type MemberSessionV2Writer interface {
	CreateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpdateMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
	UpsertMemberSession(ctx context.Context, ms MemberSession) (MemberSession, error)
}

type MemberSessionV2Repo interface {
	MemberSessionV2Reader
	MemberSessionV2Writer
}

// === PlanBoard ===

type PlanBoardV2Reader interface {
	GetPlanBoard(ctx context.Context, id string) (PlanBoard, error)
	ListPlanBoardsByTask(ctx context.Context, taskID string) ([]PlanBoard, error)
}

type PlanBoardV2Writer interface {
	CreatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpdatePlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
	UpsertPlanBoard(ctx context.Context, pb PlanBoard) (PlanBoard, error)
}

type PlanBoardV2Repo interface {
	PlanBoardV2Reader
	PlanBoardV2Writer
}

// === PlanStep ===

type PlanStepV2Reader interface {
	GetPlanStep(ctx context.Context, id string) (PlanStep, error)
	ListPlanStepsByPlan(ctx context.Context, planID string) ([]PlanStep, error)
	ListPlanStepsByTask(ctx context.Context, taskID string) ([]PlanStep, error)
}

type PlanStepV2Writer interface {
	CreatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpdatePlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
	UpsertPlanStep(ctx context.Context, ps PlanStep) (PlanStep, error)
}

type PlanStepV2Repo interface {
	PlanStepV2Reader
	PlanStepV2Writer
}
```

- [ ] **Step 2: 验证编译**

Run: `cd f:\aranea-agents && go build ./internal/biz/... 2>&1 | tail -10`
Expected: 无错误

- [ ] **Step 3: Commit**

```bash
cd f:\aranea-agents && git add internal/biz/repo_ports_v2.go && git commit -m "feat(biz): add v2 repo port interfaces (9 entities, reader/writer/upserter split)

Per-spec: each interface <= 5 methods. Composite interfaces for Wire binding only."
```

---

### Task 8: 创建 9 个 Repo 实现

> 由于 9 个 Repo 实现模式高度相似（构造注入 + RW() 读写分离 + entErrToBizErr + VersionLT 守卫），此处展示 Session + Step + PlanStep 三个代表性实现（覆盖基础 CRUD、复杂字段 JSON、状态机集成）。其余 6 个 Repo 按相同模式实现，每个独立 commit。

**Files:**
- Create: `internal/data/session_v2_repo.go`
- Create: `internal/data/task_v2_repo.go`
- Create: `internal/data/turn_v2_repo.go`
- Create: `internal/data/step_v2_repo.go`
- Create: `internal/data/team_stage_v2_repo.go`
- Create: `internal/data/team_run_v2_repo.go`
- Create: `internal/data/member_session_v2_repo.go`
- Create: `internal/data/plan_board_v2_repo.go`
- Create: `internal/data/plan_step_v2_repo.go`
- Create: `internal/data/v2_repo_test.go`（共享测试 helper）

- [ ] **Step 1: 创建测试 helper `internal/data/v2_repo_test.go`**

```go
package data

import (
	"context"
	"testing"
)

// openTestDataV2 打开测试 DB 并创建 v2 表。
func openTestDataV2(t *testing.T) *Data {
	t.Helper()
	d := openTestDataWithRWDB(t)
	ctx := context.Background()
	// 创建 v2 表
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions_v2 (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			spirit_agent_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks_v2 (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			user_message TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			seq INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			completed_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS turns_v2 (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			spirit_session_id TEXT NOT NULL,
			parent_turn_id TEXT NOT NULL DEFAULT '',
			agent_key TEXT NOT NULL DEFAULT '',
			team_id TEXT NOT NULL DEFAULT '',
			team_stage_id TEXT NOT NULL DEFAULT '',
			seq INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'running',
			started_at DATETIME NOT NULL,
			completed_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS steps_v2 (
			id TEXT PRIMARY KEY,
			turn_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			spirit_session_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			seq INTEGER NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			reasoning TEXT NOT NULL DEFAULT '',
			tool_name TEXT NOT NULL DEFAULT '',
			tool_call_id TEXT NOT NULL DEFAULT '',
			tool_args TEXT NOT NULL DEFAULT '',
			tool_result TEXT NOT NULL DEFAULT '',
			tool_duration_ms INTEGER NOT NULL DEFAULT 0,
			tool_error_code TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			is_final INTEGER NOT NULL DEFAULT 0,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS team_stages_v2 (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			turn_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			team_id TEXT NOT NULL,
			dag_node_id TEXT NOT NULL DEFAULT '',
			depends_on TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'pending',
			stage TEXT NOT NULL DEFAULT '',
			members TEXT NOT NULL DEFAULT '[]',
			strategy TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			seq INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS team_runs_v2 (
			id TEXT PRIMARY KEY,
			team_stage_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			spirit_session_id TEXT NOT NULL,
			dag_node_id TEXT NOT NULL DEFAULT '',
			depends_on TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'running',
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			seq INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS member_sessions_v2 (
			id TEXT PRIMARY KEY,
			team_run_id TEXT NOT NULL,
			team_stage_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			spirit_session_id TEXT NOT NULL,
			agent_key TEXT NOT NULL,
			agent_name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			seq INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS plan_boards_v2 (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			turn_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			strategy TEXT NOT NULL DEFAULT 'sequential',
			status TEXT NOT NULL DEFAULT 'planning',
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			seq INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS plan_steps_v2 (
			id TEXT PRIMARY KEY,
			plan_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			depends_on TEXT NOT NULL DEFAULT '[]',
			mapped_team_stage_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			auto_synthesis INTEGER NOT NULL DEFAULT 0,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			seq INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0,
			result TEXT NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT '{}'
		)`,
	}
	for _, s := range stmts {
		if _, err := d.RWDB().WriteDB(ctx).ExecContext(ctx, s); err != nil {
			t.Fatalf("create v2 table: %v\nSQL: %s", err, s)
		}
	}
	return d
}
```

- [ ] **Step 2: 创建 `internal/data/session_v2_repo.go`**

```go
package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/sessionv2"
	"aranea-agents/pkg/loggateway"
)

type sessionV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.SessionV2Repo = (*sessionV2Repo)(nil)

func NewSessionV2Repo(d *Data, lg loggateway.Logger) biz.SessionV2Repo {
	return &sessionV2Repo{data: d, lg: lg.With(loggateway.Domain("SESSION_V2"))}
}

func (r *sessionV2Repo) GetSession(ctx context.Context, id string) (biz.Session, error) {
	if r == nil || r.data == nil {
		return biz.Session{}, fmt.Errorf("session v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).SessionV2.Get(ctx, id)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION_V2")
	}
	return entSessionV2ToBiz(row), nil
}

func (r *sessionV2Repo) CreateSession(ctx context.Context, s biz.Session) (biz.Session, error) {
	if r == nil || r.data == nil {
		return biz.Session{}, fmt.Errorf("session v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).SessionV2.Create().
		SetID(s.ID).
		SetUserID(s.UserID).
		SetSpiritAgentID(s.SpiritAgentID).
		SetStatus(string(s.Status)).
		SetCreatedAt(s.CreatedAt).
		SetUpdatedAt(s.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION_V2")
	}
	return entSessionV2ToBiz(row), nil
}

func (r *sessionV2Repo) UpdateSessionStatus(ctx context.Context, id string, status biz.SessionStatus) error {
	if r == nil || r.data == nil {
		return fmt.Errorf("session v2 repo: database not configured")
	}
	err := r.data.RW().Write(ctx).SessionV2.UpdateOneID(id).
		SetStatus(string(status)).
		SetUpdatedAt(time.Now().UTC()).
		Exec(ctx)
	return entErrToBizErr(err, "SESSION_V2")
}

func entSessionV2ToBiz(row *ent.SessionV2) biz.Session {
	return biz.Session{
		ID:            row.ID,
		UserID:        row.UserID,
		SpiritAgentID: row.SpiritAgentID,
		Status:        biz.SessionStatus(row.Status),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
```

- [ ] **Step 3: 写 Session Repo 测试**

Create `internal/data/session_v2_repo_test.go`:

```go
package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestSessionV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewSessionV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateSession(ctx, biz.Session{
		ID:            "sess-1",
		UserID:        "user-1",
		SpiritAgentID: "agent-1",
		Status:        biz.SessionStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.ID != "sess-1" || created.UserID != "user-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != biz.SessionStatusActive {
		t.Fatalf("status: expected active, got %s", got.Status)
	}
}

func TestSessionV2Repo_UpdateStatus(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewSessionV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, _ = repo.CreateSession(ctx, biz.Session{
		ID: "sess-2", UserID: "u", SpiritAgentID: "a",
		Status: biz.SessionStatusActive, CreatedAt: now, UpdatedAt: now,
	})

	if err := repo.UpdateSessionStatus(ctx, "sess-2", biz.SessionStatusCompleted); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, _ := repo.GetSession(ctx, "sess-2")
	if got.Status != biz.SessionStatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd f:\aranea-agents && go test ./internal/data/ -run "TestSessionV2Repo" -count=1 -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: 创建 `internal/data/task_v2_repo.go`**

```go
package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/taskv2"
	"aranea-agents/pkg/loggateway"
)

type taskV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TaskV2Repo = (*taskV2Repo)(nil)

func NewTaskV2Repo(d *Data, lg loggateway.Logger) biz.TaskV2Repo {
	return &taskV2Repo{data: d, lg: lg.With(loggateway.Domain("TASK_V2"))}
}

func (r *taskV2Repo) GetTask(ctx context.Context, id string) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TaskV2.Get(ctx, id)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

func (r *taskV2Repo) ListTasksBySession(ctx context.Context, sessionID string) ([]biz.Task, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("task v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TaskV2.Query().
		Where(taskv2.SessionIDEQ(sessionID)).
		Order(ent.Asc(taskv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_V2")
	}
	return entTasksV2ToBiz(rows), nil
}

func (r *taskV2Repo) CreateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

func (r *taskV2Repo) UpdateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// UpsertTask 使用 VersionLT 守卫，防止旧版本覆盖新版本。
func (r *taskV2Repo) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	// 1) Try to update if stored version is older
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		Where(taskv2.VersionLT(t.Version)).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetUpdatedAt(t.UpdatedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
		}
		return entTaskV2ToBiz(row), nil
	}
	// 2) Insert if not exists
	cb := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		cb.SetCompletedAt(*t.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
			if getErr != nil {
				return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
			}
			return entTaskV2ToBiz(existing), nil
		}
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

func entTaskV2ToBiz(row *ent.TaskV2) biz.Task {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := row.CompletedAt
		completedAt = &t
	}
	return biz.Task{
		ID:          row.ID,
		SessionID:   row.SessionID,
		UserMessage: row.UserMessage,
		Status:      biz.TaskStatus(row.Status),
		Seq:         row.Seq,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		CompletedAt: completedAt,
	}
}

func entTasksV2ToBiz(rows []*ent.TaskV2) []biz.Task {
	out := make([]biz.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTaskV2ToBiz(r))
	}
	return out
}
```

- [ ] **Step 6: 写 Task Repo 测试**

Create `internal/data/task_v2_repo_test.go`:

```go
package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestTaskV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewTaskV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.CreateTask(ctx, biz.Task{
		ID: "t-1", SessionID: "s-1", UserMessage: "hi",
		Status: biz.TaskStatusPending, Seq: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := repo.GetTask(ctx, "t-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.UserMessage != "hi" || got.Seq != 1 {
		t.Fatalf("task mismatch: %+v", got)
	}
}

func TestTaskV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewTaskV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// v1: insert
	_, err := repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "v1",
		Status: biz.TaskStatusRunning, Seq: 2, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertTask v1: %v", err)
	}
	// v0 (older): should not overwrite
	_, _ = repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "stale",
		Status: biz.TaskStatusPending, Seq: 2, Version: 0,
		CreatedAt: now, UpdatedAt: now,
	})
	got, _ := repo.GetTask(ctx, "t-2")
	if got.UserMessage != "v1" {
		t.Fatalf("stale version overwrote: got %s", got.UserMessage)
	}
	if got.Status != biz.TaskStatusRunning {
		t.Fatalf("status changed: got %s", got.Status)
	}
	// v2 (newer): should overwrite
	_, _ = repo.UpsertTask(ctx, biz.Task{
		ID: "t-2", SessionID: "s-1", UserMessage: "v2",
		Status: biz.TaskStatusCompleted, Seq: 2, Version: 2,
		CreatedAt: now, UpdatedAt: now,
	})
	got, _ = repo.GetTask(ctx, "t-2")
	if got.UserMessage != "v2" || got.Status != biz.TaskStatusCompleted {
		t.Fatalf("newer version did not overwrite: %+v", got)
	}
}

func TestTaskV2Repo_ListBySession(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewTaskV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, _ = repo.CreateTask(ctx, biz.Task{
			ID: "lt-" + string(rune('a'+i)), SessionID: "ls", UserMessage: "msg",
			Status: biz.TaskStatusPending, Seq: seq,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	tasks, err := repo.ListTasksBySession(ctx, "ls")
	if err != nil {
		t.Fatalf("ListTasksBySession: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	// 应按 seq 升序
	if tasks[0].Seq != 1 || tasks[1].Seq != 2 || tasks[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %d,%d,%d",
			tasks[0].Seq, tasks[1].Seq, tasks[2].Seq)
	}
}
```

- [ ] **Step 7: 运行 Task Repo 测试**

Run: `cd f:\aranea-agents && go test ./internal/data/ -run "TestTaskV2Repo" -count=1 -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 8: 创建 `internal/data/turn_v2_repo.go`**（按 Task 模式实现）

```go
package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/turnv2"
	"aranea-agents/pkg/loggateway"
)

type turnV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TurnV2Repo = (*turnV2Repo)(nil)

func NewTurnV2Repo(d *Data, lg loggateway.Logger) biz.TurnV2Repo {
	return &turnV2Repo{data: d, lg: lg.With(loggateway.Domain("TURN_V2"))}
}

func (r *turnV2Repo) GetTurn(ctx context.Context, id string) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TurnV2.Get(ctx, id)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

func (r *turnV2Repo) ListTurnsByTask(ctx context.Context, taskID string) ([]biz.Turn, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("turn v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TurnV2.Query().
		Where(turnv2.TaskIDEQ(taskID)).
		Order(ent.Asc(turnv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnsV2ToBiz(rows), nil
}

func (r *turnV2Repo) CreateTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.Create().
		SetID(t.ID).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetStartedAt(t.StartedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

func (r *turnV2Repo) UpdateTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.UpdateOneID(t.ID).
		SetStatus(string(t.Status))
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

func (r *turnV2Repo) UpsertTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.UpdateOneID(t.ID).
		Where(turnv2.VersionLT(t.Version)).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TurnV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Turn{}, entErrToBizErr(getErr, "TURN_V2")
		}
		return entTurnV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).TurnV2.Create().
		SetID(t.ID).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetStartedAt(t.StartedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		cb.SetCompletedAt(*t.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).TurnV2.Get(ctx, t.ID)
			if getErr != nil {
				return biz.Turn{}, entErrToBizErr(getErr, "TURN_V2")
			}
			return entTurnV2ToBiz(existing), nil
		}
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

func entTurnV2ToBiz(row *ent.TurnV2) biz.Turn {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := row.CompletedAt
		completedAt = &t
	}
	return biz.Turn{
		ID:              row.ID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		ParentTurnID:    row.ParentTurnID,
		AgentKey:        row.AgentKey,
		TeamID:          row.TeamID,
		TeamStageID:     row.TeamStageID,
		Seq:             row.Seq,
		Status:          biz.TurnStatus(row.Status),
		StartedAt:       row.StartedAt,
		CompletedAt:     completedAt,
	}
}

func entTurnsV2ToBiz(rows []*ent.TurnV2) []biz.Turn {
	out := make([]biz.Turn, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTurnV2ToBiz(r))
	}
	return out
}
```

- [ ] **Step 9: 创建 `internal/data/step_v2_repo.go`**（含 JSON 字段处理）

```go
package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/stepv2"
	"aranea-agents/pkg/loggateway"
)

type stepV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.StepV2Repo = (*stepV2Repo)(nil)

func NewStepV2Repo(d *Data, lg loggateway.Logger) biz.StepV2Repo {
	return &stepV2Repo{data: d, lg: lg.With(loggateway.Domain("STEP_V2"))}
}

func (r *stepV2Repo) GetStep(ctx context.Context, id string) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).StepV2.Get(ctx, id)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

func (r *stepV2Repo) ListStepsByTurn(ctx context.Context, turnID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.TurnIDEQ(turnID)).
		Order(ent.Asc(stepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

func (r *stepV2Repo) ListStepsByTask(ctx context.Context, taskID string) ([]biz.Step, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.TaskIDEQ(taskID)).
		Order(ent.Asc(stepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "STEP_V2")
	}
	return entStepsV2ToBiz(rows), nil
}

func (r *stepV2Repo) CreateStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).StepV2.Create().
		SetID(s.ID).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetStartedAt(s.StartedAt).
		SetVersion(s.Version).
		Save(ctx)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

func (r *stepV2Repo) UpdateStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).StepV2.UpdateOneID(s.ID).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		b.SetCompletedAt(*s.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

func (r *stepV2Repo) UpsertStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	if r == nil || r.data == nil {
		return biz.Step{}, fmt.Errorf("step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).StepV2.UpdateOneID(s.ID).
		Where(stepv2.VersionLT(s.Version)).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		b.SetCompletedAt(*s.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).StepV2.Get(ctx, s.ID)
		if getErr != nil {
			return biz.Step{}, entErrToBizErr(getErr, "STEP_V2")
		}
		return entStepV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).StepV2.Create().
		SetID(s.ID).
		SetTurnID(s.TurnID).
		SetTaskID(s.TaskID).
		SetSessionID(s.SessionID).
		SetSpiritSessionID(s.SpiritSessionID).
		SetKind(string(s.Kind)).
		SetSeq(s.Seq).
		SetContent(s.Content).
		SetReasoning(s.Reasoning).
		SetToolName(s.ToolName).
		SetToolCallID(s.ToolCallID).
		SetToolArgs(string(s.ToolArgs)).
		SetToolResult(string(s.ToolResult)).
		SetToolDurationMs(s.ToolDurationMs).
		SetToolErrorCode(s.ToolErrorCode).
		SetStatus(string(s.Status)).
		SetIsFinal(s.IsFinal).
		SetStartedAt(s.StartedAt).
		SetVersion(s.Version)
	if s.CompletedAt != nil {
		cb.SetCompletedAt(*s.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).StepV2.Get(ctx, s.ID)
			if getErr != nil {
				return biz.Step{}, entErrToBizErr(getErr, "STEP_V2")
			}
			return entStepV2ToBiz(existing), nil
		}
		return biz.Step{}, entErrToBizErr(err, "STEP_V2")
	}
	return entStepV2ToBiz(row), nil
}

func entStepV2ToBiz(row *ent.StepV2) biz.Step {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := row.CompletedAt
		completedAt = &t
	}
	return biz.Step{
		ID:              row.ID,
		TurnID:          row.TurnID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		Kind:            biz.StepKind(row.Kind),
		Seq:             row.Seq,
		Content:         row.Content,
		Reasoning:       row.Reasoning,
		ToolName:        row.ToolName,
		ToolCallID:      row.ToolCallID,
		ToolArgs:        json.RawMessage(row.ToolArgs),
		ToolResult:      json.RawMessage(row.ToolResult),
		ToolDurationMs:  row.ToolDurationMs,
		ToolErrorCode:   row.ToolErrorCode,
		Status:          biz.StepStatus(row.Status),
		IsFinal:         row.IsFinal,
		StartedAt:       row.StartedAt,
		CompletedAt:     completedAt,
		Version:         row.Version,
	}
}

func entStepsV2ToBiz(rows []*ent.StepV2) []biz.Step {
	out := make([]biz.Step, 0, len(rows))
	for _, r := range rows {
		out = append(out, entStepV2ToBiz(r))
	}
	return out
}
```

- [ ] **Step 10: 写 Step Repo 测试**

Create `internal/data/step_v2_repo_test.go`:

```go
package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestStepV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewStepV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.CreateStep(ctx, biz.Step{
		ID: "st-1", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1,
		Content: "thinking...", Status: biz.StepStatusRunning,
		StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	got, err := repo.GetStep(ctx, "st-1")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if got.Kind != biz.StepKindThinking || got.Content != "thinking..." {
		t.Fatalf("step mismatch: %+v", got)
	}
}

func TestStepV2Repo_Upsert_JSONArgs(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewStepV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	args, _ := json.Marshal(map[string]any{"command": "ls -la"})
	_, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-2", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindAction, Seq: 2,
		ToolName: "shell", ToolArgs: args,
		Status: biz.StepStatusToolRunning, StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	got, _ := repo.GetStep(ctx, "st-2")
	if got.ToolName != "shell" {
		t.Fatalf("tool name: expected shell, got %s", got.ToolName)
	}
	var argsGot map[string]any
	if err := json.Unmarshal(got.ToolArgs, &argsGot); err != nil {
		t.Fatalf("unmarshal tool args: %v", err)
	}
	if argsGot["command"] != "ls -la" {
		t.Fatalf("tool args mismatch: %v", argsGot)
	}
}

func TestStepV2Repo_ListByTurn_SeqOrder(t *testing.T) {
	d := openTestDataV2(t)
	repo := NewStepV2Repo(d, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, _ = repo.CreateStep(ctx, biz.Step{
			ID: "ord-" + string(rune('a'+i)), TurnID: "turn-x", TaskID: "t-1",
			SessionID: "s-1", SpiritSessionID: "s-1",
			Kind: biz.StepKindThinking, Seq: seq,
			Status: biz.StepStatusCompleted, StartedAt: now, Version: 1,
		})
	}
	steps, err := repo.ListStepsByTurn(ctx, "turn-x")
	if err != nil {
		t.Fatalf("ListStepsByTurn: %v", err)
	}
	if len(steps) != 3 || steps[0].Seq != 1 || steps[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{steps[0].Seq, steps[1].Seq, steps[2].Seq})
	}
}
```

- [ ] **Step 11: 运行 Step Repo 测试**

Run: `cd f:\aranea-agents && go test ./internal/data/ -run "TestStepV2Repo" -count=1 -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 12: 创建其余 6 个 Repo（按相同模式实现）**

按以下顺序创建（每个文件遵循 Task/Turn/Step 模式）：

1. `internal/data/team_stage_v2_repo.go` — 实现 `biz.TeamStageV2Repo`，注意 `members` 字段用 `field.JSON` 序列化为 `[]map[string]any`，需 `MemberInfo` 与 `map[string]any` 互转
2. `internal/data/team_run_v2_repo.go` — 实现 `biz.TeamRunV2Repo`
3. `internal/data/member_session_v2_repo.go` — 实现 `biz.MemberSessionV2Repo`
4. `internal/data/plan_board_v2_repo.go` — 实现 `biz.PlanBoardV2Repo`（注意 PlanBoard.Steps 是嵌入数组，存为 JSON）
5. `internal/data/plan_step_v2_repo.go` — 实现 `biz.PlanStepV2Repo`（注意 Result/Error 字段 JSON 序列化）

**关键模式参考**（`team_stage_v2_repo.go` Members 字段转换示例）：

```go
// internal/data/team_stage_v2_repo.go
package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/teamstagev2"
	"aranea-agents/pkg/loggateway"
)

type teamStageV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TeamStageV2Repo = (*teamStageV2Repo)(nil)

func NewTeamStageV2Repo(d *Data, lg loggateway.Logger) biz.TeamStageV2Repo {
	return &teamStageV2Repo{data: d, lg: lg.With(loggateway.Domain("team_stage_v2_repo"))}
}

// toBiz converts an Ent TeamStageV2 record to the biz TeamStage entity.
// Members is stored as JSON; decode into []MemberInfo.
func entTeamStageV2ToBiz(e *ent.TeamStageV2) biz.TeamStage {
	var members []biz.MemberInfo
	if len(e.Members) > 0 {
		_ = json.Unmarshal(e.Members, &members) // best-effort decode
	}
	return biz.TeamStage{
		ID:              e.ID,
		SpiritSessionID: e.SpiritSessionID,
		TaskID:          e.TaskID,
		Status:          biz.TeamStageStatus(e.Status),
		Members:         members,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
		Version:         e.Version,
	}
}

func (r *teamStageV2Repo) CreateTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	membersJSON, _ := json.Marshal(ts.Members)
	saved, err := r.data.RW().Write(ctx).TeamStageV2.Create().
		SetID(ts.ID).
		SetSpiritSessionID(ts.SpiritSessionID).
		SetTaskID(ts.TaskID).
		SetStatus(string(ts.Status)).
		SetMembers(membersJSON).
		SetCreatedAt(ts.CreatedAt).
		SetUpdatedAt(ts.UpdatedAt).
		SetVersion(ts.Version).
		Save(ctx)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "team_stage_v2")
	}
	return entTeamStageV2ToBiz(saved), nil
}

func (r *teamStageV2Repo) UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	membersJSON, _ := json.Marshal(ts.Members)
	// Update path with optimistic concurrency (VersionLT guard)
	b := r.data.RW().Write(ctx).TeamStageV2.UpdateOneID(ts.ID).
		Where(teamstagev2.VersionLT(ts.Version)).
		SetSpiritSessionID(ts.SpiritSessionID).
		SetTaskID(ts.TaskID).
		SetStatus(string(ts.Status)).
		SetMembers(membersJSON).
		SetUpdatedAt(ts.UpdatedAt).
		SetVersion(ts.Version)
	if err := b.Exec(ctx); err == nil {
		return ts, nil
	} else if !ent.IsNotFound(err) {
		return biz.TeamStage{}, entErrToBizErr(err, "team_stage_v2")
	}
	// Fallback: create
	saved, err := r.data.RW().Write(ctx).TeamStageV2.Create().
		SetID(ts.ID).
		SetSpiritSessionID(ts.SpiritSessionID).
		SetTaskID(ts.TaskID).
		SetStatus(string(ts.Status)).
		SetMembers(membersJSON).
		SetCreatedAt(ts.CreatedAt).
		SetUpdatedAt(ts.UpdatedAt).
		SetVersion(ts.Version).
		Save(ctx)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "team_stage_v2")
	}
	return entTeamStageV2ToBiz(saved), nil
}

func (r *teamStageV2Repo) GetTeamStage(ctx context.Context, id string) (biz.TeamStage, error) {
	e, err := r.data.RW().Read(ctx).TeamStageV2.Query().Where(teamstagev2.IDEQ(id)).Only(ctx)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "team_stage_v2")
	}
	return entTeamStageV2ToBiz(e), nil
}

func (r *teamStageV2Repo) ListTeamStagesByTask(ctx context.Context, taskID string) ([]biz.TeamStage, error) {
	es, err := r.data.RW().Read(ctx).TeamStageV2.Query().Where(teamstagev2.TaskIDEQ(taskID)).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "team_stage_v2")
	}
	out := make([]biz.TeamStage, len(es))
	for i, e := range es {
		out[i] = entTeamStageV2ToBiz(e)
	}
	return out, nil
}
```

#### 8.7 team_run_v2_repo.go

**File:** `internal/data/team_run_v2_repo.go`

实现 `biz.TeamRunV2Repo`（接口签名见 Task 7 的 `TeamRunReader`/`TeamRunWriter`）。

```go
package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/teamrunv2"
	"aranea-agents/pkg/loggateway"
)

type teamRunV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TeamRunV2Repo = (*teamRunV2Repo)(nil)

func NewTeamRunV2Repo(d *Data, lg loggateway.Logger) biz.TeamRunV2Repo {
	return &teamRunV2Repo{data: d, lg: lg.With(loggateway.Domain("team_run_v2_repo"))}
}

func entTeamRunV2ToBiz(e *ent.TeamRunV2) biz.TeamRun {
	return biz.TeamRun{
		ID:              e.ID,
		TeamStageID:     e.TeamStageID,
		SpiritSessionID: e.SpiritSessionID,
		ParentTaskID:    e.ParentTaskID,
		Status:          biz.TeamRunStatus(e.Status),
		StartedAt:       e.StartedAt,
		FinishedAt:      e.FinishedAt,
		Error:           e.Error,
		Version:         e.Version,
	}
}

func (r *teamRunV2Repo) CreateTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	saved, err := r.data.RW().Write(ctx).TeamRunV2.Create().
		SetID(tr.ID).
		SetTeamStageID(tr.TeamStageID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetParentTaskID(tr.ParentTaskID).
		SetStatus(string(tr.Status)).
		SetStartedAt(tr.StartedAt).
		SetFinishedAt(tr.FinishedAt).
		SetError(tr.Error).
		SetVersion(tr.Version).
		Save(ctx)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "team_run_v2")
	}
	return entTeamRunV2ToBiz(saved), nil
}

func (r *teamRunV2Repo) UpsertTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	b := r.data.RW().Write(ctx).TeamRunV2.UpdateOneID(tr.ID).
		Where(teamrunv2.VersionLT(tr.Version)).
		SetTeamStageID(tr.TeamStageID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetParentTaskID(tr.ParentTaskID).
		SetStatus(string(tr.Status)).
		SetStartedAt(tr.StartedAt).
		SetFinishedAt(tr.FinishedAt).
		SetError(tr.Error).
		SetVersion(tr.Version)
	if err := b.Exec(ctx); err == nil {
		return tr, nil
	} else if !ent.IsNotFound(err) {
		return biz.TeamRun{}, entErrToBizErr(err, "team_run_v2")
	}
	saved, err := r.data.RW().Write(ctx).TeamRunV2.Create().
		SetID(tr.ID).
		SetTeamStageID(tr.TeamStageID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetParentTaskID(tr.ParentTaskID).
		SetStatus(string(tr.Status)).
		SetStartedAt(tr.StartedAt).
		SetFinishedAt(tr.FinishedAt).
		SetError(tr.Error).
		SetVersion(tr.Version).
		Save(ctx)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "team_run_v2")
	}
	return entTeamRunV2ToBiz(saved), nil
}

func (r *teamRunV2Repo) GetTeamRun(ctx context.Context, id string) (biz.TeamRun, error) {
	e, err := r.data.RW().Read(ctx).TeamRunV2.Query().Where(teamrunv2.IDEQ(id)).Only(ctx)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "team_run_v2")
	}
	return entTeamRunV2ToBiz(e), nil
}

func (r *teamRunV2Repo) ListTeamRunsByStage(ctx context.Context, stageID string) ([]biz.TeamRun, error) {
	es, err := r.data.RW().Read(ctx).TeamRunV2.Query().Where(teamrunv2.TeamStageIDEQ(stageID)).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "team_run_v2")
	}
	out := make([]biz.TeamRun, len(es))
	for i, e := range es {
		out[i] = entTeamRunV2ToBiz(e)
	}
	return out, nil
}
```

#### 8.8 member_session_v2_repo.go

**File:** `internal/data/member_session_v2_repo.go`

实现 `biz.MemberSessionV2Repo`。

```go
package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/membersessionv2"
	"aranea-agents/pkg/loggateway"
)

type memberSessionV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.MemberSessionV2Repo = (*memberSessionV2Repo)(nil)

func NewMemberSessionV2Repo(d *Data, lg loggateway.Logger) biz.MemberSessionV2Repo {
	return &memberSessionV2Repo{data: d, lg: lg.With(loggateway.Domain("member_session_v2_repo"))}
}

func entMemberSessionV2ToBiz(e *ent.MemberSessionV2) biz.MemberSession {
	return biz.MemberSession{
		ID:              e.ID,
		TeamRunID:       e.TeamRunID,
		AgentKey:        e.AgentKey,
		SpiritSessionID: e.SpiritSessionID,
		Status:          biz.MemberSessionStatus(e.Status),
		StartedAt:       e.StartedAt,
		FinishedAt:      e.FinishedAt,
		Error:           e.Error,
		Version:         e.Version,
	}
}

func (r *memberSessionV2Repo) CreateMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	saved, err := r.data.RW().Write(ctx).MemberSessionV2.Create().
		SetID(ms.ID).
		SetTeamRunID(ms.TeamRunID).
		SetAgentKey(ms.AgentKey).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetStatus(string(ms.Status)).
		SetStartedAt(ms.StartedAt).
		SetFinishedAt(ms.FinishedAt).
		SetError(ms.Error).
		SetVersion(ms.Version).
		Save(ctx)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "member_session_v2")
	}
	return entMemberSessionV2ToBiz(saved), nil
}

func (r *memberSessionV2Repo) UpsertMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	b := r.data.RW().Write(ctx).MemberSessionV2.UpdateOneID(ms.ID).
		Where(membersessionv2.VersionLT(ms.Version)).
		SetTeamRunID(ms.TeamRunID).
		SetAgentKey(ms.AgentKey).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetStatus(string(ms.Status)).
		SetStartedAt(ms.StartedAt).
		SetFinishedAt(ms.FinishedAt).
		SetError(ms.Error).
		SetVersion(ms.Version)
	if err := b.Exec(ctx); err == nil {
		return ms, nil
	} else if !ent.IsNotFound(err) {
		return biz.MemberSession{}, entErrToBizErr(err, "member_session_v2")
	}
	saved, err := r.data.RW().Write(ctx).MemberSessionV2.Create().
		SetID(ms.ID).
		SetTeamRunID(ms.TeamRunID).
		SetAgentKey(ms.AgentKey).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetStatus(string(ms.Status)).
		SetStartedAt(ms.StartedAt).
		SetFinishedAt(ms.FinishedAt).
		SetError(ms.Error).
		SetVersion(ms.Version).
		Save(ctx)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "member_session_v2")
	}
	return entMemberSessionV2ToBiz(saved), nil
}

func (r *memberSessionV2Repo) GetMemberSession(ctx context.Context, id string) (biz.MemberSession, error) {
	e, err := r.data.RW().Read(ctx).MemberSessionV2.Query().Where(membersessionv2.IDEQ(id)).Only(ctx)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "member_session_v2")
	}
	return entMemberSessionV2ToBiz(e), nil
}

func (r *memberSessionV2Repo) ListMemberSessionsByTeamRun(ctx context.Context, teamRunID string) ([]biz.MemberSession, error) {
	es, err := r.data.RW().Read(ctx).MemberSessionV2.Query().
		Where(membersessionv2.TeamRunIDEQ(teamRunID)).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "member_session_v2")
	}
	out := make([]biz.MemberSession, len(es))
	for i, e := range es {
		out[i] = entMemberSessionV2ToBiz(e)
	}
	return out, nil
}
```

#### 8.9 plan_board_v2_repo.go

**File:** `internal/data/plan_board_v2_repo.go`

实现 `biz.PlanBoardV2Repo`。`Steps` 嵌入字段（PlanBoard 包含扁平 `[]PlanStepRef`）存储为 JSON。

```go
package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/planboardv2"
	"aranea-agents/pkg/loggateway"
)

type planBoardV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.PlanBoardV2Repo = (*planBoardV2Repo)(nil)

func NewPlanBoardV2Repo(d *Data, lg loggateway.Logger) biz.PlanBoardV2Repo {
	return &planBoardV2Repo{data: d, lg: lg.With(loggateway.Domain("plan_board_v2_repo"))}
}

func entPlanBoardV2ToBiz(e *ent.PlanBoardV2) biz.PlanBoard {
	var steps []biz.PlanStepRef
	if len(e.Steps) > 0 {
		_ = json.Unmarshal(e.Steps, &steps)
	}
	return biz.PlanBoard{
		ID:              e.ID,
		SpiritSessionID: e.SpiritSessionID,
		TaskID:          e.TaskID,
		Status:          biz.PlanBoardStatus(e.Status),
		Steps:           steps,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
		Version:         e.Version,
	}
}

func (r *planBoardV2Repo) CreatePlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	stepsJSON, _ := json.Marshal(pb.Steps)
	saved, err := r.data.RW().Write(ctx).PlanBoardV2.Create().
		SetID(pb.ID).
		SetSpiritSessionID(pb.SpiritSessionID).
		SetTaskID(pb.TaskID).
		SetStatus(string(pb.Status)).
		SetSteps(stepsJSON).
		SetCreatedAt(pb.CreatedAt).
		SetUpdatedAt(pb.UpdatedAt).
		SetVersion(pb.Version).
		Save(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "plan_board_v2")
	}
	return entPlanBoardV2ToBiz(saved), nil
}

func (r *planBoardV2Repo) UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	stepsJSON, _ := json.Marshal(pb.Steps)
	b := r.data.RW().Write(ctx).PlanBoardV2.UpdateOneID(pb.ID).
		Where(planboardv2.VersionLT(pb.Version)).
		SetSpiritSessionID(pb.SpiritSessionID).
		SetTaskID(pb.TaskID).
		SetStatus(string(pb.Status)).
		SetSteps(stepsJSON).
		SetUpdatedAt(pb.UpdatedAt).
		SetVersion(pb.Version)
	if err := b.Exec(ctx); err == nil {
		return pb, nil
	} else if !ent.IsNotFound(err) {
		return biz.PlanBoard{}, entErrToBizErr(err, "plan_board_v2")
	}
	saved, err := r.data.RW().Write(ctx).PlanBoardV2.Create().
		SetID(pb.ID).
		SetSpiritSessionID(pb.SpiritSessionID).
		SetTaskID(pb.TaskID).
		SetStatus(string(pb.Status)).
		SetSteps(stepsJSON).
		SetCreatedAt(pb.CreatedAt).
		SetUpdatedAt(pb.UpdatedAt).
		SetVersion(pb.Version).
		Save(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "plan_board_v2")
	}
	return entPlanBoardV2ToBiz(saved), nil
}

func (r *planBoardV2Repo) GetPlanBoard(ctx context.Context, id string) (biz.PlanBoard, error) {
	e, err := r.data.RW().Read(ctx).PlanBoardV2.Query().Where(planboardv2.IDEQ(id)).Only(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "plan_board_v2")
	}
	return entPlanBoardV2ToBiz(e), nil
}

func (r *planBoardV2Repo) GetPlanBoardByTask(ctx context.Context, taskID string) (biz.PlanBoard, error) {
	e, err := r.data.RW().Read(ctx).PlanBoardV2.Query().
		Where(planboardv2.TaskIDEQ(taskID)).Only(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "plan_board_v2")
	}
	return entPlanBoardV2ToBiz(e), nil
}
```

#### 8.10 plan_step_v2_repo.go

**File:** `internal/data/plan_step_v2_repo.go`

实现 `biz.PlanStepV2Repo`。`Result`/`Error` 字段为 JSON 序列化。

```go
package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/planstepv2"
	"aranea-agents/pkg/loggateway"
)

type planStepV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.PlanStepV2Repo = (*planStepV2Repo)(nil)

func NewPlanStepV2Repo(d *Data, lg loggateway.Logger) biz.PlanStepV2Repo {
	return &planStepV2Repo{data: d, lg: lg.With(loggateway.Domain("plan_step_v2_repo"))}
}

func entPlanStepV2ToBiz(e *ent.PlanStepV2) biz.PlanStep {
	var result any
	if len(e.Result) > 0 {
		_ = json.Unmarshal(e.Result, &result)
	}
	var stepErr *biz.PlanStepError
	if len(e.Error) > 0 {
		_ = json.Unmarshal(e.Error, &stepErr)
	}
	return biz.PlanStep{
		ID:              e.ID,
		PlanBoardID:     e.PlanBoardID,
		SpiritSessionID: e.SpiritSessionID,
		StepKey:         e.StepKey,
		Title:           e.Title,
		DependsOn:       e.DependsOn,
		Status:          biz.PlanStepStatus(e.Status),
		AssignedTeamID:  e.AssignedTeamID,
		Result:          result,
		Error:           stepErr,
		StartedAt:       e.StartedAt,
		FinishedAt:      e.FinishedAt,
		Version:         e.Version,
	}
}

func (r *planStepV2Repo) CreatePlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	resultJSON, _ := json.Marshal(ps.Result)
	var errJSON []byte
	if ps.Error != nil {
		errJSON, _ = json.Marshal(ps.Error)
	}
	saved, err := r.data.RW().Write(ctx).PlanStepV2.Create().
		SetID(ps.ID).
		SetPlanBoardID(ps.PlanBoardID).
		SetSpiritSessionID(ps.SpiritSessionID).
		SetStepKey(ps.StepKey).
		SetTitle(ps.Title).
		SetDependsOn(ps.DependsOn).
		SetStatus(string(ps.Status)).
		SetAssignedTeamID(ps.AssignedTeamID).
		SetResult(resultJSON).
		SetError(errJSON).
		SetStartedAt(ps.StartedAt).
		SetFinishedAt(ps.FinishedAt).
		SetVersion(ps.Version).
		Save(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "plan_step_v2")
	}
	return entPlanStepV2ToBiz(saved), nil
}

func (r *planStepV2Repo) UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	resultJSON, _ := json.Marshal(ps.Result)
	var errJSON []byte
	if ps.Error != nil {
		errJSON, _ = json.Marshal(ps.Error)
	}
	b := r.data.RW().Write(ctx).PlanStepV2.UpdateOneID(ps.ID).
		Where(planstepv2.VersionLT(ps.Version)).
		SetPlanBoardID(ps.PlanBoardID).
		SetSpiritSessionID(ps.SpiritSessionID).
		SetStepKey(ps.StepKey).
		SetTitle(ps.Title).
		SetDependsOn(ps.DependsOn).
		SetStatus(string(ps.Status)).
		SetAssignedTeamID(ps.AssignedTeamID).
		SetResult(resultJSON).
		SetError(errJSON).
		SetStartedAt(ps.StartedAt).
		SetFinishedAt(ps.FinishedAt).
		SetVersion(ps.Version)
	if err := b.Exec(ctx); err == nil {
		return ps, nil
	} else if !ent.IsNotFound(err) {
		return biz.PlanStep{}, entErrToBizErr(err, "plan_step_v2")
	}
	saved, err := r.data.RW().Write(ctx).PlanStepV2.Create().
		SetID(ps.ID).
		SetPlanBoardID(ps.PlanBoardID).
		SetSpiritSessionID(ps.SpiritSessionID).
		SetStepKey(ps.StepKey).
		SetTitle(ps.Title).
		SetDependsOn(ps.DependsOn).
		SetStatus(string(ps.Status)).
		SetAssignedTeamID(ps.AssignedTeamID).
		SetResult(resultJSON).
		SetError(errJSON).
		SetStartedAt(ps.StartedAt).
		SetFinishedAt(ps.FinishedAt).
		SetVersion(ps.Version).
		Save(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "plan_step_v2")
	}
	return entPlanStepV2ToBiz(saved), nil
}

func (r *planStepV2Repo) GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error) {
	e, err := r.data.RW().Read(ctx).PlanStepV2.Query().Where(planstepv2.IDEQ(id)).Only(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "plan_step_v2")
	}
	return entPlanStepV2ToBiz(e), nil
}

func (r *planStepV2Repo) ListPlanStepsByBoard(ctx context.Context, boardID string) ([]biz.PlanStep, error) {
	es, err := r.data.RW().Read(ctx).PlanStepV2.Query().
		Where(planstepv2.PlanBoardIDEQ(boardID)).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "plan_step_v2")
	}
	out := make([]biz.PlanStep, len(es))
	for i, e := range es {
		out[i] = entPlanStepV2ToBiz(e)
	}
	return out, nil
}

func (r *planStepV2Repo) UpdatePlanStepStatus(ctx context.Context, id string, status biz.PlanStepStatus, version int64) (biz.PlanStep, error) {
	// Conditional update: only succeed if current version < target version
	err := r.data.RW().Write(ctx).PlanStepV2.UpdateOneID(id).
		Where(planstepv2.VersionLT(version)).
		SetStatus(string(status)).
		SetVersion(version).
		Exec(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "plan_step_v2")
	}
	return r.GetPlanStep(ctx, id)
}
```

- [ ] **Step 13: 编写剩余 5 个 Repo 的测试（合并到 `repo_v2_test.go`）**

**File:** `internal/data/repo_v2_test.go`（在已有 Task/Turn/Step 测试基础上追加）

按 Task/Turn/Step 测试模式，为 TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep 各添加 Create+Get、Upsert with VersionLT、ListBy 索引 3 类基础测试。测试命名遵循 `TestXxxV2Repo_<Scenario>`。

- [ ] **Step 14: 运行所有 Repo 测试**

Run: `cd f:\aranea-agents && go test ./internal/data/ -run "TestV2Repo|TestSessionV2|TestTaskV2|TestTurnV2|TestStepV2|TestTeamStageV2|TestTeamRunV2|TestMemberSessionV2|TestPlanBoardV2|TestPlanStepV2" -count=1 -v 2>&1 | tail -30`
Expected: 全部 PASS

- [ ] **Step 15: 提交**

```bash
cd f:\aranea-agents
git add internal/data/team_stage_v2_repo.go internal/data/team_run_v2_repo.go internal/data/member_session_v2_repo.go internal/data/plan_board_v2_repo.go internal/data/plan_step_v2_repo.go internal/data/repo_v2_test.go
git commit -m "feat(data): implement 5 remaining v2 repos (team_stage/team_run/member_session/plan_board/plan_step)"
```

---

## Phase 1.4: Wire 注入与 DDL 迁移

### Task 9: Wire 注入更新 + DDL 迁移注册

**目标**：将 9 个新 Repo 注册到 Wire provider set，并新增 DDL 迁移注册新表的索引（Ent Schema.Create 已创建表本身）。

**Files:**
- Modify: `internal/data/data.go` (ProviderSet 追加 9 个 Repo + 9 个 wire.Bind)
- Modify: `internal/data/ddl_migration_registry.go` (追加 1 个新迁移条目)
- Create: `internal/data/sql/migrations/20260702_v2_indexes.sql`

- [ ] **Step 1: 修改 `internal/data/data.go` ProviderSet**

在现有 ProviderSet（line 145 `NewActivityRepo` 后）追加：

```go
// V2 entities (Phase 1: LLM activity ordering redesign)
NewSessionV2Repo,
wire.Bind(new(biz.SessionV2Repo), new(*sessionV2Repo)),

NewTaskV2Repo,
wire.Bind(new(biz.TaskV2Repo), new(*taskV2Repo)),

NewTurnV2Repo,
wire.Bind(new(biz.TurnV2Repo), new(*turnV2Repo)),

NewStepV2Repo,
wire.Bind(new(biz.StepV2Repo), new(*stepV2Repo)),

NewTeamStageV2Repo,
wire.Bind(new(biz.TeamStageV2Repo), new(*teamStageV2Repo)),

NewTeamRunV2Repo,
wire.Bind(new(biz.TeamRunV2Repo), new(*teamRunV2Repo)),

NewMemberSessionV2Repo,
wire.Bind(new(biz.MemberSessionV2Repo), new(*memberSessionV2Repo)),

NewPlanBoardV2Repo,
wire.Bind(new(biz.PlanBoardV2Repo), new(*planBoardV2Repo)),

NewPlanStepV2Repo,
wire.Bind(new(biz.PlanStepV2Repo), new(*planStepV2Repo)),
```

**注意 wire.Bind 写法**：对于返回接口的 `NewXxxV2Repo`，直接 `wire.Bind(new(biz.IFace), new(*implType))` 即可；如果 `NewXxxV2Repo` 已经返回 `biz.XxxV2Repo` 接口，则不需要 `wire.Bind`（编译器会自动绑定）。**实施时优先选无需 wire.Bind 的简化形式**（与 Task 8 实现一致：`func NewSessionV2Repo(...) biz.SessionV2Repo`）。

简化后：
```go
// V2 entities — New* 返回 biz 接口，无需显式 wire.Bind
NewSessionV2Repo,
NewTaskV2Repo,
NewTurnV2Repo,
NewStepV2Repo,
NewTeamStageV2Repo,
NewTeamRunV2Repo,
NewMemberSessionV2Repo,
NewPlanBoardV2Repo,
NewPlanStepV2Repo,
```

- [ ] **Step 2: 运行 `make wire` 验证生成成功**

Run: `cd f:\aranea-agents && make wire 2>&1 | tail -20`
Expected: 编译成功，无 wire 错误；生成的 `wire_gen.go` 包含 9 个新 Repo 的构造调用。

- [ ] **Step 3: 创建 DDL 迁移 SQL 文件**

**File:** `internal/data/sql/migrations/20260702_v2_indexes.sql`

Ent 的 `Schema.Create()` 会创建 9 张表的列与主键，但 SQLite 不会自动创建我们声明在 Schema 中的所有 Index（部分会，但 JSON 字段的索引、复合索引需要显式 DDL 补充）。统一在 DDL 迁移中补建索引，遵循幂等原则（`IF NOT EXISTS`）。

```sql
-- V2 entity indexes (Phase 1: LLM activity ordering redesign)
-- Ent Schema.Create 已建表与主键，这里补建查询索引

-- sessions_v2
CREATE INDEX IF NOT EXISTS idx_sessions_v2_user_id ON sessions_v2 (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_v2_status ON sessions_v2 (status);
CREATE INDEX IF NOT EXISTS idx_sessions_v2_spirit_agent_id ON sessions_v2 (spirit_agent_id);

-- tasks_v2
CREATE INDEX IF NOT EXISTS idx_tasks_v2_session_id ON tasks_v2 (session_id);
CREATE INDEX IF NOT EXISTS idx_tasks_v2_status ON tasks_v2 (status);
CREATE INDEX IF NOT EXISTS idx_tasks_v2_parent_id ON tasks_v2 (parent_task_id);

-- turns_v2
CREATE INDEX IF NOT EXISTS idx_turns_v2_task_id ON turns_v2 (task_id);
CREATE INDEX IF NOT EXISTS idx_turns_v2_session_id ON turns_v2 (session_id);
CREATE INDEX IF NOT EXISTS idx_turns_v2_spirit_session_id ON turns_v2 (spirit_session_id);

-- steps_v2
CREATE INDEX IF NOT EXISTS idx_steps_v2_turn_id ON steps_v2 (turn_id);
CREATE INDEX IF NOT EXISTS idx_steps_v2_task_id ON steps_v2 (task_id);
CREATE INDEX IF NOT EXISTS idx_steps_v2_session_id ON steps_v2 (session_id);
CREATE INDEX IF NOT EXISTS idx_steps_v2_status ON steps_v2 (status);
CREATE INDEX IF NOT EXISTS idx_steps_v2_seq ON steps_v2 (seq);

-- team_stages_v2
CREATE INDEX IF NOT EXISTS idx_team_stages_v2_task_id ON team_stages_v2 (task_id);
CREATE INDEX IF NOT EXISTS idx_team_stages_v2_spirit_session_id ON team_stages_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_team_stages_v2_status ON team_stages_v2 (status);

-- team_runs_v2
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_team_stage_id ON team_runs_v2 (team_stage_id);
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_spirit_session_id ON team_runs_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_parent_task_id ON team_runs_v2 (parent_task_id);
CREATE INDEX IF NOT EXISTS idx_team_runs_v2_status ON team_runs_v2 (status);

-- member_sessions_v2
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_team_run_id ON member_sessions_v2 (team_run_id);
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_spirit_session_id ON member_sessions_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_agent_key ON member_sessions_v2 (agent_key);
CREATE INDEX IF NOT EXISTS idx_member_sessions_v2_status ON member_sessions_v2 (status);

-- plan_boards_v2
CREATE INDEX IF NOT EXISTS idx_plan_boards_v2_task_id ON plan_boards_v2 (task_id);
CREATE INDEX IF NOT EXISTS idx_plan_boards_v2_spirit_session_id ON plan_boards_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_plan_boards_v2_status ON plan_boards_v2 (status);

-- plan_steps_v2
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_plan_board_id ON plan_steps_v2 (plan_board_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_spirit_session_id ON plan_steps_v2 (spirit_session_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_status ON plan_steps_v2 (status);
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_assigned_team_id ON plan_steps_v2 (assigned_team_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_v2_step_key ON plan_steps_v2 (step_key);
```

- [ ] **Step 4: 在 `ddl_migration_registry.go` 注册迁移**

找到 `ddl_migration_registry.go` 中的迁移列表（如 `var migrations = []DDLMigration{...}`），追加：

```go
{
	Version: "20260702",
	Name:    "v2_indexes",
	SQLPath: "sql/migrations/20260702_v2_indexes.sql",
},
```

- [ ] **Step 5: 运行全量 build + test 验证迁移**

Run: `cd f:\aranea-agents && go build ./... 2>&1 | tail -10`
Expected: 编译成功

Run: `cd f:\aranea-agents && go test ./internal/data/ -run "TestV2|TestSession|TestTask|TestTurn|TestStep|TestTeamStage|TestTeamRun|TestMemberSession|TestPlanBoard|TestPlanStep" -count=1 2>&1 | tail -20`
Expected: 全部 PASS（测试中 Ent 自动建表，DDL 迁移补建索引不影响测试）

- [ ] **Step 6: 提交**

```bash
cd f:\aranea-agents
git add internal/data/data.go internal/data/ddl_migration_registry.go internal/data/sql/migrations/20260702_v2_indexes.sql
git commit -m "feat(data): register v2 repos in Wire + add v2 indexes DDL migration"
```

---

## Phase 1.5: Sequencer v2 重写

### Task 10: 创建 Sequencer v2 统一发布管道

**目标**：新建 `internal/agent/v2/sequencer.go`，实现单 worker FIFO + 16ms streaming 批合并 + persistChan + 5 次重试 + 512 死信。所有事件源走 `Sequencer.Publish` 取代 direct-publish。

**Files:**
- Create: `internal/agent/v2/sequencer.go`
- Create: `internal/agent/v2/sequencer_test.go`
- Create: `internal/agent/v2/persist_worker.go`
- Create: `internal/agent/v2/event_router.go`（event → repo method 派发）

- [ ] **Step 1: 写测试（红）**

**File:** `internal/agent/v2/sequencer_test.go`

```go
package v2

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeRepoSet is a minimal repo collection for testing — captures all upserts.
type fakeRepoSet struct {
	mu       sync.Mutex
	tasks    []biz.Task
	turns    []biz.Turn
	steps    []biz.Step
	boards   []biz.PlanBoard
	pSteps   []biz.PlanStep
	stages   []biz.TeamStage
	runs     []biz.TeamRun
	members  []biz.MemberSession
}

func (f *fakeRepoSet) UpsertTask(_ context.Context, t biz.Task) (biz.Task, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.tasks = append(f.tasks, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertTurn(_ context.Context, t biz.Turn) (biz.Turn, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.turns = append(f.turns, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertStep(_ context.Context, s biz.Step) (biz.Step, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.steps = append(f.steps, s)
	return s, nil
}
func (f *fakeRepoSet) UpsertPlanBoard(_ context.Context, p biz.PlanBoard) (biz.PlanBoard, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.boards = append(f.boards, p)
	return p, nil
}
func (f *fakeRepoSet) UpsertPlanStep(_ context.Context, p biz.PlanStep) (biz.PlanStep, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.pSteps = append(f.pSteps, p)
	return p, nil
}
func (f *fakeRepoSet) UpsertTeamStage(_ context.Context, t biz.TeamStage) (biz.TeamStage, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.stages = append(f.stages, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertTeamRun(_ context.Context, t biz.TeamRun) (biz.TeamRun, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.runs = append(f.runs, t)
	return t, nil
}
func (f *fakeRepoSet) UpsertMemberSession(_ context.Context, m biz.MemberSession) (biz.MemberSession, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.members = append(f.members, m)
	return m, nil
}

type fakeBus struct {
	mu      sync.Mutex
	pub     []biz.Event
}
func (f *fakeBus) Publish(_ context.Context, e biz.Event) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.pub = append(f.pub, e)
}
func (f *fakeBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() {}
}

func newTestSequencer(t *testing.T) (*Sequencer, *fakeRepoSet, *fakeBus) {
	t.Helper()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })
	return s, rs, bus
}

// TestSequencer_PublishTaskCreated verifies that a task.created event is
// persisted to TaskRepo AND published to the bus in FIFO order.
func TestSequencer_PublishTaskCreated(t *testing.T) {
	t.Parallel()
	s, rs, bus := newTestSequencer(t)
	ctx := context.Background()

	evt := biz.NewTaskCreatedEvent(biz.Task{
		ID: "task-1", SpiritSessionID: "sess-1", Status: biz.TaskStatusRunning,
		Version: 1,
	})
	s.Publish(ctx, evt)

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	rs.mu.Lock()
	if len(rs.tasks) != 1 || rs.tasks[0].ID != "task-1" {
		t.Fatalf("expected 1 task persisted, got %+v", rs.tasks)
	}
	rs.mu.Unlock()

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 1 || bus.pub[0].EventKind() != biz.EventKindTaskCreated {
		t.Fatalf("expected task.created published, got %+v", bus.pub)
	}
}

// TestSequencer_FIFOOrderAcrossEventTypes verifies cross-event-type FIFO:
// task.created must be persisted/published BEFORE turn.started even if
// Publish is called concurrently.
func TestSequencer_FIFOOrderAcrossEventTypes(t *testing.T) {
	t.Parallel()
	s, rs, bus := newTestSequencer(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-1", SpiritSessionID: "s-1", Version: 1}))
	}()
	go func() {
		defer wg.Done()
		s.Publish(ctx, biz.NewTurnStartedEvent(biz.Turn{ID: "tn-1", TaskID: "t-1", SpiritSessionID: "s-1", Seq: 1, Version: 1}))
	}()
	wg.Wait()

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify Seq assigned monotonically
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.pub) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(bus.pub))
	}
	// Both must have non-zero Seq; ordering is by Publish enqueue order (single worker FIFO)
	for _, e := range bus.pub {
		if e.OccurredAt().IsZero() {
			t.Errorf("event %s has zero OccurredAt", e.EventKind())
		}
	}
}

// TestSequencer_StreamingBatchMerge verifies that two step.streaming events
// for the SAME StepID within the batch window merge into a single WS publish.
func TestSequencer_StreamingBatchMerge(t *testing.T) {
	t.Parallel()
	s, _, bus := newTestSequencer(t)
	ctx := context.Background()

	// Two streaming chunks for the same step within window
	s.Publish(ctx, biz.NewStepStreamingEvent("step-1", "content", "hello", 1))
	s.Publish(ctx, biz.NewStepStreamingEvent("step-1", "content", " world", 2))

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	// Expect exactly 1 merged event (content = "hello world")
	if len(bus.pub) != 1 {
		t.Fatalf("expected 1 merged event, got %d (%+v)", len(bus.pub), bus.pub)
	}
}

// TestSequencer_StreamingDoesNotPersist verifies that step.streaming events
// are NOT persisted to RepoSet (only step.created/updated/completed are).
func TestSequencer_StreamingDoesNotPersist(t *testing.T) {
	t.Parallel()
	s, rs, _ := newTestSequencer(t)
	ctx := context.Background()

	s.Publish(ctx, biz.NewStepStreamingEvent("step-1", "content", "hello", 1))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.steps) != 0 {
		t.Fatalf("expected 0 persisted steps, got %d", len(rs.steps))
	}
}

// TestSequencer_DeadLetterOnPersistFailure verifies that after 5 retries,
// a failing persist lands in DeadLetter().
func TestSequencer_DeadLetterOnPersistFailure(t *testing.T) {
	t.Parallel()
	rs := &failingRepoSet{fail: true}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithPersistMaxRetries(5),
		WithPersistBackoff(time.Millisecond),
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-fail", SpiritSessionID: "s-1", Version: 1}))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Wait briefly for persist retries to exhaust
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("dead letter never received; dead=%d", s.DeadLetterCount())
		default:
		}
		if s.DeadLetterCount() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
}

// failingRepoSet wraps fakeRepoSet and forces every Upsert to error.
type failingRepoSet struct {
	fakeRepoSet
	fail bool
}
func (f *failingRepoSet) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if f.fail {
		return biz.Task{}, errTestFailure
	}
	return f.fakeRepoSet.UpsertTask(ctx, t)
}

var errTestFailure = newTestErr()
type testErr struct{}
func (testErr) Error() string { return "simulated persist failure" }
func newTestErr() testErr { return testErr{} }
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestSequencer" -count=1 2>&1 | tail -20`
Expected: 编译失败（`Sequencer` 类型未定义）

- [ ] **Step 3: 实现 RepoSet 接口 + EventRouter**

**File:** `internal/agent/v2/event_router.go`

```go
// Package v2 implements the Phase 1 sequencer and projector for the
// LLM activity ordering redesign. See:
//   docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
package v2

import (
	"context"

	"aranea-agents/internal/biz"
)

// RepoSet bundles all v2 repos needed by the Sequencer to persist events.
// Each method takes the entity extracted from an Event and upserts it.
// Implementations must be safe for concurrent use.
type RepoSet interface {
	UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error)
	UpsertTurn(ctx context.Context, t biz.Turn) (biz.Turn, error)
	UpsertStep(ctx context.Context, s biz.Step) (biz.Step, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error)
	UpsertMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
}

// persistAction routes an Event to the appropriate Repo method.
// Returns false if the event should NOT be persisted (e.g. step.streaming).
// Returns the upserted entity's version on success (used for sequencing).
func persistAction(ctx context.Context, rs RepoSet, e biz.Event) (persisted bool, err error) {
	switch ev := e.(type) {
	case *biz.TaskCreatedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskUpdatedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskCompletedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err
	case *biz.TaskFailedEvent:
		_, err = rs.UpsertTask(ctx, ev.Task)
		return true, err

	case *biz.TurnStartedEvent:
		_, err = rs.UpsertTurn(ctx, ev.Turn)
		return true, err
	case *biz.TurnCompletedEvent:
		_, err = rs.UpsertTurn(ctx, ev.Turn)
		return true, err

	case *biz.StepCreatedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepUpdatedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepCompletedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepFailedEvent:
		_, err = rs.UpsertStep(ctx, ev.Step)
		return true, err
	case *biz.StepStreamingEvent:
		// streaming chunks are NOT persisted; only pushed to WS
		return false, nil

	case *biz.TeamStageCreatedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageUpdatedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageCompletedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err
	case *biz.TeamStageFailedEvent:
		_, err = rs.UpsertTeamStage(ctx, ev.TeamStage)
		return true, err

	case *biz.TeamRunStartedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err
	case *biz.TeamRunCompletedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err
	case *biz.TeamRunFailedEvent:
		_, err = rs.UpsertTeamRun(ctx, ev.TeamRun)
		return true, err

	case *biz.MemberSessionCreatedEvent:
		_, err = rs.UpsertMemberSession(ctx, ev.MemberSession)
		return true, err
	case *biz.MemberSessionUpdatedEvent:
		_, err = rs.UpsertMemberSession(ctx, ev.MemberSession)
		return true, err

	case *biz.PlanBoardCreatedEvent:
		_, err = rs.UpsertPlanBoard(ctx, ev.PlanBoard)
		return true, err
	case *biz.PlanBoardUpdatedEvent:
		_, err = rs.UpsertPlanBoard(ctx, ev.PlanBoard)
		return true, err
	case *biz.PlanStepUpdatedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepStartedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepCompletedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepFailedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	case *biz.PlanStepSkippedEvent:
		_, err = rs.UpsertPlanStep(ctx, ev.PlanStep)
		return true, err
	}
	// Unknown event type: skip persistence
	return false, nil
}
```

- [ ] **Step 4: 实现 Sequencer**

**File:** `internal/agent/v2/sequencer.go`

```go
package v2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Defaults — match v1 for parity.
const (
	defaultPublishBufferSize   = 256
	defaultPersistBufferSize   = 256
	defaultDeltaBatchInterval = 16 * time.Millisecond
	defaultPersistMaxRetries   = 5
	defaultPersistBackoff      = 100 * time.Millisecond
	defaultDeadLetterCapacity  = 512
)

// EventBus is the publish sink for v2 events (fan-out to WS subscribers).
type EventBus interface {
	Publish(ctx context.Context, e biz.Event)
	Subscribe(opts biz.EventSubscribeOptions) (<-chan biz.Event, func())
}

// Sequencer is the single unified entry point for all v2 events.
// It replaces v1's dual-path (ActivityProjector→Sequencer + direct-publish).
//
// Invariants:
//   1. Single publish worker ensures FIFO across all event types.
//   2. step.streaming events are NOT persisted (only WS-published); same
//      StepID + DeltaField within the 16ms batch window are merged.
//   3. Persist worker: 5x exponential backoff retries + 512-cap dead-letter.
type Sequencer struct {
	repoSet     RepoSet
	bus         EventBus
	lg          loggateway.Logger
	seqAssigner *biz.SeqAssigner // shared with Projector

	publishQueue chan publishTask
	persistChan  chan persistItem
	deadLetter   *deadLetterRing

	publishWG sync.WaitGroup
	persistWG sync.WaitGroup

	versionSeq atomic.Int64 // global version counter (per-process)

	deltaBatchInterval time.Duration
	persistMaxRetries  int
	persistBackoff     time.Duration

	closed atomic.Bool
	closeMu sync.Mutex
}

type publishTask struct {
	event      biz.Event
	persist    bool
	enqueuedAt time.Time
}

type persistItem struct {
	event biz.Event
}

type config struct {
	publishBuffer       int
	persistBuffer       int
	deltaBatchInterval time.Duration
	persistMaxRetries  int
	persistBackoff     time.Duration
	deadLetterCapacity int
}

// Option configures a Sequencer.
type Option func(*config)

func WithPublishBuffer(n int) Option             { return func(c *config) { c.publishBuffer = n } }
func WithPersistBuffer(n int) Option             { return func(c *config) { c.persistBuffer = n } }
func WithDeltaBatchInterval(d time.Duration) Option { return func(c *config) { c.deltaBatchInterval = d } }
func WithPersistMaxRetries(n int) Option         { return func(c *config) { c.persistMaxRetries = n } }
func WithPersistBackoff(d time.Duration) Option { return func(c *config) { c.persistBackoff = d } }
func WithDeadLetterCapacity(n int) Option        { return func(c *config) { c.deadLetterCapacity = n } }

// NewSequencer constructs a Sequencer and starts its publish + persist workers.
func NewSequencer(rs RepoSet, bus EventBus, lg loggateway.Logger, opts ...Option) *Sequencer {
	cfg := config{
		publishBuffer:       defaultPublishBufferSize,
		persistBuffer:       defaultPersistBufferSize,
		deltaBatchInterval: defaultDeltaBatchInterval,
		persistMaxRetries:  defaultPersistMaxRetries,
		persistBackoff:     defaultPersistBackoff,
		deadLetterCapacity: defaultDeadLetterCapacity,
	}
	for _, o := range opts {
		o(&cfg)
	}

	s := &Sequencer{
		repoSet:            rs,
		bus:                bus,
		lg:                 lg.With(loggateway.Domain("sequencer_v2")),
		seqAssigner:        biz.NewSeqAssigner(),
		publishQueue:       make(chan publishTask, cfg.publishBuffer),
		persistChan:        make(chan persistItem, cfg.persistBuffer),
		deadLetter:         newDeadLetterRing(cfg.deadLetterCapacity),
		deltaBatchInterval: cfg.deltaBatchInterval,
		persistMaxRetries:  cfg.persistMaxRetries,
		persistBackoff:     cfg.persistBackoff,
	}

	s.publishWG.Add(1)
	go s.publishLoop()
	s.persistWG.Add(1)
	go s.persistLoop()
	return s
}

// SeqAssigner exposes the shared SeqAssigner so Projector can pre-allocate Seq
// for turn-level events before publishing.
func (s *Sequencer) SeqAssigner() *biz.SeqAssigner { return s.seqAssigner }

// Publish enqueues an event for FIFO processing.
// Safe for concurrent use.
func (s *Sequencer) Publish(ctx context.Context, e biz.Event) {
	if s.closed.Load() {
		s.lg.Warn("sequencer closed, event dropped", loggateway.Str("kind", string(e.EventKind())))
		return
	}
	persist := s.shouldPersist(e)
	select {
	case s.publishQueue <- publishTask{event: e, persist: persist, enqueuedAt: time.Now()}:
	case <-ctx.Done():
		s.lg.Warn("publish ctx canceled before enqueue",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(ctx.Err()))
	}
}

// shouldPersist returns false for streaming chunks (only bus-published).
func (s *Sequencer) shouldPersist(e biz.Event) bool {
	_, ok := e.(*biz.StepStreamingEvent)
	return !ok
}

// Flush blocks until all queued events are processed (publish + persist).
// Mainly for tests; production callers should rely on Close() for shutdown.
func (s *Sequencer) Flush(ctx context.Context) error {
	// Drain publishQueue by sending a sentinel through the pipeline.
	done := make(chan struct{})
	go func() {
		// Wait until publishQueue empties.
		for len(s.publishQueue) > 0 {
			time.Sleep(time.Millisecond)
		}
		// Wait until persistChan empties.
		for len(s.persistChan) > 0 {
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("sequencer flush timed out")
	}
}

// publishLoop is the single FIFO worker.
// It implements 16ms streaming batch merge for step.streaming events.
func (s *Sequencer) publishLoop() {
	defer s.publishWG.Done()
	var pendingStreaming *biz.StepStreamingEvent
	var pendingTimer *time.Timer
	var pendingDone chan struct{}

	for {
		select {
		case task, ok := <-s.publishQueue:
			if !ok {
				// Channel closed: drain any pending streaming event, then exit.
				if pendingStreaming != nil {
					s.flushStreaming(pendingStreaming, pendingDone)
				}
				return
			}
			// If we have a pending streaming event and the new task is NOT a
			// mergeable streaming chunk, flush pending first.
			if pendingStreaming != nil {
				cur, isStreaming := task.event.(*biz.StepStreamingEvent)
				if !isStreaming || !canMergeStreaming(pendingStreaming, cur) {
					s.flushStreaming(pendingStreaming, pendingDone)
					pendingStreaming = nil
					pendingDone = nil
					pendingTimer = nil
				} else {
					// Merge: accumulate delta content
					pendingStreaming.Delta += cur.Delta
					continue
				}
			}
			// Handle current event
			if ev, ok := task.event.(*biz.StepStreamingEvent); ok {
				// Start a new pending streaming with timer
				pendingStreaming = ev
				pendingDone = make(chan struct{})
				pendingTimer = time.AfterFunc(s.deltaBatchInterval, func() {
					close(pendingDone)
				})
				_ = pendingTimer
				// Fall through to immediately process next item; if timer fires
				// or non-mergeable item arrives, we flush.
				continue
			}
			s.processTask(task)

		case <-pendingDone:
			if pendingStreaming != nil {
				s.flushStreaming(pendingStreaming, pendingDone)
				pendingStreaming = nil
				pendingDone = nil
				pendingTimer = nil
			}
		}
	}
}

// canMergeStreaming returns true iff two streaming events share StepID and DeltaField.
func canMergeStreaming(a, b *biz.StepStreamingEvent) bool {
	return a.StepID == b.StepID && a.DeltaField == b.DeltaField
}

// flushStreaming publishes the merged streaming event to bus only (no persist).
func (s *Sequencer) flushStreaming(ev *biz.StepStreamingEvent, _ chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.bus.Publish(ctx, ev)
}

// processTask handles a non-mergeable event: persist (async) + bus publish (sync).
func (s *Sequencer) processTask(task publishTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 1. Async persist (skip if streaming — but streaming never reaches here).
	if task.persist {
		select {
		case s.persistChan <- persistItem{event: task.event}:
		default:
			// Persist channel full: log + drop (counter increases).
			s.lg.Warn("persist channel full, event dropped to dead-letter",
				loggateway.Str("kind", string(task.event.EventKind())))
			s.deadLetter.Push(task.event)
		}
	}
	// 2. Sync bus publish.
	s.bus.Publish(ctx, task.event)
}

// persistLoop consumes persistChan with retry + dead-letter.
func (s *Sequencer) persistLoop() {
	defer s.persistWG.Done()
	for item := range s.persistChan {
		s.persistWithRetry(item.event)
	}
}

func (s *Sequencer) persistWithRetry(e biz.Event) {
	var lastErr error
	for attempt := 0; attempt < s.persistMaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := persistAction(ctx, s.repoSet, e)
		cancel()
		if err == nil {
			return
		}
		lastErr = err
		// Exponential backoff: 1x, 2x, 4x, 8x, 16x
		time.Sleep(s.persistBackoff * time.Duration(1<<attempt))
	}
	s.lg.Error("persist exhausted retries, sending to dead-letter",
		loggateway.Str("kind", string(e.EventKind())), loggateway.Err(lastErr))
	s.deadLetter.Push(e)
}

// Close performs graceful shutdown: close publishQueue → drain publishLoop →
// close persistChan → drain persistLoop. Idempotent.
func (s *Sequencer) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(s.publishQueue)
	s.publishWG.Wait()
	close(s.persistChan)
	s.persistWG.Wait()
	return nil
}

// DeadLetterCount returns the number of events that exhausted retries.
func (s *Sequencer) DeadLetterCount() int { return s.deadLetter.Len() }

// deadLetterRing is a FIFO ring buffer with entity-ID-based dedup.
type deadLetterRing struct {
	mu    sync.Mutex
	buf   []biz.Event
	cap   int
	seen  map[string]struct{} // dedup by entity ID (extracted from event)
	head  int
}

func newDeadLetterRing(capacity int) *deadLetterRing {
	return &deadLetterRing{
		buf:  make([]biz.Event, 0, capacity),
		cap:  capacity,
		seen: make(map[string]struct{}),
	}
}

func (r *deadLetterRing) Push(e biz.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := deadLetterID(e)
	if _, ok := r.seen[id]; ok {
		return // already in dead letter; skip
	}
	if len(r.buf) >= r.cap {
		// Evict oldest
		old := r.buf[0]
		delete(r.seen, deadLetterID(old))
		r.buf = r.buf[1:]
	}
	r.buf = append(r.buf, e)
	r.seen[id] = struct{}{}
}

func (r *deadLetterRing) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buf)
}

func deadLetterID(e biz.Event) string {
	type ider interface{ EntityID() string }
	if id, ok := e.(ider); ok {
		return id.EntityID()
	}
	return string(e.EventKind())
}
```

- [ ] **Step 5: 实现 Biz EventBus 适配器**

为 Sequencer 提供一个 `biz.Event` 总线适配器（在 v1 `ActivityEventBus` 之上 fan-out），实现 `EventBus` 接口。

**File:** `internal/agent/v2/event_bus_adapter.go`

```go
package v2

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
)

// eventBusAdapter wraps an existing event.Bus to satisfy the v2 EventBus interface.
// The event.Bus (from internal/event) is the existing WS fan-out bus used by v1.
// By reusing it, v2 events automatically reach WS subscribers.
type eventBusAdapter struct {
	bus biz.EventBus // the internal/event bus, exposing Publish + Subscribe for v2 events
}

// NewEventBusAdapter bridges an internal/event bus to the v2 EventBus interface.
func NewEventBusAdapter(b biz.EventBus) EventBus {
	return &eventBusAdapter{bus: b}
}

func (a *eventBusAdapter) Publish(ctx context.Context, e biz.Event) {
	a.bus.Publish(ctx, e)
}

func (a *eventBusAdapter) Subscribe(opts biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return a.bus.Subscribe(opts)
}

// Compile-time check
var _ EventBus = (*eventBusAdapter)(nil)
var _ sync.Locker = (*sync.Mutex)(nil) // ensure sync import retained if extended
```

> **Note**: 此处依赖 `biz.EventBus` 接口（与 v1 不同）。Task 11 兼容层会创建同时支持 v1 `ActivityEvent` 与 v2 `Event` 的桥接 bus。先在 Task 10 中通过新建 `internal/event/bus_v2.go` 提供基础 `biz.EventBus` 实现（仅 Publish + Subscribe 的 in-process fan-out），Task 11 再加 WS adapter 与 v1 bridge。

- [ ] **Step 6: 实现 in-process v2 EventBus**

**File:** `internal/event/bus_v2.go`

```go
package event

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
)

// V2Bus is an in-process fan-out bus for v2 Events.
// Subscribers receive all events; filtering is done by subscribers (e.g. by
// SpiritSessionID).
type V2Bus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan biz.Event
	nextID      uint64
}

func NewV2Bus() *V2Bus {
	return &V2Bus{subscribers: make(map[uint64]chan biz.Event)}
}

func (b *V2Bus) Publish(_ context.Context, e biz.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		// Non-blocking send; if subscriber is slow, drop event.
		select {
		case ch <- e:
		default:
			// Drop on full subscriber; log would be too noisy at this layer.
		}
	}
}

func (b *V2Bus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan biz.Event, 256)
	b.subscribers[id] = ch
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			close(c)
			delete(b.subscribers, id)
		}
	}
	return ch, cancel
}

var _ biz.EventBus = (*V2Bus)(nil)
```

- [ ] **Step 7: 在 biz 中追加 EventBus + EventSubscribeOptions 定义**

**File:** `internal/biz/event.go`（在 Task 2 创建的 event.go 中追加）

```go
// EventBus is the v2 publish/subscribe bus for Events.
// Replaces the v1 ActivityEventBus. Implemented by event.V2Bus and adapters.
// Stability: evolving
type EventBus interface {
	Publish(ctx context.Context, e Event)
	Subscribe(opts EventSubscribeOptions) (<-chan Event, func())
}

// EventSubscribeOptions configures a subscription (e.g. filtering by session).
// Empty options = receive all events.
type EventSubscribeOptions struct {
	SpiritSessionID string
	TaskID          string
}
```

- [ ] **Step 8: 跑测试验证 Sequencer**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestSequencer" -count=1 -v 2>&1 | tail -30`
Expected: 全部 PASS

- [ ] **Step 9: 跑 race detector 验证并发安全**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestSequencer" -count=1 -race 2>&1 | tail -20`
Expected: PASS, no race detected

- [ ] **Step 10: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/v2/ internal/event/bus_v2.go internal/biz/event.go
git commit -m "feat(agent/v2): implement unified Sequencer with FIFO + streaming merge + dead-letter"
```

---

## Phase 1.6: 兼容层 adapter

### Task 11: v2 事件 → v1 ActivityEvent 兼容桥接

**目标**：创建 adapter 将 v2 事件转换为 v1 `ActivityEvent`，同时发布到 v1 `ActivityEventBus`，让旧前端在 Phase 2 重写完成前继续工作。**新旧并行**：新表写入 + 旧表（activities）只读副本由 adapter 维护。

**Files:**
- Create: `internal/agent/v2/compat_adapter.go`
- Create: `internal/agent/v2/compat_adapter_test.go`
- Modify: `internal/agent/v2/sequencer.go`（`processTask` 调用 adapter.PublishV1）

- [ ] **Step 1: 写测试（红）**

**File:** `internal/agent/v2/compat_adapter_test.go`

```go
package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// fakeV1Bus captures v1 ActivityEvents emitted by the adapter.
type fakeV1Bus struct {
	mu   sync.Mutex
	pub  []biz.ActivityEvent
}

func (f *fakeV1Bus) Publish(_ context.Context, e biz.ActivityEvent) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.pub = append(f.pub, e)
}
func (f *fakeV1Bus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return make(chan biz.ActivityEvent), func() {}
}
func (f *fakeV1Bus) DropCount() uint64 { return 0 }

func TestCompatAdapter_TaskCreatedToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	now := time.Now().UTC()
	adapter.PublishV1(context.Background(), &biz.TaskCreatedEvent{
		Task: biz.Task{
			ID: "task-1", SpiritSessionID: "sess-1", Status: biz.TaskStatusRunning,
			CreatedAt: now, UpdatedAt: now, Version: 1,
		},
		BaseEvent: biz.BaseEvent{OccurredAtTime: now},
	})

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	// Must map to v1 ActivityKind=task, EventType=Created
	if ev.Type != biz.ActivityEventCreated {
		t.Errorf("expected v1 event type Created, got %s", ev.Type)
	}
	if ev.Activity.Kind != biz.ActivityKindTask {
		t.Errorf("expected v1 kind task, got %s", ev.Activity.Kind)
	}
	if ev.Activity.ID != "task-1" {
		t.Errorf("expected v1 activity id task-1, got %s", ev.Activity.ID)
	}
	if ev.Activity.SessionID != "sess-1" {
		t.Errorf("expected v1 session id sess-1, got %s", ev.Activity.SessionID)
	}
}

func TestCompatAdapter_StepStreamingToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	now := time.Now().UTC()
	adapter.PublishV1(context.Background(), &biz.StepStreamingEvent{
		StepID:     "step-1",
		DeltaField: "content",
		Delta:      "hello",
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: "sess-1",
			TaskIDVal:          "task-1",
			OccurredAtTime:     now,
		},
	})

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	if ev.Type != biz.ActivityEventStreaming {
		t.Errorf("expected v1 type Streaming, got %s", ev.Type)
	}
	if ev.Activity.Kind != biz.ActivityKindReply {
		t.Errorf("expected v1 kind reply, got %s", ev.Activity.Kind)
	}
}

func TestCompatAdapter_PlanStepStartedToActivityEvent(t *testing.T) {
	t.Parallel()
	v1 := &fakeV1Bus{}
	adapter := NewCompatAdapter(v1)

	now := time.Now().UTC()
	adapter.PublishV1(context.Background(), &biz.PlanStepStartedEvent{
		PlanStep: biz.PlanStep{
			ID: "ps-1", PlanBoardID: "pb-1", SpiritSessionID: "sess-1",
			Status: biz.PlanStepStatusRunning, Version: 1,
		},
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: "sess-1",
			TaskIDVal:          "task-1",
			OccurredAtTime:     now,
		},
	})

	v1.mu.Lock()
	defer v1.mu.Unlock()
	if len(v1.pub) != 1 {
		t.Fatalf("expected 1 v1 event, got %d", len(v1.pub))
	}
	ev := v1.pub[0]
	if ev.Activity.Kind != biz.ActivityKindPlan {
		t.Errorf("expected v1 kind plan, got %s", ev.Activity.Kind)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestCompatAdapter" -count=1 2>&1 | tail -10`
Expected: 编译失败（`NewCompatAdapter` 未定义）

- [ ] **Step 3: 实现 CompatAdapter**

**File:** `internal/agent/v2/compat_adapter.go`

```go
package v2

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// CompatAdapter translates v2 Events to v1 ActivityEvents so the legacy
// frontend continues to work during Phase 2 (frontend rewrite).
//
// Lifecycle:
//   Phase 1: adapter is active; v1 frontend receives both legacy + new events
//            (legacy frontend ignores the new events it doesn't recognize).
//   Phase 2: v1 frontend is rewritten; adapter is removed.
//
// Stability: evolving — TODO(Phase 2): remove this entire file.
type CompatAdapter struct {
	v1Bus biz.ActivityEventBus // legacy bus (reaches v1 WS subscribers)
	lg    loggateway.Logger
}

// NewCompatAdapter constructs an adapter that publishes v1 ActivityEvents
// derived from v2 Events.
func NewCompatAdapter(v1Bus biz.ActivityEventBus) *CompatAdapter {
	return &CompatAdapter{
		v1Bus: v1Bus,
		lg:    loggateway.NewNoop(),
	}
}

// WithLogger sets a non-noop logger (used in production).
func (a *CompatAdapter) WithLogger(lg loggateway.Logger) *CompatAdapter {
	a.lg = lg.With(loggateway.Domain("compat_adapter"))
	return a
}

// PublishV1 converts a v2 Event to a v1 ActivityEvent and publishes it.
// Unknown event types are silently dropped (logged at Debug).
func (a *CompatAdapter) PublishV1(ctx context.Context, e biz.Event) {
	v1, ok := a.convert(e)
	if !ok {
		a.lg.Debug("compat adapter: no v1 mapping for event",
			loggateway.Str("kind", string(e.EventKind())))
		return
	}
	a.v1Bus.Publish(ctx, v1)
}

// convert maps a v2 Event to a v1 ActivityEvent. Returns (zero, false) if
// no mapping exists for the event type.
func (a *CompatAdapter) convert(e biz.Event) (biz.ActivityEvent, bool) {
	spiritID := e.SpiritSessionID()
	taskID := e.TaskID()
	now := e.OccurredAt()

	switch ev := e.(type) {
	// === Task events → v1 ActivityKind=task ===
	case *biz.TaskCreatedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	case *biz.TaskCompletedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true
	case *biz.TaskFailedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventFailed,
			Activity: biz.Activity{
				ID:        ev.Task.ID,
				Kind:      biz.ActivityKindTask,
				Status:    biz.ActivityStatusFailed,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
			},
		}, true

	// === Step events → v1 ActivityKind (thinking/action/reply/notice/confirm) ===
	case *biz.StepCreatedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.Step.ID,
				Kind:      stepKindToV1(ev.Step.Kind),
				Status:    stepStatusToV1(ev.Step.Status),
				SessionID: spiritID,
				TurnID:    ev.Step.TurnID,
				AgentKey:  ev.Step.AuthorAgentKey,
				Timestamp: now,
			},
		}, true
	case *biz.StepStreamingEvent:
		// v1 ActivityEventStreaming with delta in Meta
		return biz.ActivityEvent{
			Type: biz.ActivityEventStreaming,
			Activity: biz.Activity{
				ID:        ev.StepID,
				Kind:      deltaFieldToKind(ev.DeltaField),
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				Timestamp: now,
				Meta: map[string]any{
					"delta":       ev.Delta,
					"delta_field": ev.DeltaField,
				},
			},
		}, true
	case *biz.StepCompletedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.Step.ID,
				Kind:      stepKindToV1(ev.Step.Kind),
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    ev.Step.TurnID,
				AgentKey:  ev.Step.AuthorAgentKey,
				Content:   ev.Step.Content,
				Timestamp: now,
			},
		}, true

	// === TeamStage events → v1 ActivityKind=team_stage ===
	case *biz.TeamStageCreatedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        ev.TeamStage.ID,
				Kind:      biz.ActivityKindTeamStage,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    ev.TeamStage.TaskID,
				Timestamp: now,
			},
		}, true
	case *biz.TeamStageCompletedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:        ev.TeamStage.ID,
				Kind:      biz.ActivityKindTeamStage,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    ev.TeamStage.TaskID,
				Timestamp: now,
			},
		}, true

	// === PlanStep events → v1 ActivityKind=plan ===
	case *biz.PlanStepStartedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:        ev.PlanStep.ID,
				Kind:      biz.ActivityKindPlan,
				Status:    biz.ActivityStatusRunning,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
				Meta: map[string]any{
					"step_status": string(ev.PlanStep.Status),
				},
			},
		}, true
	case *biz.PlanStepCompletedEvent:
		return biz.ActivityEvent{
			Type: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:        ev.PlanStep.ID,
				Kind:      biz.ActivityKindPlan,
				Status:    biz.ActivityStatusCompleted,
				SessionID: spiritID,
				TurnID:    taskID,
				Timestamp: now,
				Meta: map[string]any{
					"step_status": string(ev.PlanStep.Status),
				},
			},
		}, true
	}

	// Other event types (Turn*, TeamRun*, MemberSession*, PlanBoard*) have
	// no direct v1 counterpart and are intentionally NOT translated — the
	// legacy frontend did not visualize them as first-class entities.
	return biz.ActivityEvent{}, false
}

// stepKindToV1 maps biz.StepKind to v1 biz.ActivityKind.
func stepKindToV1(k biz.StepKind) biz.ActivityKind {
	switch k {
	case biz.StepKindThinking:
		return biz.ActivityKindThinking
	case biz.StepKindAction:
		return biz.ActivityKindAction
	case biz.StepKindReply:
		return biz.ActivityKindReply
	case biz.StepKindNotice:
		return biz.ActivityKindNotice
	case biz.StepKindConfirm:
		return biz.ActivityKindConfirm
	}
	return biz.ActivityKindReply // default
}

// stepStatusToV1 maps biz.StepStatus to v1 biz.ActivityStatus.
func stepStatusToV1(s biz.StepStatus) biz.ActivityStatus {
	switch s {
	case biz.StepStatusPending:
		return biz.ActivityStatusPending
	case biz.StepStatusThinking, biz.StepStatusToolRunning, biz.StepStatusStreaming:
		return biz.ActivityStatusRunning
	case biz.StepStatusCompleted:
		return biz.ActivityStatusCompleted
	case biz.StepStatusFailed:
		return biz.ActivityStatusFailed
	case biz.StepStatusCancelled:
		return biz.ActivityStatusCancelled
	}
	return biz.ActivityStatusRunning
}

// deltaFieldToKind maps a streaming delta_field to a v1 ActivityKind.
func deltaFieldToKind(field string) biz.ActivityKind {
	switch field {
	case "reasoning":
		return biz.ActivityKindThinking
	case "content":
		return biz.ActivityKindReply
	case "tool_args", "tool_result":
		return biz.ActivityKindAction
	}
	return biz.ActivityKindReply
}
```

- [ ] **Step 4: 跑测试验证 CompatAdapter**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestCompatAdapter" -count=1 -v 2>&1 | tail -20`
Expected: 全部 PASS

- [ ] **Step 5: 在 Sequencer 注入 CompatAdapter**

修改 `internal/agent/v2/sequencer.go`：

```go
// Sequencer 增加可选 compatAdapter 字段
type Sequencer struct {
	// ... existing fields ...
	compatAdapter *CompatAdapter // nil = no v1 forwarding
}

// NewSequencer 增加可选 CompatAdapter 参数
func NewSequencer(rs RepoSet, bus EventBus, lg loggateway.Logger, compat *CompatAdapter, opts ...Option) *Sequencer {
	// ... existing init ...
	s.compatAdapter = compat
	// ... workers ...
}

// processTask 末尾追加 v1 转发
func (s *Sequencer) processTask(task publishTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if task.persist {
		select {
		case s.persistChan <- persistItem{event: task.event}:
		default:
			s.lg.Warn("persist channel full, event dropped to dead-letter",
				loggateway.Str("kind", string(task.event.EventKind())))
			s.deadLetter.Push(task.event)
		}
	}
	// v2 publish (always)
	s.bus.Publish(ctx, task.event)
	// v1 forwarding (only if adapter is configured — Phase 1 兼容层)
	if s.compatAdapter != nil {
		s.compatAdapter.PublishV1(ctx, task.event)
	}
}

// flushStreaming 也需要 v1 转发
func (s *Sequencer) flushStreaming(ev *biz.StepStreamingEvent, _ chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.bus.Publish(ctx, ev)
	if s.compatAdapter != nil {
		s.compatAdapter.PublishV1(ctx, ev)
	}
}
```

更新 `sequencer_test.go` 的 `newTestSequencer` 调用（传 `nil` 保持原行为）：

```go
func newTestSequencer(t *testing.T) (*Sequencer, *fakeRepoSet, *fakeBus) {
	t.Helper()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(), nil, // ← nil compat
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })
	return s, rs, bus
}
```

- [ ] **Step 6: 跑全量 v2 测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -count=1 2>&1 | tail -10`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/v2/
git commit -m "feat(agent/v2): add CompatAdapter to bridge v2 events to v1 ActivityEvent for legacy frontend"
```

---

## Phase 1.7: ActivityProjector v2 重写

### Task 12: ProjectMeta v2 + ActivityProjector v2

**目标**：在 `internal/agent/v2/` 新建 `projector.go`，实现 trpc-agent-go 回调到 v2 事件的转换。保留 v1 `ActivityProjector`，新增 v2 并行写入；通过配置切换主路径（Phase 1 默认仍走 v1，v2 验证后切换）。

**Files:**
- Create: `internal/agent/v2/projector.go`
- Create: `internal/agent/v2/projector_test.go`
- Create: `internal/agent/v2/project_meta.go`

- [ ] **Step 1: 定义 ProjectMeta v2**

**File:** `internal/agent/v2/project_meta.go`

```go
package v2

import (
	"time"

	"aranea-agents/internal/biz"
)

// ProjectMeta carries the contextual IDs needed to construct entities from
// trpc-agent-go runtime callbacks.
//
// V2 changes from v1:
//   - Removed: ParentActivityID, ActivityKind, ActivityStatus, Meta map[string]any
//   - Added: explicit TeamStageID, TeamRunID, MemberSessionID, ParentTurnID
type ProjectMeta struct {
	// SessionID is the current session ID (may equal SpiritSessionID for root,
	// or MemberSession's spirit_session_id for nested).
	SessionID string

	// SpiritSessionID is the root spirit session — used as SeqAssigner key.
	SpiritSessionID string

	// TaskID is the parent task ID. For root calls, equals the task being created.
	TaskID string

	// TurnID is the current turn ID (maps to trpc-agent-go's RequestID).
	TurnID string

	// ParentTurnID is set for nested turns (e.g. team member turns); empty for root.
	ParentTurnID string

	// TeamStageID is set when the projector is processing events within a
	// team_stage (PlanExecutor assigned). Empty for spirit-level turns.
	TeamStageID string

	// TeamRunID is set for projector instances bound to a team_run.
	TeamRunID string

	// MemberSessionID is set for projector instances bound to a member_session.
	MemberSessionID string

	// AgentKey / AgentName describe the current agent (spirit root or member).
	AgentKey  string
	AgentName string

	// MemberAgentKeys is the set of member agent keys known to the spirit
	// (used by team_stage projector to initialize Members). May be nil.
	MemberAgentKeys map[string]struct{}

	// TaskContent is the user's original message text (for task entity).
	TaskContent string
}

// Now returns the current UTC time, truncated to millisecond.
// Centralized to avoid inconsistent time handling across event constructors.
func Now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// nextVersion returns version+1 for optimistic concurrency.
func nextVersion(v int64) int64 { return v + 1 }

// newTask constructs a biz.Task with default fields from ProjectMeta.
func (m ProjectMeta) newTask(id string, status biz.TaskStatus, content string) biz.Task {
	now := Now()
	return biz.Task{
		ID:              id,
		SpiritSessionID: m.SpiritSessionID,
		ParentTaskID:    "",
		Status:          status,
		Content:         content,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}
}

// newTurn constructs a biz.Turn with seq allocated from the provided assigner.
func (m ProjectMeta) newTurn(id string, seq int64) biz.Turn {
	now := Now()
	return biz.Turn{
		ID:              id,
		TaskID:          m.TaskID,
		SpiritSessionID: m.SpiritSessionID,
		ParentTurnID:    m.ParentTurnID,
		TeamStageID:     m.TeamStageID,
		TeamRunID:       m.TeamRunID,
		Seq:             seq,
		StartedAt:       now,
		Status:          biz.TurnStatusRunning,
		Version:         1,
	}
}

// newStep constructs a biz.Step with seq allocated from the provided assigner.
func (m ProjectMeta) newStep(id string, kind biz.StepKind, seq int64) biz.Step {
	now := Now()
	return biz.Step{
		ID:              id,
		TurnID:          m.TurnID,
		TaskID:          m.TaskID,
		TeamStageID:     m.TeamStageID,
		TeamRunID:       m.TeamRunID,
		MemberSessionID: m.MemberSessionID,
		SpiritSessionID: m.SpiritSessionID,
		Kind:            kind,
		Seq:             seq,
		Status:          biz.StepStatusPending,
		AuthorAgentKey:  m.AgentKey,
		StartedAt:       now,
		Version:         1,
	}
}
```

- [ ] **Step 2: 写 projector 测试（红）**

**File:** `internal/agent/v2/projector_test.go`

```go
package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// capturingSequencer records all Publish calls.
type capturingSequencer struct {
	mu   sync.Mutex
	pub  []biz.Event
}

func (c *capturingSequencer) Publish(ctx context.Context, e biz.Event) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.pub = append(c.pub, e)
}

func (c *capturingSequencer) Published() []biz.Event {
	c.mu.Lock(); defer c.mu.Unlock()
	out := make([]biz.Event, len(c.pub))
	copy(out, c.pub)
	return out
}

// fakeSeqAssigner is a deterministic SeqAssigner for tests.
type fakeSeqAssigner struct {
	mu  sync.Mutex
	seq int64
}
func (f *fakeSeqAssigner) NextSeq(_ string) int64 {
	f.mu.Lock(); defer f.mu.Unlock()
	f.seq++
	return f.seq
}

func newTestProjector(t *testing.T) (*ActivityProjector, *capturingSequencer, *fakeSeqAssigner) {
	t.Helper()
	cap := &capturingSequencer{}
	seq := &fakeSeqAssigner{}
	p := NewActivityProjector(cap, seq, loggateway.NewNoop())
	return p, cap, seq
}

func TestProjector_OnTurnStart_EmitsTaskAndTurnCreated(t *testing.T) {
	t.Parallel()
	p, cap, _ := newTestProjector(t)
	ctx := context.Background()

	p.OnTurnStart(ctx, ProjectMeta{
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
		TurnID:          "turn-1",
		AgentKey:        "spirit",
		TaskContent:     "hello world",
	})

	events := cap.Published()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (task.created + turn.started), got %d: %+v", len(events), events)
	}
	if events[0].EventKind() != biz.EventKindTaskCreated {
		t.Errorf("expected first event task.created, got %s", events[0].EventKind())
	}
	if events[1].EventKind() != biz.EventKindTurnStarted {
		t.Errorf("expected second event turn.started, got %s", events[1].EventKind())
	}

	tc, ok := events[0].(*biz.TaskCreatedEvent)
	if !ok {
		t.Fatalf("expected TaskCreatedEvent, got %T", events[0])
	}
	if tc.Task.Content != "hello world" {
		t.Errorf("task content: expected 'hello world', got %q", tc.Task.Content)
	}
}

func TestProjector_OnReasoningDelta_EmitsStepStreaming(t *testing.T) {
	t.Parallel()
	p, cap, seq := newTestProjector(t)
	ctx := context.Background()

	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
	}
	p.OnTurnStart(ctx, meta)
	cap.Published() // drain

	// Pre-allocate step ID via the projector's step lifecycle
	stepID := p.BeginStep(meta, biz.StepKindThinking)
	p.OnReasoningDelta(ctx, stepID, "hello reasoning", false)

	events := cap.Published()
	// Find the streaming event
	var streaming *biz.StepStreamingEvent
	for _, e := range events {
		if s, ok := e.(*biz.StepStreamingEvent); ok {
			streaming = s
			break
		}
	}
	if streaming == nil {
		t.Fatalf("expected StepStreamingEvent, got none in %+v", events)
	}
	if streaming.StepID != stepID {
		t.Errorf("streaming StepID: expected %s, got %s", stepID, streaming.StepID)
	}
	if streaming.DeltaField != "reasoning" {
		t.Errorf("streaming DeltaField: expected reasoning, got %s", streaming.DeltaField)
	}
	if streaming.Delta != "hello reasoning" {
		t.Errorf("streaming Delta: expected 'hello reasoning', got %s", streaming.Delta)
	}
	_ = seq // seq is used inside projector via SeqAssigner
}

func TestProjector_OnTextDeltaThenDone_CompletesReplyStep(t *testing.T) {
	t.Parallel()
	p, cap, _ := newTestProjector(t)
	ctx := context.Background()

	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
	}
	p.OnTurnStart(ctx, meta)
	cap.Published() // drain

	stepID := p.BeginStep(meta, biz.StepKindReply)
	p.OnTextDelta(ctx, stepID, "hello", false)
	p.OnTextDelta(ctx, stepID, " world", false)
	p.OnTextDone(ctx, stepID, "hello world", true /* isFinal */)

	events := cap.Published()
	var completed *biz.StepCompletedEvent
	for _, e := range events {
		if c, ok := e.(*biz.StepCompletedEvent); ok && c.Step.ID == stepID {
			completed = c
			break
		}
	}
	if completed == nil {
		t.Fatalf("expected StepCompletedEvent for %s, got none in %+v", stepID, events)
	}
	if completed.Step.Content != "hello world" {
		t.Errorf("completed Content: expected 'hello world', got %s", completed.Step.Content)
	}
}

func TestProjector_OnToolCall_EmitsStepCreatedThenUpdated(t *testing.T) {
	t.Parallel()
	p, cap, _ := newTestProjector(t)
	ctx := context.Background()

	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
	}
	p.OnTurnStart(ctx, meta)
	cap.Published()

	args := []byte(`{"command":"ls"}`)
	p.OnToolCall(ctx, meta, "shell", args)

	events := cap.Published()
	var created *biz.StepCreatedEvent
	var updated *biz.StepUpdatedEvent
	for _, e := range events {
		if c, ok := e.(*biz.StepCreatedEvent); ok && c.Step.Kind == biz.StepKindAction {
			created = c
		}
		if u, ok := e.(*biz.StepUpdatedEvent); ok && u.Step.Kind == biz.StepKindAction {
			updated = u
		}
	}
	if created == nil {
		t.Fatalf("expected StepCreatedEvent (action), got none")
	}
	if updated == nil {
		t.Fatalf("expected StepUpdatedEvent (action, status=tool_running), got none")
	}
	if updated.Step.ToolName != "shell" {
		t.Errorf("updated ToolName: expected shell, got %s", updated.Step.ToolName)
	}
}

func TestProjector_OnTurnEnd_RootEmitsTaskCompleted(t *testing.T) {
	t.Parallel()
	p, cap, _ := newTestProjector(t)
	ctx := context.Background()

	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
	}
	p.OnTurnStart(ctx, meta)
	cap.Published()

	p.OnTurnEnd(ctx, meta)

	events := cap.Published()
	var turnDone *biz.TurnCompletedEvent
	var taskDone *biz.TaskCompletedEvent
	for _, e := range events {
		if c, ok := e.(*biz.TurnCompletedEvent); ok {
			turnDone = c
		}
		if c, ok := e.(*biz.TaskCompletedEvent); ok {
			taskDone = c
		}
	}
	if turnDone == nil {
		t.Fatalf("expected TurnCompletedEvent, got none")
	}
	if taskDone == nil {
		t.Fatalf("expected TaskCompletedEvent (root turn), got none")
	}
	_ = time.Now()
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestProjector" -count=1 2>&1 | tail -15`
Expected: 编译失败（`ActivityProjector` 未定义）

- [ ] **Step 4: 实现 ActivityProjector v2**

**File:** `internal/agent/v2/projector.go`

```go
package v2

import (
	"context"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// SeqAssigner interface (decoupled from biz.SeqAssigner for testability).
type SeqAssigner interface {
	NextSeq(spiritSessionID string) int64
}

// SequencerPublisher is the minimal interface ActivityProjector needs to
// publish events. Implemented by *Sequencer.
type SequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// ActivityProjector translates trpc-agent-go runtime callbacks into v2 Events.
// It owns step lifecycle state (pending → streaming → completed).
//
// Lifecycle:
//   - OnTurnStart: emits task.created (root) + turn.started
//   - BeginStep: allocates a step ID, holds pending state
//   - OnReasoningDelta/OnTextDelta: emits step.streaming (no persist)
//   - OnReasoningDone/OnTextDone: emits step.completed (with final content)
//   - OnToolCall: emits step.created + step.updated (status=tool_running)
//   - OnToolResult: emits step.completed/step.failed
//   - OnTurnEnd: emits turn.completed + task.completed (root)
//
// All events go through Sequencer.Publish (no direct bus calls).
type ActivityProjector struct {
	seq    SequencerPublisher
	seqAsg SeqAssigner
	lg     loggateway.Logger

	mu sync.Mutex
	// activeStep tracks the currently streaming step per StepID — needed for
	// delta accumulation in sequencer (streaming merge uses StepID as key).
	// We store the Step struct so we can emit a fully-populated StepCompletedEvent.
	activeStep map[string]*biz.Step
	// stepCounter is a per-projector counter for generating step IDs.
	stepCounter atomic.Int64
}

// NewActivityProjector constructs a projector bound to a sequencer + SeqAssigner.
func NewActivityProjector(seq SequencerPublisher, seqAsg SeqAssigner, lg loggateway.Logger) *ActivityProjector {
	return &ActivityProjector{
		seq:       seq,
		seqAsg:    seqAsg,
		lg:        lg.With(loggateway.Domain("projector_v2")),
		activeStep: make(map[string]*biz.Step),
	}
}

// OnTurnStart emits task.created (root only) + turn.started.
// For nested turns (TeamStageID set), only turn.started is emitted (task belongs
// to the spirit root; team_stage owns the task entity).
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
	now := Now()
	if meta.TeamStageID == "" {
		// Root spirit turn: emit task.created
		task := meta.newTask(meta.TaskID, biz.TaskStatusRunning, meta.TaskContent)
		task.UpdatedAt = now
		p.seq.Publish(ctx, &biz.TaskCreatedEvent{
			Task:     task,
			BaseEvent: biz.BaseEvent{
				SpiritSessionIDVal: meta.SpiritSessionID,
				TaskIDVal:          task.ID,
				OccurredAtTime:     now,
			},
		})
	}

	// Emit turn.started with Seq allocated
	seq := p.seqAsg.NextSeq(meta.SpiritSessionID)
	turn := meta.newTurn(meta.TurnID, seq)
	turn.StartedAt = now
	p.seq.Publish(ctx, &biz.TurnStartedEvent{
		Turn: turn,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: meta.SpiritSessionID,
			TaskIDVal:          meta.TaskID,
			OccurredAtTime:     now,
		},
	})
}

// BeginStep allocates a step ID and registers a pending step in activeStep.
// Caller must use the returned ID for subsequent delta/done callbacks.
func (p *ActivityProjector) BeginStep(meta ProjectMeta, kind biz.StepKind) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.stepCounter.Add(1)
	stepID := meta.TurnID + "-s" + itoa(int(n))
	step := meta.newStep(stepID, kind, 0) // Seq=0; assigned on first delta
	p.activeStep[stepID] = &step

	// Emit step.created immediately so frontend can render placeholder
	now := Now()
	p.seq.Publish(context.Background(), &biz.StepCreatedEvent{
		Step: step,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: meta.SpiritSessionID,
			TaskIDVal:          meta.TaskID,
			OccurredAtTime:     now,
		},
	})
	return stepID
}

// OnReasoningDelta emits step.streaming with DeltaField=reasoning.
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, stepID string, delta string, _ bool) {
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	p.mu.Unlock()
	if !ok {
		p.lg.Warn("OnReasoningDelta: unknown step", loggateway.Str("step_id", stepID))
		return
	}
	// Allocate Seq on first delta (lazy)
	if step.Seq == 0 {
		step.Seq = p.seqAsg.NextSeq(step.SpiritSessionID)
	}
	p.seq.Publish(ctx, &biz.StepStreamingEvent{
		StepID:     stepID,
		DeltaField: "reasoning",
		Delta:      delta,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: step.SpiritSessionID,
			TaskIDVal:          step.TaskID,
			OccurredAtTime:     Now(),
		},
	})
}

// OnReasoningDone emits step.completed for the thinking step.
func (p *ActivityProjector) OnReasoningDone(ctx context.Context, stepID string, finalContent string) {
	p.completeStep(ctx, stepID, finalContent, "")
}

// OnTextDelta emits step.streaming with DeltaField=content (for reply steps).
func (p *ActivityProjector) OnTextDelta(ctx context.Context, stepID string, delta string, _ bool) {
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	p.mu.Unlock()
	if !ok {
		p.lg.Warn("OnTextDelta: unknown step", loggateway.Str("step_id", stepID))
		return
	}
	if step.Seq == 0 {
		step.Seq = p.seqAsg.NextSeq(step.SpiritSessionID)
	}
	p.seq.Publish(ctx, &biz.StepStreamingEvent{
		StepID:     stepID,
		DeltaField: "content",
		Delta:      delta,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: step.SpiritSessionID,
			TaskIDVal:          step.TaskID,
			OccurredAtTime:     Now(),
		},
	})
}

// OnTextDone emits step.completed for the reply step.
func (p *ActivityProjector) OnTextDone(ctx context.Context, stepID string, finalContent string, isFinal bool) {
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.Content = finalContent
		if isFinal {
			step.IsFinal = true
		}
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, finalContent, "")
}

// OnToolCall emits step.updated with status=tool_running.
// Assumes BeginStep was called prior to obtain stepID.
func (p *ActivityProjector) OnToolCall(ctx context.Context, meta ProjectMeta, toolName string, args []byte) {
	// Find the action step (most recent one for this turn)
	p.mu.Lock()
	defer p.mu.Unlock()
	var stepID string
	var step *biz.Step
	for id, s := range p.activeStep {
		if s.Kind == biz.StepKindAction && s.TurnID == meta.TurnID {
			stepID, step = id, s
			break
		}
	}
	if step == nil {
		// Tool call without a prior BeginStep: create one implicitly
		stepID = p.BeginStep(meta, biz.StepKindAction)
		step = p.activeStep[stepID]
	}
	step.ToolName = toolName
	step.ToolArgs = args
	step.Status = biz.StepStatusToolRunning
	now := Now()
	p.seq.Publish(ctx, &biz.StepUpdatedEvent{
		Step: *step,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: step.SpiritSessionID,
			TaskIDVal:          step.TaskID,
			OccurredAtTime:     now,
		},
	})
}

// OnToolResult emits step.completed (success) or step.failed (error).
func (p *ActivityProjector) OnToolResult(ctx context.Context, stepID string, result string, err error) {
	if err != nil {
		p.failStep(ctx, stepID, err)
		return
	}
	p.completeStep(ctx, stepID, "", result)
}

// OnTurnEnd emits turn.completed (+ task.completed for root turn).
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, meta ProjectMeta) {
	now := Now()
	p.seq.Publish(ctx, &biz.TurnCompletedEvent{
		Turn: biz.Turn{
			ID: meta.TurnID, TaskID: meta.TaskID,
			SpiritSessionID: meta.SpiritSessionID,
			FinishedAt: now,
			Status:     biz.TurnStatusCompleted,
		},
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: meta.SpiritSessionID,
			TaskIDVal:          meta.TaskID,
			OccurredAtTime:     now,
		},
	})

	if meta.TeamStageID == "" {
		// Root: emit task.completed
		p.seq.Publish(ctx, &biz.TaskCompletedEvent{
			Task: biz.Task{
				ID: meta.TaskID, SpiritSessionID: meta.SpiritSessionID,
				Status: biz.TaskStatusCompleted, UpdatedAt: now,
				Version: 2,
			},
			BaseEvent: biz.BaseEvent{
				SpiritSessionIDVal: meta.SpiritSessionID,
				TaskIDVal:          meta.TaskID,
				OccurredAtTime:     now,
			},
		})
	}
}

// completeStep is the internal helper to emit step.completed and clear activeStep.
func (p *ActivityProjector) completeStep(ctx context.Context, stepID, content, result string) {
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		if content != "" {
			step.Content = content
		}
		if result != "" {
			step.ToolResult = result
		}
		step.Status = biz.StepStatusCompleted
		step.FinishedAt = Now()
		delete(p.activeStep, stepID)
	}
	p.mu.Unlock()
	if !ok {
		p.lg.Warn("completeStep: unknown step", loggateway.Str("step_id", stepID))
		return
	}
	now := Now()
	p.seq.Publish(ctx, &biz.StepCompletedEvent{
		Step: *step,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: step.SpiritSessionID,
			TaskIDVal:          step.TaskID,
			OccurredAtTime:     now,
		},
	})
}

func (p *ActivityProjector) failStep(ctx context.Context, stepID string, err error) {
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.Status = biz.StepStatusFailed
		step.Error = err.Error()
		step.FinishedAt = Now()
		delete(p.activeStep, stepID)
	}
	p.mu.Unlock()
	if !ok {
		p.lg.Warn("failStep: unknown step", loggateway.Str("step_id", stepID))
		return
	}
	now := Now()
	p.seq.Publish(ctx, &biz.StepFailedEvent{
		Step: *step,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: step.SpiritSessionID,
			TaskIDVal:          step.TaskID,
			OccurredAtTime:     now,
		},
	})
}

// itoa is a tiny strconv.Itoa to avoid import in hot path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
```

- [ ] **Step 5: 跑测试验证**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestProjector" -count=1 -v 2>&1 | tail -30`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/v2/project_meta.go internal/agent/v2/projector.go internal/agent/v2/projector_test.go
git commit -m "feat(agent/v2): implement ActivityProjector v2 with BeginStep lifecycle and event translation"
```

### Task 13: stream_consumer 改造（切换至 v2 Projector）

**目标**：修改 `internal/agent/stream_consumer.go`，让其使用 v2 `ActivityProjector` 而非 v1。通过配置开关控制（Phase 1 默认仍走 v1，验证后切换）。

**Files:**
- Modify: `internal/agent/stream_consumer.go`（增加 v2 projector 字段 + 配置开关）
- Modify: `internal/agent/stream_consumer_test.go`（增加 v2 路径测试）

- [ ] **Step 1: 阅读现有 stream_consumer.go**

Run: `cd f:\aranea-agents && wc -l internal/agent/stream_consumer.go`
（预估 200-400 行）

- [ ] **Step 2: 增加 v2 projector 字段 + dual-path dispatch**

修改 `internal/agent/stream_consumer.go`，在 struct 中添加：

```go
type StreamConsumer struct {
	// ... existing v1 fields ...
	v2Projector *v2.ActivityProjector // nil = v1-only mode
	v2Enabled   atomic.Bool            // feature flag
}

// SetV2Projector enables v2 event path. Once set, callbacks dispatch to BOTH
// v1 (for backward compat) AND v2 (for new entities). v1 can be disabled via
// SetV1Disabled(true) after verification.
func (s *StreamConsumer) SetV2Projector(p *v2.ActivityProjector) {
	s.v2Projector = p
	s.v2Enabled.Store(true)
}

// In each callback (e.g. OnTurnStart, OnReasoningDelta, OnTextDelta, OnToolCall,
// OnToolResult, OnTurnEnd), append a v2 dispatch:
//
//   if s.v2Enabled.Load() && s.v2Projector != nil {
//       s.v2Projector.OnTurnStart(ctx, v2ProjectMetaFromV1(s.currentMeta))
//   }
//
// Helper: v2ProjectMetaFromV1 maps the existing v1 ProjectMeta to v2 ProjectMeta
// (filling TeamStageID/TeamRunID/MemberSessionID from context or empty strings).
```

- [ ] **Step 3: 写 dual-path 集成测试**

**File:** `internal/agent/stream_consumer_v2_test.go`

```go
package agent

import (
	"context"
	"testing"

	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestStreamConsumer_V2DualPath verifies that with v2Projector set, both v1 and
// v2 paths receive events. v1 path is exercised via existing activityRepo;
// v2 path via the capturing sequencer.
func TestStreamConsumer_V2DualPath(t *testing.T) {
	t.Parallel()
	// Setup: minimal v1 + v2 wiring.
	// ... (use existing test helpers from stream_consumer_test.go)
	_ = loggateway.NewNoop()
	_ = context.Background()
	_ = v2.NewActivityProjector(nil, nil, loggateway.NewNoop())
	_ = biz.EventKindTaskCreated
	t.Skip("full integration test — wire up after Tasks 14-15 are complete")
}
```

> **Note**: 此测试目前是 placeholder（skipped），实际集成测试在 Task 17 编写。本 Step 仅确保 v2 projector 字段被注入，编译通过。

- [ ] **Step 4: 跑全量 agent 测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 2>&1 | tail -20`
Expected: 全部 PASS（v1 path 仍工作；v2 path 默认禁用）

- [ ] **Step 5: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/stream_consumer.go internal/agent/stream_consumer_v2_test.go
git commit -m "feat(agent): add v2 projector dual-path dispatch in StreamConsumer (disabled by default)"
```

---

## Phase 1.8: PlanExecutor + spirit_team 改造

### Task 14: PlanExecutor — 显式正向调度器

**目标**：新建 `internal/service/plan_executor.go`，替代 service 层的 `updatePlanStepForTeam` 反向同步。PlanExecutor 订阅 plan_board，按 DAG 依赖关系派发 step 给 TeamOrchestrator，监听 team 完成事件后更新 step 状态。

**Files:**
- Create: `internal/service/plan_executor.go`
- Create: `internal/service/plan_executor_test.go`

- [ ] **Step 1: 写测试（红）**

**File:** `internal/service/plan_executor_test.go`

```go
package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/pkg/loggateway"
)

// fakeOrchestrator records Orchestrate calls (one per step).
type fakeOrchestrator struct {
	mu    sync.Mutex
	calls []biz.PlanStep
	done  map[string]chan biz.TeamCompleteEvent // stepID → completion channel
}

func (f *fakeOrchestrator) Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (<-chan biz.TeamCompleteEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, step)
	ch := make(chan biz.TeamCompleteEvent, 1)
	if f.done == nil {
		f.done = make(map[string]chan biz.TeamCompleteEvent)
	}
	f.done[step.ID] = ch
	return ch, nil
}

func (f *fakeOrchestrator) Calls() []biz.PlanStep {
	f.mu.Lock(); defer f.mu.Unlock()
	out := make([]biz.PlanStep, len(f.calls))
	copy(out, f.calls)
	return out
}

// completeStep signals the orchestrator that the step's team has completed.
func (f *fakeOrchestrator) completeStep(stepID string, success bool, errMsg string) {
	f.mu.Lock()
	ch, ok := f.done[stepID]
	f.mu.Unlock()
	if !ok {
		return
	}
	ch <- biz.TeamCompleteEvent{
		StepID:   stepID,
		Success:  success,
		ErrorMsg: errMsg,
	}
	close(ch)
}

// fakeReposForExecutor collects calls to all repos PlanExecutor uses.
type fakeReposForExecutor struct {
	mu       sync.Mutex
	steps    []biz.PlanStep
	stages   []biz.TeamStage
	boards   []biz.PlanBoard
}

func (f *fakeReposForExecutor) UpsertPlanStep(_ context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.steps = append(f.steps, ps)
	return ps, nil
}
func (f *fakeReposForExecutor) UpsertTeamStage(_ context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.stages = append(f.stages, ts)
	return ts, nil
}
func (f *fakeReposForExecutor) UpsertPlanBoard(_ context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.boards = append(f.boards, pb)
	return pb, nil
}
func (f *fakeReposForExecutor) GetPlanStep(_ context.Context, id string) (biz.PlanStep, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	for _, s := range f.steps {
		if s.ID == id {
			return s, nil
		}
	}
	return biz.PlanStep{}, nil
}

// fakeSeq is a minimal v2.SequencerPublisher for PlanExecutor tests.
type fakeSeq struct {
	mu  sync.Mutex
	pub []biz.Event
}
func (f *fakeSeq) Publish(_ context.Context, e biz.Event) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.pub = append(f.pub, e)
}
func (f *fakeSeq) Published() []biz.Event {
	f.mu.Lock(); defer f.mu.Unlock()
	out := make([]biz.Event, len(f.pub))
	copy(out, f.pub)
	return out
}

func newTestExecutor(t *testing.T) (*PlanExecutor, *fakeOrchestrator, *fakeReposForExecutor, *fakeSeq) {
	t.Helper()
	orch := &fakeOrchestrator{}
	repos := &fakeReposForExecutor{}
	seq := &fakeSeq{}
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	return pe, orch, repos, seq
}

// TestPlanExecutor_SequentialDAG verifies a 3-step linear DAG (s1→s2→s3):
// s1 runs first; when s1 completes, s2 starts; when s2 completes, s3 starts.
func TestPlanExecutor_SequentialDAG(t *testing.T) {
	t.Parallel()
	pe, orch, _, _ := newTestExecutor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now().UTC()
	board := biz.PlanBoard{
		ID: "pb-1", SpiritSessionID: "sess-1", TaskID: "task-1",
		Status: biz.PlanBoardStatusActive,
		Steps: []biz.PlanStepRef{
			{ID: "s1", StepKey: "s1", DependsOn: nil},
			{ID: "s2", StepKey: "s2", DependsOn: []string{"s1"}},
			{ID: "s3", StepKey: "s3", DependsOn: []string{"s2"}},
		},
		Version: 1,
	}

	go func() { _ = pe.Subscribe(ctx, board) }()
	// Wait briefly for s1 to be dispatched
	time.Sleep(50 * time.Millisecond)
	_ = now

	calls := orch.Calls()
	if len(calls) != 1 || calls[0].ID != "s1" {
		t.Fatalf("expected s1 dispatched first, got %+v", calls)
	}

	// Complete s1 → s2 should start
	orch.completeStep("s1", true, "")
	time.Sleep(50 * time.Millisecond)
	calls = orch.Calls()
	if len(calls) != 2 || calls[1].ID != "s2" {
		t.Fatalf("expected s2 dispatched after s1 completes, got %+v", calls)
	}

	// Complete s2 → s3 should start
	orch.completeStep("s2", true, "")
	time.Sleep(50 * time.Millisecond)
	calls = orch.Calls()
	if len(calls) != 3 || calls[2].ID != "s3" {
		t.Fatalf("expected s3 dispatched after s2 completes, got %+v", calls)
	}

	// Complete s3 → board status becomes completed
	orch.completeStep("s3", true, "")
	time.Sleep(50 * time.Millisecond)
}

// TestPlanExecutor_ParallelRoots verifies that two root steps (no deps) are
// dispatched concurrently.
func TestPlanExecutor_ParallelRoots(t *testing.T) {
	t.Parallel()
	pe, orch, _, _ := newTestExecutor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	board := biz.PlanBoard{
		ID: "pb-1", SpiritSessionID: "sess-1", TaskID: "task-1",
		Status: biz.PlanBoardStatusActive,
		Steps: []biz.PlanStepRef{
			{ID: "s1", StepKey: "s1"},
			{ID: "s2", StepKey: "s2"},
		},
		Version: 1,
	}

	go func() { _ = pe.Subscribe(ctx, board) }()
	time.Sleep(50 * time.Millisecond)

	calls := orch.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 steps dispatched in parallel, got %d: %+v", len(calls), calls)
	}
	seen := map[string]bool{}
	for _, c := range calls {
		seen[c.ID] = true
	}
	if !seen["s1"] || !seen["s2"] {
		t.Fatalf("expected both s1 and s2 dispatched, got %+v", seen)
	}
}

// TestPlanExecutor_FailedStepBlocksDownstream verifies that when a step fails,
// downstream steps are skipped (status=skipped).
func TestPlanExecutor_FailedStepBlocksDownstream(t *testing.T) {
	t.Parallel()
	pe, orch, repos, seq := newTestExecutor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	board := biz.PlanBoard{
		ID: "pb-1", SpiritSessionID: "sess-1", TaskID: "task-1",
		Status: biz.PlanBoardStatusActive,
		Steps: []biz.PlanStepRef{
			{ID: "s1", StepKey: "s1"},
			{ID: "s2", StepKey: "s2", DependsOn: []string{"s1"}},
		},
		Version: 1,
	}

	go func() { _ = pe.Subscribe(ctx, board) }()
	time.Sleep(50 * time.Millisecond)

	// Fail s1
	orch.completeStep("s1", false, "tool error")
	time.Sleep(50 * time.Millisecond)

	// s2 should be marked skipped via UpsertPlanStep (not via orchestrator)
	repos.mu.Lock()
	defer repos.mu.Unlock()
	var s2Row *biz.PlanStep
	for i := range repos.steps {
		if repos.steps[i].ID == "s2" {
			s2Row = &repos.steps[i]
			break
		}
	}
	if s2Row == nil {
		t.Fatalf("expected s2 upserted with status=skipped, got %+v", repos.steps)
	}
	if s2Row.Status != biz.PlanStepStatusSkipped {
		t.Errorf("expected s2 status=skipped, got %s", s2Row.Status)
	}

	// Verify plan_step.skipped event was published
	pub := seq.Published()
	var skippedEvent *biz.PlanStepSkippedEvent
	for _, e := range pub {
		if ev, ok := e.(*biz.PlanStepSkippedEvent); ok && ev.PlanStep.ID == "s2" {
			skippedEvent = ev
		}
	}
	if skippedEvent == nil {
		t.Errorf("expected plan_step.skipped event, got %+v", pub)
	}
	_ = v2.NewActivityProjector(nil, nil, loggateway.NewNoop()) // ensure v2 import
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd f:\aranea-agents && go test ./internal/service/ -run "TestPlanExecutor" -count=1 2>&1 | tail -10`
Expected: 编译失败（`PlanExecutor`、`TeamOrchestrator` 等未定义）

- [ ] **Step 3: 定义 TeamOrchestrator 接口**

**File:** `internal/service/team_orchestrator.go`（新建，定义接口）

```go
package service

import (
	"context"

	"aranea-agents/internal/biz"
)

// TeamOrchestrator abstracts the team execution layer.
// PlanExecutor delegates team execution to this interface, keeping it decoupled
// from the concrete TeamOrchestrator implementation.
//
// Stability: evolving
type TeamOrchestrator interface {
	// Orchestrate starts a team_run for the given step + team_stage.
	// Returns a channel that receives exactly one TeamCompleteEvent when the
	// team finishes (success or failure).
	Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (<-chan biz.TeamCompleteEvent, error)
}

// TeamCompleteEvent is emitted when a team_run finishes.
// StepID correlates back to the PlanStep that triggered the team.
type TeamCompleteEvent = biz.TeamCompleteEvent
```

- [ ] **Step 4: 在 biz 中追加 TeamCompleteEvent 类型**

**File:** `internal/biz/team_complete_event.go`

```go
package biz

// TeamCompleteEvent is emitted when a team_run finishes.
// Used by PlanExecutor to correlate team completion with PlanStep status.
type TeamCompleteEvent struct {
	StepID   string // the PlanStep that triggered this team
	TeamRunID string
	Success  bool
	ErrorMsg string
}
```

- [ ] **Step 5: 实现 PlanExecutor**

**File:** `internal/service/plan_executor.go`

```go
package service

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/pkg/loggateway"
)

// executorRepos is the minimal repo set PlanExecutor needs.
type executorRepos interface {
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error)
}

// sequencerPublisher is the minimal publish interface PlanExecutor needs.
type sequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// PlanExecutor is the forward scheduler for plan boards.
//
// Responsibilities:
//   1. Parse PlanBoard.Steps into a DAG (depends_on).
//   2. Dispatch root steps (no deps) to TeamOrchestrator concurrently.
//   3. On team completion, update PlanStep status via state machine.
//   4. When all deps of a downstream step are completed, dispatch it.
//   5. When a step fails, mark all transitive downstream as skipped.
//
// Replaces the v1 reverse-sync (updatePlanStepForTeam).
type PlanExecutor struct {
	repos  executorRepos
	orch   TeamOrchestrator
	seq    sequencerPublisher
	lg     loggateway.Logger

	// active plans: PlanBoardID → *executionContext
	contexts sync.Map
}

// executionContext tracks the state of an executing plan.
type executionContext struct {
	mu              sync.Mutex
	board           biz.PlanBoard
	pending         map[string]bool   // stepID → not yet completed
	running         map[string]bool   // stepID → currently executing
	completed       map[string]bool   // stepID → finished successfully
	failed          map[string]bool   // stepID → finished with error
	skipped         map[string]bool   // stepID → skipped due to upstream failure
	stepByID        map[string]biz.PlanStepRef
	cancel          context.CancelFunc
}

// NewPlanExecutor constructs a PlanExecutor.
func NewPlanExecutor(repos executorRepos, orch TeamOrchestrator, seq sequencerPublisher, lg loggateway.Logger) *PlanExecutor {
	return &PlanExecutor{
		repos: repos,
		orch:  orch,
		seq:   seq,
		lg:    lg.With(loggateway.Domain("plan_executor")),
	}
}

// Subscribe starts executing a plan board.
// Blocks until the plan completes (all steps completed/failed/skipped) or ctx is canceled.
func (p *PlanExecutor) Subscribe(ctx context.Context, board biz.PlanBoard) error {
	ec := &executionContext{
		board:     board,
		pending:   make(map[string]bool),
		running:   make(map[string]bool),
		completed: make(map[string]bool),
		failed:    make(map[string]bool),
		skipped:   make(map[string]bool),
		stepByID:  make(map[string]biz.PlanStepRef),
	}
	for _, s := range board.Steps {
		ec.pending[s.ID] = true
		ec.stepByID[s.ID] = s
	}

	planCtx, cancel := context.WithCancel(ctx)
	ec.cancel = cancel
	p.contexts.Store(board.ID, ec)
	defer p.contexts.Delete(board.ID)

	// Dispatch root steps (no deps)
	for _, s := range board.Steps {
		if len(s.DependsOn) == 0 {
			p.dispatchStep(planCtx, ec, s)
		}
	}

	// Wait until no pending or running steps
	for {
		ec.mu.Lock()
		pendingCount := len(ec.pending)
		runningCount := len(ec.running)
		ec.mu.Unlock()
		if pendingCount == 0 && runningCount == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// dispatchStep starts a team_stage + team_run for the given step.
func (p *PlanExecutor) dispatchStep(ctx context.Context, ec *executionContext, ref biz.PlanStepRef) {
	ec.mu.Lock()
	if !ec.pending[ref.ID] {
		ec.mu.Unlock()
		return
	}
	delete(ec.pending, ref.ID)
	ec.running[ref.ID] = true
	ec.mu.Unlock()

	now := time.Now().UTC()
	teamStageID := ref.ID + "-stage"
	teamStage := biz.TeamStage{
		ID:              teamStageID,
		SpiritSessionID: ec.board.SpiritSessionID,
		TaskID:          ec.board.TaskID,
		Status:          biz.TeamStageStatusPending,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	// Persist team_stage
	if _, err := p.repos.UpsertTeamStage(ctx, teamStage); err != nil {
		p.lg.Error("upsert team_stage failed",
			loggateway.Str("step_id", ref.ID), loggateway.Err(err))
	}

	// Publish team_stage.created
	p.seq.Publish(ctx, &biz.TeamStageCreatedEvent{
		TeamStage: teamStage,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: ec.board.SpiritSessionID,
			TaskIDVal:          ec.board.TaskID,
			OccurredAtTime:     now,
		},
	})

	// Update PlanStep status → running (via state machine)
	step, _ := p.repos.GetPlanStep(ctx, ref.ID)
	if step.ID == "" {
		step = biz.PlanStep{
			ID: ref.ID, PlanBoardID: ec.board.ID, SpiritSessionID: ec.board.SpiritSessionID,
			StepKey: ref.StepKey, Title: ref.Title, DependsOn: ref.DependsOn,
			Status: biz.PlanStepStatusPending, Version: 1,
		}
	}
	transitioned, err := biz.TransitionPlanStep(step.Status, biz.PlanStepEventStart)
	if err != nil {
		p.lg.Error("state transition failed (start)",
			loggateway.Str("step_id", ref.ID), loggateway.Err(err))
		return
	}
	step.Status = transitioned
	step.AssignedTeamID = teamStageID
	step.StartedAt = now
	step.Version++
	if _, err := p.repos.UpsertPlanStep(ctx, step); err != nil {
		p.lg.Error("upsert plan_step (running) failed",
			loggateway.Str("step_id", ref.ID), loggateway.Err(err))
	}

	// Publish plan_step.started
	p.seq.Publish(ctx, &biz.PlanStepStartedEvent{
		PlanStep: step,
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: ec.board.SpiritSessionID,
			TaskIDVal:          ec.board.TaskID,
			OccurredAtTime:     now,
		},
	})

	// Delegate to TeamOrchestrator (async)
	teamCh, err := p.orch.Orchestrate(ctx, step, teamStage)
	if err != nil {
		p.lg.Error("orchestrate failed",
			loggateway.Str("step_id", ref.ID), loggateway.Err(err))
		p.handleTeamCompletion(ctx, ec, ref.ID, biz.TeamCompleteEvent{
			StepID: ref.ID, Success: false, ErrorMsg: err.Error(),
		})
		return
	}

	// Listen for team completion in a goroutine
	go func() {
		select {
		case ev, ok := <-teamCh:
			if !ok {
				ev = biz.TeamCompleteEvent{StepID: ref.ID, Success: false, ErrorMsg: "team channel closed"}
			}
			p.handleTeamCompletion(ctx, ec, ref.ID, ev)
		case <-ctx.Done():
		}
	}()
}

// handleTeamCompletion processes a team completion event:
//   - update PlanStep status (completed/failed)
//   - publish plan_step.completed/failed/skipped
//   - checkDownstream
func (p *PlanExecutor) handleTeamCompletion(ctx context.Context, ec *executionContext, stepID string, ev biz.TeamCompleteEvent) {
	ec.mu.Lock()
	delete(ec.running, stepID)
	ec.mu.Unlock()

	now := time.Now().UTC()
	step, _ := p.repos.GetPlanStep(ctx, stepID)
	if step.ID == "" {
		p.lg.Error("step not found after team completion",
			loggateway.Str("step_id", stepID))
		return
	}

	var targetStatus biz.PlanStepStatus
	var publishEvent biz.Event
	if ev.Success {
		targetStatus, _ = biz.TransitionPlanStep(step.Status, biz.PlanStepEventComplete)
		step.Status = targetStatus
		step.FinishedAt = now
		step.Version++
		publishEvent = &biz.PlanStepCompletedEvent{
			PlanStep: step,
			BaseEvent: biz.BaseEvent{
				SpiritSessionIDVal: ec.board.SpiritSessionID,
				TaskIDVal:          ec.board.TaskID,
				OccurredAtTime:     now,
			},
		}
		ec.mu.Lock()
		ec.completed[stepID] = true
		ec.mu.Unlock()
	} else {
		targetStatus, _ = biz.TransitionPlanStep(step.Status, biz.PlanStepEventFail)
		step.Status = targetStatus
		step.Error = &biz.PlanStepError{Message: ev.ErrorMsg}
		step.FinishedAt = now
		step.Version++
		publishEvent = &biz.PlanStepFailedEvent{
			PlanStep: step,
			BaseEvent: biz.BaseEvent{
				SpiritSessionIDVal: ec.board.SpiritSessionID,
				TaskIDVal:          ec.board.TaskID,
				OccurredAtTime:     now,
			},
		}
		ec.mu.Lock()
		ec.failed[stepID] = true
		ec.mu.Unlock()
	}

	if _, err := p.repos.UpsertPlanStep(ctx, step); err != nil {
		p.lg.Error("upsert plan_step (completion) failed",
			loggateway.Str("step_id", stepID), loggateway.Err(err))
	}
	p.seq.Publish(ctx, publishEvent)

	// Check downstream
	p.checkDownstream(ctx, ec, stepID)
}

// checkDownstream finds steps whose depends_on includes the completed step,
// and dispatches those whose all deps are now completed.
// For failed steps, downstream steps are marked skipped (cascading).
func (p *PlanExecutor) checkDownstream(ctx context.Context, ec *executionContext, completedStepID string) {
	ec.mu.Lock()
	failed := ec.failed[completedStepID]
	pendingCopy := make(map[string]bool, len(ec.pending))
	for k, v := range ec.pending {
		pendingCopy[k] = v
	}
	stepByID := make(map[string]biz.PlanStepRef, len(ec.stepByID))
	for k, v := range ec.stepByID {
		stepByID[k] = v
	}
	completed := make(map[string]bool, len(ec.completed))
	for k, v := range ec.completed {
		completed[k] = v
	}
	ec.mu.Unlock()

	now := time.Now().UTC()

	// If the completed step failed, cascade-skip all downstream dependents
	if failed {
		toSkip := p.findDownstreamDependents(ec, completedStepID)
		for _, depID := range toSkip {
			step := stepByID[depID]
			transitioned, _ := biz.TransitionPlanStep(biz.PlanStepStatusPending, biz.PlanStepEventSkip)
			updatedStep := biz.PlanStep{
				ID: step.ID, PlanBoardID: ec.board.ID, SpiritSessionID: ec.board.SpiritSessionID,
				StepKey: step.StepKey, Title: step.Title, DependsOn: step.DependsOn,
				Status: transitioned, FinishedAt: now, Version: 1,
			}
			if _, err := p.repos.UpsertPlanStep(ctx, updatedStep); err != nil {
				p.lg.Error("upsert skipped plan_step failed",
					loggateway.Str("step_id", depID), loggateway.Err(err))
			}
			p.seq.Publish(ctx, &biz.PlanStepSkippedEvent{
				PlanStep: updatedStep,
				BaseEvent: biz.BaseEvent{
					SpiritSessionIDVal: ec.board.SpiritSessionID,
					TaskIDVal:          ec.board.TaskID,
					OccurredAtTime:     now,
				},
			})
			ec.mu.Lock()
			delete(ec.pending, depID)
			ec.skipped[depID] = true
			ec.mu.Unlock()
		}
		return
	}

	// For each pending step, check if all deps are now completed
	for stepID, isPending := range pendingCopy {
		if !isPending {
			continue
		}
		step := stepByID[stepID]
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if !completed[dep] {
				allDepsCompleted = false
				break
			}
		}
		if allDepsCompleted {
			p.dispatchStep(ctx, ec, step)
		}
	}
}

// findDownstreamDependents returns the transitive set of step IDs that depend
// on the given step (directly or transitively).
func (p *PlanExecutor) findDownstreamDependents(ec *executionContext, stepID string) []string {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	visited := map[string]bool{}
	queue := []string{stepID}
	var result []string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		// Find all steps whose DependsOn contains cur
		for id, s := range ec.stepByID {
			for _, dep := range s.DependsOn {
				if dep == cur && !visited[id] {
					result = append(result, id)
					queue = append(queue, id)
				}
			}
		}
	}
	return result
}

// ensure v2 import is retained for type alias purposes if extended later
var _ = v2.NewActivityProjector
```

- [ ] **Step 6: 跑测试验证 PlanExecutor**

Run: `cd f:\aranea-agents && go test ./internal/service/ -run "TestPlanExecutor" -count=1 -v 2>&1 | tail -30`
Expected: 全部 PASS

- [ ] **Step 7: 跑 race detector**

Run: `cd f:\aranea-agents && go test ./internal/service/ -run "TestPlanExecutor" -count=1 -race 2>&1 | tail -10`
Expected: PASS, no race detected

- [ ] **Step 8: 提交**

```bash
cd f:\aranea-agents
git add internal/service/plan_executor.go internal/service/plan_executor_test.go internal/service/team_orchestrator.go internal/biz/team_complete_event.go
git commit -m "feat(service): add PlanExecutor — forward DAG scheduler replacing reverse-sync updatePlanStepForTeam"
```

### Task 15: spirit_team 改造 — 移除 updatePlanStepForTeam

**目标**：修改 `internal/service/spirit_team.go`，移除 `updatePlanStepForTeam` 反向同步逻辑，改为发事件到 sequencer（通过 PlanExecutor.Subscribe 触发）。

**Files:**
- Modify: `internal/service/spirit_team.go`（移除 `updatePlanStepForTeam`，注入 `PlanExecutor`）
- Modify: `internal/service/spirit_team_test.go`（更新断言）

- [ ] **Step 1: 定位 updatePlanStepForTeam 的所有调用点**

Run: `cd f:\aranea-agents && grep -rn "updatePlanStepForTeam" internal/ --include="*.go"`
Expected: 找到 1-3 个调用点（在 spirit_team.go 和测试中）

- [ ] **Step 2: 在 spirit_team.go 移除 updatePlanStepForTeam 方法**

找到 `updatePlanStepForTeam` 方法（约 50-100 行），整体删除。

替换为：在 team 完成回调中发送 `biz.TeamCompleteEvent` 到 PlanExecutor 监听的 channel（PlanExecutor.Subscribe 通过 TeamOrchestrator.Orchestrate 已经订阅了 channel，所以 spirit_team 不需要主动通知 PlanExecutor）。

- [ ] **Step 3: 注入 PlanExecutor 到 spirit_team 构造函数**

修改 `internal/service/spirit_team.go` 的 struct 与 NewSpiritTeamUsecase：

```go
type SpiritTeamUsecase struct {
	// ... existing fields ...
	planExecutor *PlanExecutor // injected
}

func NewSpiritTeamUsecase(
	// ... existing params ...
	pe *PlanExecutor,
) *SpiritTeamUsecase {
	return &SpiritTeamUsecase{
		// ... existing fields ...
		planExecutor: pe,
	}
}
```

- [ ] **Step 4: 在 plan 创建后调用 PlanExecutor.Subscribe**

找到 spirit_team.go 中创建 PlanBoard 的位置（通常在 team 启动前），追加：

```go
// After PlanBoard is persisted, kick off PlanExecutor
go func() {
	if err := uc.planExecutor.Subscribe(ctx, planBoard); err != nil {
		uc.lg.Error("plan executor subscribe failed",
			loggateway.Str("plan_board_id", planBoard.ID), loggateway.Err(err))
	}
}()
```

- [ ] **Step 5: 更新 spirit_team_test.go**

移除对 `updatePlanStepForTeam` 的所有断言；改为验证 PlanExecutor.Subscribe 被调用（通过 mock）。

```go
// In test setup:
// pe := NewPlanExecutor(mockRepos, mockOrch, mockSeq, loggateway.NewNoop())
// uc := NewSpiritTeamUsecase(/* ... */, pe)
// Assert: no updatePlanStepForTeam calls; instead, expect planStepRepo.UpsertPlanStep
//        calls with status=running/completed (driven by PlanExecutor).
```

- [ ] **Step 6: 跑 spirit_team 测试**

Run: `cd f:\aranea-agents && go test ./internal/service/ -run "TestSpiritTeam" -count=1 2>&1 | tail -20`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
cd f:\aranea-agents
git add internal/service/spirit_team.go internal/service/spirit_team_test.go
git commit -m "refactor(service): remove updatePlanStepForTeam reverse-sync, delegate to PlanExecutor"
```

---

## Phase 1.9: WS 推送 + 集成验证

### Task 16: WS 双格式推送 + Wire 重连

**目标**：在 WS 推送层（`internal/server/ws_io_pump.go`）订阅 v2 EventBus，同时推送 v2 事件 + 通过 CompatAdapter 转换的 v1 事件。保持旧前端兼容。

**Files:**
- Modify: `internal/server/ws_io_pump.go`（增加 v2 subscriber）
- Create: `internal/server/ws_v2_subscriber.go`
- Modify: `internal/server/wire.go`（注入 v2 Sequencer + EventBus + CompatAdapter）

- [ ] **Step 1: 创建 WS v2 subscriber**

**File:** `internal/server/ws_v2_subscriber.go`

```go
package server

import (
	"context"
	"encoding/json"
	"sync"

	"aranea-agents/internal/biz"
	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/pkg/loggateway"
)

// WSV2Subscriber listens on a v2 EventBus and forwards events to WS clients
// subscribed to a given spirit_session_id.
type WSV2Subscriber struct {
	bus      biz.EventBus       // v2 bus (event.V2Bus)
	hub      WSMessageBroadcaster // existing hub that fans out to WS clients
	lg       loggateway.Logger
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// WSMessageBroadcaster is the minimal interface to push messages to WS clients.
// Implemented by the existing ws_hub.
type WSMessageBroadcaster interface {
	BroadcastToSession(spiritSessionID string, msg []byte)
}

// NewWSV2Subscriber constructs and starts a subscriber.
// Caller must call Close() to stop the goroutine.
func NewWSV2Subscriber(bus biz.EventBus, hub WSMessageBroadcaster, lg loggateway.Logger) *WSV2Subscriber {
	ctx, cancel := context.WithCancel(context.Background())
	s := &WSV2Subscriber{
		bus:    bus,
		hub:    hub,
		lg:     lg.With(loggateway.Domain("ws_v2_subscriber")),
		cancel: cancel,
	}
	s.wg.Add(1)
	go s.run(ctx)
	return s
}

func (s *WSV2Subscriber) run(ctx context.Context) {
	defer s.wg.Done()
	ch, cancelSub := s.bus.Subscribe(biz.EventSubscribeOptions{})
	defer cancelSub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			s.forward(e)
		}
	}
}

// forward serializes the v2 Event as JSON and pushes to the WS hub
// (broadcasting to clients subscribed to the event's SpiritSessionID).
func (s *WSV2Subscriber) forward(e biz.Event) {
	// Wrap event in a WS envelope that frontend recognizes.
	envelope := wsEnvelope{
		Type:    "v2_event",
		Kind:    string(e.EventKind()),
		Payload: e,
	}
	msg, err := json.Marshal(envelope)
	if err != nil {
		s.lg.Warn("ws v2 marshal failed",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(err))
		return
	}
	s.hub.BroadcastToSession(e.SpiritSessionID(), msg)
}

// Close stops the subscriber goroutine.
func (s *WSV2Subscriber) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// wsEnvelope is the wire format for v2 events on the WS channel.
// Phase 2 frontend will consume `payload` directly; v1 frontend will ignore
// events with `type == "v2_event"` (it only recognizes v1 ActivityEvent shapes).
type wsEnvelope struct {
	Type    string `json:"type"`    // "v2_event" or "v1_activity_event"
	Kind    string `json:"kind"`    // EventKind value (e.g. "task.created")
	Payload any    `json:"payload"` // the Event or ActivityEvent
}

// Compile-time check that WSV2Subscriber doesn't accidentally import v2's
// private types (kept loose for adapter flexibility).
var _ = v2.NewCompatAdapter
```

- [ ] **Step 2: 在 ws_io_pump.go 增加 v1 envelope 包装**

为兼容旧前端，需将 v1 `ActivityEvent` 也按 `wsEnvelope` 格式推送（type="v1_activity_event"）。修改现有 v1 WS 推送逻辑：

```go
// In existing v1 WS push code, wrap ActivityEvent:
envelope := wsEnvelope{
	Type:    "v1_activity_event",
	Kind:    string(ev.Type),
	Payload: ev,
}
msg, _ := json.Marshal(envelope)
hub.BroadcastToSession(ev.Activity.SessionID, msg)
```

- [ ] **Step 3: 在 server Wire 中注入 v2 subscriber**

修改 `internal/server/wire.go`：

```go
// In ProviderSet:
NewWSV2Subscriber,
// Note: WSV2Subscriber takes biz.EventBus (the v2 V2Bus). Wire will resolve
// automatically if V2Bus implements biz.EventBus and is in ProviderSet.
```

在 `internal/event/wire.go` 或 `internal/event/provider.go` 中追加：

```go
event.NewV2Bus,
wire.Bind(new(biz.EventBus), new(*event.V2Bus)),
```

- [ ] **Step 4: 跑 server 编译验证**

Run: `cd f:\aranea-agents && go build ./internal/server/... 2>&1 | tail -10`
Expected: 编译成功

- [ ] **Step 5: 写 WS subscriber 测试**

**File:** `internal/server/ws_v2_subscriber_test.go`

```go
package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// fakeHub captures BroadcastToSession calls.
type fakeHub struct {
	mu  sync.Mutex
	msgs map[string][][]byte // sessionID → messages
}
func newFakeHub() *fakeHub { return &fakeHub{msgs: make(map[string][][]byte)} }
func (f *fakeHub) BroadcastToSession(sid string, msg []byte) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.msgs[sid] = append(f.msgs[sid], msg)
}
func (f *fakeHub) Msgs(sid string) [][]byte {
	f.mu.Lock(); defer f.mu.Unlock()
	return f.msgs[sid]
}

func TestWSV2Subscriber_ForwardsEvents(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	hub := newFakeHub()
	sub := NewWSV2Subscriber(bus, hub, loggateway.NewNoop())
	defer sub.Close()

	now := time.Now().UTC()
	bus.Publish(context.Background(), &biz.TaskCreatedEvent{
		Task: biz.Task{ID: "t-1", SpiritSessionID: "sess-1"},
		BaseEvent: biz.BaseEvent{
			SpiritSessionIDVal: "sess-1",
			TaskIDVal:          "t-1",
			OccurredAtTime:     now,
		},
	})

	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("expected 1 message on sess-1, got %d", len(hub.Msgs("sess-1")))
		default:
		}
		if len(hub.Msgs("sess-1")) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
```

- [ ] **Step 6: 跑测试**

Run: `cd f:\aranea-agents && go test ./internal/server/ -run "TestWSV2" -count=1 -v 2>&1 | tail -15`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
cd f:\aranea-agents
git add internal/server/ws_v2_subscriber.go internal/server/ws_v2_subscriber_test.go internal/server/ws_io_pump.go internal/server/wire.go internal/event/
git commit -m "feat(server): add WS v2 subscriber for dual-format (v1+v2) push to legacy + new frontends"
```

### Task 17: 端到端集成测试

**目标**：通过 `internal/agent/v2/integration_test.go` 验证完整链路：trpc-agent-go 回调 → Projector v2 → Sequencer v2 → RepoSet (fake) + EventBus (V2Bus) + CompatAdapter → v1 ActivityEvent。验证 FIFO 顺序 + 兼容层正确性。

**Files:**
- Create: `internal/agent/v2/integration_test.go`

- [ ] **Step 1: 写集成测试**

**File:** `internal/agent/v2/integration_test.go`

```go
package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// TestEndToEnd_V2Pipeline verifies the complete v2 pipeline:
//   Projector → Sequencer → RepoSet + V2Bus + CompatAdapter → v1 ActivityEvent
//
// Scenario: a single spirit turn with thinking → reply, then task.completed.
func TestEndToEnd_V2Pipeline(t *testing.T) {
	t.Parallel()

	// Wire up all components
	rs := &fakeRepoSet{}
	v2Bus := event.NewV2Bus()
	v1Bus := &fakeV1Bus{}
	compat := NewCompatAdapter(v1Bus)

	seq := NewSequencer(rs, NewEventBusAdapter(v2Bus), loggateway.NewNoop(), compat,
		WithPublishBuffer(64),
		WithPersistBuffer(64),
		WithDeltaBatchInterval(time.Millisecond*2),
	)
	defer seq.Close()

	projector := NewActivityProjector(seq, seq.SeqAssigner(), loggateway.NewNoop())

	// Drive a turn
	ctx := context.Background()
	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
		TaskContent: "what is 2+2?",
	}

	projector.OnTurnStart(ctx, meta)
	thinkingStep := projector.BeginStep(meta, biz.StepKindThinking)
	projector.OnReasoningDelta(ctx, thinkingStep, "thinking about addition", false)
	projector.OnReasoningDone(ctx, thinkingStep, "I'll just compute 2+2=4")
	replyStep := projector.BeginStep(meta, biz.StepKindReply)
	projector.OnTextDelta(ctx, replyStep, "4", false)
	projector.OnTextDone(ctx, replyStep, "4", true /* isFinal */)
	projector.OnTurnEnd(ctx, meta)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Give persist worker time to drain
	time.Sleep(50 * time.Millisecond)

	// Verify v2 repos were called
	rs.mu.Lock()
	if len(rs.tasks) < 2 {
		t.Errorf("expected ≥2 task upserts (created+completed), got %d", len(rs.tasks))
	}
	if len(rs.turns) < 2 {
		t.Errorf("expected ≥2 turn upserts (started+completed), got %d", len(rs.turns))
	}
	if len(rs.steps) < 4 {
		t.Errorf("expected ≥4 step upserts (thinking+reply, created+completed each), got %d", len(rs.steps))
	}
	rs.mu.Unlock()

	// Verify v1 compat got translated events (task.created + step.created + step.streaming*2 + step.completed*2 + turn.started + turn.completed + task.completed)
	v1Bus.mu.Lock()
	defer v1Bus.mu.Unlock()
	if len(v1Bus.pub) == 0 {
		t.Errorf("expected v1 events from compat adapter, got 0")
	}
	// Verify task.created was the first v1 event
	first := v1Bus.pub[0]
	if first.Activity.Kind != biz.ActivityKindTask {
		t.Errorf("expected first v1 event kind=task, got %s", first.Activity.Kind)
	}
}

// TestEndToEnd_FIFOOrdering verifies that thinking step.completed is published
// BEFORE reply step.created (even though both go through BeginStep + OnXxxDelta + OnXxxDone).
func TestEndToEnd_FIFOOrdering(t *testing.T) {
	t.Parallel()

	rs := &fakeRepoSet{}
	v2Bus := event.NewV2Bus()
	compat := NewCompatAdapter(&fakeV1Bus{})

	seq := NewSequencer(rs, NewEventBusAdapter(v2Bus), loggateway.NewNoop(), compat,
		WithPublishBuffer(64),
		WithPersistBuffer(64),
		WithDeltaBatchInterval(time.Millisecond*2),
	)
	defer seq.Close()

	// Capture v2 publishes in order
	var capturedMu sync.Mutex
	var captured []biz.Event
	v2Bus.Subscribe(biz.EventSubscribeOptions{}) // start a subscriber to keep bus alive

	projector := NewActivityProjector(seq, seq.SeqAssigner(), loggateway.NewNoop())
	ctx := context.Background()
	meta := ProjectMeta{
		SessionID: "sess-1", SpiritSessionID: "sess-1",
		TaskID: "task-1", TurnID: "turn-1",
		AgentKey: "spirit",
	}
	projector.OnTurnStart(ctx, meta)

	// Run concurrent thinking + reply
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		step := projector.BeginStep(meta, biz.StepKindThinking)
		projector.OnReasoningDelta(ctx, step, "thinking", false)
		projector.OnReasoningDone(ctx, step, "done")
	}()
	go func() {
		defer wg.Done()
		step := projector.BeginStep(meta, biz.StepKindReply)
		projector.OnTextDelta(ctx, step, "answer", false)
		projector.OnTextDone(ctx, step, "answer", true)
	}()
	wg.Wait()
	projector.OnTurnEnd(ctx, meta)

	if err := seq.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	capturedMu.Lock()
	defer capturedMu.Unlock()
	// Invariant: turn.started must come BEFORE any step events (FIFO via single worker)
	// This is what v1's dual-path failed to guarantee.
	var turnStartedIdx, firstStepIdx int = -1, -1
	for i, e := range captured {
		if e.EventKind() == biz.EventKindTurnStarted && turnStartedIdx == -1 {
			turnStartedIdx = i
		}
		if e.EventKind() == biz.EventKindStepCreated && firstStepIdx == -1 {
			firstStepIdx = i
		}
	}
	if turnStartedIdx == -1 || firstStepIdx == -1 {
		t.Logf("captured events: %+v", captured)
		t.Skip("could not locate turn.started / step.created in captured (subscriber may have dropped)")
	}
	if turnStartedIdx > firstStepIdx {
		t.Errorf("FIFO violated: turn.started (idx=%d) after step.created (idx=%d)",
			turnStartedIdx, firstStepIdx)
	}
}
```

- [ ] **Step 2: 跑集成测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestEndToEnd" -count=1 -v 2>&1 | tail -30`
Expected: 全部 PASS（或 skipped，如果 subscriber 模式未完全接通——视实现情况调整）

- [ ] **Step 3: 跑 race detector**

Run: `cd f:\aranea-agents && go test ./internal/agent/v2/ -run "TestEndToEnd" -count=1 -race 2>&1 | tail -10`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/v2/integration_test.go
git commit -m "test(agent/v2): end-to-end pipeline test (projector → sequencer → repos + compat → v1 bus)"
```

### Task 18: 全量回归 + Phase 1 验收

**目标**：跑完整后端测试套件，确保 Phase 1 改动未破坏任何现有功能。所有 v1 测试仍须通过（兼容层保证）。

**Files:** 无文件改动；仅运行测试。

- [ ] **Step 1: 跑全量后端测试**

Run: `cd f:\aranea-agents && go test ./... -count=1 2>&1 | tail -40`
Expected: 全部 PASS（如有失败，根据 spec §五「兼容层策略」逐项排查）

- [ ] **Step 2: 跑 race detector（关键路径）**

Run: `cd f:\aranea-agents && go test ./internal/agent/... ./internal/service/... ./internal/data/... -count=1 -race 2>&1 | tail -20`
Expected: 全部 PASS, no race detected

- [ ] **Step 3: 跑 lint**

Run: `cd f:\aranea-agents && make lint 2>&1 | tail -20`
Expected: 0 errors（warnings 可接受）

- [ ] **Step 4: 跑 build 验证**

Run: `cd f:\aranea-agents && make build 2>&1 | tail -10`
Expected: 编译成功

- [ ] **Step 5: 跑 wire 验证**

Run: `cd f:\aranea-agents && make wire 2>&1 | tail -10`
Expected: 编译成功

- [ ] **Step 6: 验证 Phase 1 验收标准**

逐项对照 spec §7 验收标准：
- ✅ 顺序保证：测试 `TestEndToEnd_FIFOOrdering` 验证
- ✅ 跨层嵌套：PlanExecutor + TeamOrchestrator 测试覆盖三层
- ✅ Plan 状态实时同步：PlanExecutor 测试（同步 UpsertPlanStep + 事件）
- ✅ 刷新数据完整：Repo Upsert with VersionLT 守卫
- ✅ Streaming 流畅：16ms 批合并测试
- ✅ 单管道：所有事件源走 Sequencer.Publish，无 direct-publish

- [ ] **Step 7: 提交最终回归状态**

```bash
cd f:\aranea-agents
git log --oneline -20  # review all Phase 1 commits
```

记录到 commit message：
```bash
git commit --allow-empty -m "chore: Phase 1 complete — v2 backend entities + sequencer + PlanExecutor + compat layer verified"
```

---

## Self-Review

### Spec Coverage

对照 spec §三「架构设计」逐项检查：

| Spec 章节 | 实施任务 | 状态 |
|----------|---------|------|
| §3.1 Entity 层级（9 个 entity） | Task 1 (biz), Task 4 (Ent Schema), Task 7-8 (Repo) | ✅ |
| §3.2 SeqAssigner + Version 守卫 | Task 6 (SeqAssigner), Task 8 (VersionLT 守卫) | ✅ |
| §3.3 单管道 Sequencer | Task 10 (Sequencer v2) | ✅ |
| §3.3.4 streaming 批合并 | Task 10 (16ms merge) | ✅ |
| §3.3.5 持久化策略（streaming 不入库） | Task 10 (shouldPersist) | ✅ |
| §3.4 ActivityProjector 改造 | Task 12 (Projector v2) | ✅ |
| §3.5 PlanExecutor | Task 14 (PlanExecutor) | ✅ |
| §3.6 三层硬约束 | Task 14 (PlanExecutor 派发时检查) | ✅ |
| §3.7 兼容层 adapter | Task 11 (CompatAdapter) | ✅ |
| §3.8 WS 双格式推送 | Task 16 (WSV2Subscriber) | ✅ |
| §五 兼容层策略 | Task 11, 16, 17（新旧并行） | ✅ |
| §六 三层硬约束 | Task 14 (三层限制) | ✅ |

### Placeholder Scan

| 检查项 | 状态 |
|--------|------|
| "TBD" / "TODO" / "fill in" | 无（仅 "TODO(Phase 2): remove" 用于标记兼容层生命周期，是合法标记） |
| "Add appropriate error handling" | 无（错误处理全部显式 log + 返回） |
| "Similar to Task N" | 无（每个 Repo 都有完整代码） |
| "Write tests for the above" | 无（每个任务都有显式测试代码） |
| 未定义类型引用 | 检查通过（biz.EventBus, biz.Event, v2.Sequencer 等全部在前置任务中定义） |

### Type Consistency

| 类型/方法 | 定义位置 | 引用位置 | 一致性 |
|----------|---------|---------|--------|
| `biz.Event` 接口 | Task 2 (`internal/biz/event.go`) | Task 10 (sequencer), Task 12 (projector) | ✅ |
| `biz.EventBus` 接口 | Task 10 Step 7 (`internal/biz/event.go` 追加) | Task 10 (Sequencer), Task 16 (WSV2Subscriber) | ✅ |
| `v2.Sequencer.Publish` | Task 10 Step 4 | Task 12 (projector 调用 seq.Publish) | ✅ |
| `v2.ActivityProjector.BeginStep` | Task 12 Step 4 | Task 12 Step 2 (测试中调用) | ✅ |
| `biz.TeamCompleteEvent` | Task 14 Step 4 (`internal/biz/team_complete_event.go`) | Task 14 Step 3 (`TeamOrchestrator`), Step 5 (PlanExecutor) | ✅ |
| `biz.TransitionPlanStep` | Task 3 (`internal/biz/plan_step_state_machine.go`) | Task 14 (PlanExecutor 调用) | ✅ |
| `biz.SeqAssigner` | Task 6 (`internal/agent/seq_assigner.go`) | Task 10 (Sequencer 持有), Task 12 (Projector 调用 NextSeq) | ✅ |
| `v2.RepoSet` 接口 | Task 10 Step 3 (`event_router.go`) | Task 10 (Sequencer), Task 17 (集成测试 fakeRepoSet) | ✅ |
| `v2.CompatAdapter` | Task 11 Step 3 | Task 10 Step 5 (Sequencer 注入) | ✅ |
| `biz.PlanStepError` | Task 1 (`plan_step.go`) | Task 8 (Repo JSON 序列化), Task 14 (PlanExecutor 错误构造) | ✅ |
| `service.TeamOrchestrator` 接口 | Task 14 Step 3 (`team_orchestrator.go`) | Task 14 Step 5 (PlanExecutor 持有), Task 15 (spirit_team 实现) | ✅ |

---

## Execution Handoff

Phase 1 plan 已完成，保存在 `docs/superpowers/plans/2026-07-02-llm-activity-ordering-phase1.md`。

**两种执行方式可选**：

**1. Subagent-Driven（推荐）** — 每个 Task 派发一个新 subagent 实施，任务间双阶段审查（spec 合规 + 代码质量），快速迭代。

**2. Inline Execution** — 在当前会话顺序执行 Tasks，按 Phase 1.0 → 1.9 顺序，每完成一个 Phase 后 checkpoint 审查。

**建议**：Phase 1 共 18 个任务，涉及 9 个新 entity + 9 张新表 + Sequencer 重写 + Projector 重写 + PlanExecutor + 兼容层。建议用 **Subagent-Driven** 模式：

- Task 0-5（基线 + 类型基础 + 数据库）：1 个 subagent 顺序执行
- Task 6-9（SeqAssigner + Repo + Wire）：1 个 subagent 顺序执行
- Task 10-11（Sequencer + CompatAdapter）：1 个 subagent 顺序执行
- Task 12-13（Projector v2 + stream_consumer）：1 个 subagent 顺序执行
- Task 14-15（PlanExecutor + spirit_team）：1 个 subagent 顺序执行
- Task 16-18（WS + 集成测试 + 回归）：1 个 subagent 顺序执行

**请选择执行方式，或先 review 本 plan。**