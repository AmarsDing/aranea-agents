package tools

import (
	"aranea-agents/pkg/loggateway"

	taskrunruntime "trpc.group/trpc-go/trpc-agent-go/agent/taskrun"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpctaskrun "trpc.group/trpc-go/trpc-agent-go/tool/taskrun"
)

// TECH-DEBT(B-7): TaskRunAdapter 已实现但未接入生产路径。
// 阻塞类型：缺少核心依赖 + 复杂依赖链。
// 适配器需要 taskrun.Controller 实例管理异步任务生命周期，但项目缺少：
//   1. 任务持久化层（存储任务状态和结果）
//   2. 任务生命周期管理器（状态机：pending → running → completed/failed/cancelled）
//   3. 任务状态追踪（per-session、per-run 的任务索引）
//   4. 任务结果回调机制
// 解除方案：Phase 1 统一执行引擎实施时解除（唯一有明确解除路径的阻塞项）。
// 详见 docs/trpc-agent-go/alignment-plan.md §十一/B-7。

// TaskRunAdapter wraps the framework's taskrun tools to allow Agent async
// delegation while coexisting with the project's existing tool system.
//
// The framework's taskrun package provides tools for spawning, listing,
// monitoring, and canceling background task runs. This adapter bridges
// those tools into the project's tool registry so agents can delegate
// subtasks to background workers.
type TaskRunAdapter struct {
	tools      trpctaskrun.Tools
	controller taskrunruntime.Controller
	lg         loggateway.Logger
}

// TaskRunConfig holds the configuration for creating a TaskRunAdapter.
type TaskRunConfig struct {
	// DefaultAgentName is the agent selected by spawn when the caller
	// does not provide one.
	DefaultAgentName string
	// RuntimeState merges static runtime state into each spawned run.
	RuntimeState map[string]any
	// AllowNested enables nested task runs (a task run spawning another).
	AllowNested bool
	// PropagateParentAppName copies the current invocation app name
	// into spawned runs.
	PropagateParentAppName bool
}

// NewTaskRunAdapter creates a TaskRunAdapter that wraps the framework's
// taskrun tools with the given controller and configuration.
func NewTaskRunAdapter(
	controller taskrunruntime.Controller,
	sessionSvc trpcsession.Service,
	cfg TaskRunConfig,
	lg loggateway.Logger,
) *TaskRunAdapter {
	opts := []trpctaskrun.Option{
		trpctaskrun.WithSessionService(sessionSvc),
	}
	if cfg.DefaultAgentName != "" {
		opts = append(opts, trpctaskrun.WithDefaultAgentName(cfg.DefaultAgentName))
	}
	if len(cfg.RuntimeState) > 0 {
		opts = append(opts, trpctaskrun.WithRuntimeState(cfg.RuntimeState))
	}
	if cfg.AllowNested {
		opts = append(opts, trpctaskrun.WithNestedSpawns(true))
	}
	if cfg.PropagateParentAppName {
		opts = append(opts, trpctaskrun.WithParentAppNamePropagation(true))
	}

	tools := trpctaskrun.NewTools(controller, opts...)
	lg.Info("TaskRun 适配器创建成功",
		loggateway.StepID("taskrun.create"),
		loggateway.Str("default_agent", cfg.DefaultAgentName))
	return &TaskRunAdapter{
		tools:      tools,
		controller: controller,
		lg:         lg,
	}
}

// Tools returns all taskrun tool declarations for registration with an agent.
// The returned slice contains: start_task_run, list_task_runs,
// get_task_run, cancel_task_run, wait_task_run, and optionally
// read_task_run_transcript (when session service is available).
func (a *TaskRunAdapter) Tools() []trpctool.Tool {
	if a == nil {
		return nil
	}
	return a.tools.All()
}

// SetController updates the controller used by all taskrun tools.
// This is useful when the controller is initialized after the adapter
// is created (e.g., during lazy initialization).
func (a *TaskRunAdapter) SetController(controller taskrunruntime.Controller) {
	if a == nil {
		return
	}
	a.controller = controller
	a.tools.SetController(controller)
}

// Controller returns the current taskrun controller.
func (a *TaskRunAdapter) Controller() taskrunruntime.Controller {
	if a == nil {
		return nil
	}
	return a.controller
}
