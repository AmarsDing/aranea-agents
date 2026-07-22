package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// L3 (2026-07-22)：interrupted task 的显式续跑。
//
// 进程突然关闭时，启动恢复把 in-flight task 终态化为 interrupted（可续跑），
// 其余 v2 实体为 failed。用户在 UI 点「继续执行」触发本方法：
//
//  1. 预检（存在性 / session 归属 / interrupted 状态 / 活跃 run 冲突）
//  2. CAS：interrupted → running（防双击/并发复活两次）
//  3. 从持久化 step 组装紧凑执行轨迹（BuildTaskResumeTrace）
//  4. 异步重跑：RunNativeTurn(ParentTaskID=taskID)——projector 把新 turn
//     attach 到原 task（system-push continuation 语义），完成时由
//     CompleteTaskTerminal 终态化，不受事件 version 影响。
//
// 上下文处理：崩溃前的 LLM 对话历史仍在 session 消息表中（turn 持久化与
// 进程同生共死），重跑的 turn 自然加载；执行轨迹作为 system 前缀注入
// content，agent 据此跳过已完成步骤继续未完成工作。

// ResumeInterruptedTask resumes an interrupted task by rerunning it with the
// persisted execution trace injected (L3). Synchronous part validates and
// claims the task (CAS); the rerun itself is asynchronous.
func (s *ChatService) ResumeInterruptedTask(ctx context.Context, sessionID, taskID string) error {
	task, content, err := s.prepareInterruptedResume(ctx, sessionID, taskID)
	if err != nil {
		return err
	}
	s.startInterruptedResume(task, content)
	return nil
}

// prepareInterruptedResume validates the request and atomically claims the
// task (interrupted → running). Returns the running task and the resume
// content (original message + execution trace).
func (s *ChatService) prepareInterruptedResume(ctx context.Context, sessionID, taskID string) (biz.Task, string, error) {
	sessionID = strings.TrimSpace(sessionID)
	taskID = strings.TrimSpace(taskID)
	if sessionID == "" || taskID == "" {
		return biz.Task{}, "", apierror.BadRequest(apierror.DomainChat, "session_id and task_id are required")
	}
	if s.taskV2 == nil || s.stepReader == nil {
		return biz.Task{}, "", apierror.Internal(apierror.DomainChat, "task v2 repo unavailable for resume")
	}

	// Pre-check: existence, ownership, state (fast reject before the CAS).
	task, err := s.taskV2.GetTask(ctx, taskID)
	if err != nil {
		return biz.Task{}, "", err
	}
	if task.SessionID != sessionID {
		return biz.Task{}, "", apierror.BadRequest(apierror.DomainChat, "task %s does not belong to session %s", taskID, sessionID)
	}
	if task.Status != biz.TaskStatusInterrupted {
		return biz.Task{}, "", apierror.Conflict(apierror.DomainChat, "task %s is %s, not interrupted", taskID, task.Status)
	}
	if s.orch != nil && s.orch.HasActiveRun(sessionID) {
		return biz.Task{}, "", apierror.Conflict(apierror.DomainChat, "session %s has an active run", sessionID)
	}

	// CAS claim: a concurrent click or double submit loses here.
	claimed, ok, err := s.taskV2.ResumeInterruptedTask(ctx, taskID, time.Now().UTC())
	if err != nil {
		return biz.Task{}, "", err
	}
	if !ok {
		return biz.Task{}, "", apierror.Conflict(apierror.DomainChat, "task %s was already resumed", taskID)
	}

	// Build the execution trace from persisted steps. A read failure degrades
	// to an empty trace (plain rerun) rather than blocking the resume.
	var trace string
	steps, stepErr := s.stepReader.ListStepsByTask(ctx, taskID)
	if stepErr != nil {
		s.lg.Warn("interrupted resume: list steps failed, degrade to plain rerun",
			loggateway.StepID("chat.interrupted_resume.steps"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(stepErr))
	} else {
		trace = biz.BuildTaskResumeTrace(steps)
	}
	return claimed, biz.InterruptedResumeUserContent(task.UserMessage, trace), nil
}

// startInterruptedResume publishes the running state and reruns the task
// asynchronously with the resume content. On turn failure the task is
// terminalized to failed via the sequencer (persist + WS).
func (s *ChatService) startInterruptedResume(task biz.Task, content string) {
	if s.orch == nil {
		s.lg.Warn("interrupted resume: orchestrator unavailable, task left running",
			loggateway.StepID("chat.interrupted_resume.no_orch"),
			loggateway.Str("task_id", task.ID))
		return
	}
	if s.orch.v2Seq != nil {
		s.orch.v2Seq.Publish(context.Background(), biz.NewTaskUpdatedEvent(task))
	}
	if s.sessions != nil {
		if err := s.sessions.TransitionStatus(context.Background(), task.SessionID, sessstatus.SessionStatusRunning, ""); err != nil {
			s.lg.Warn("interrupted resume: session transition to running failed",
				loggateway.StepID("chat.interrupted_resume.session"),
				loggateway.Str("session_id", task.SessionID),
				loggateway.Err(err))
		}
	}

	deadline := time.Duration(biz.DefaultDurableDeadlineSec()) * time.Second
	runCtx, cancel := context.WithTimeout(context.Background(), deadline)
	safego.Go(runCtx, "interrupted-task-resume", func() {
		defer cancel()
		req := biz.TurnInput{
			SessionID:    task.SessionID,
			Content:      content,
			ParentTaskID: task.ID,
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint: biz.EntryPointWeb,
				AllowQueue: false,
			},
		}
		_, _, turnErr := s.RunNativeTurn(runCtx, req)
		if turnErr == nil {
			return
		}
		s.lg.Warn("interrupted resume: rerun failed",
			loggateway.StepID("chat.interrupted_resume.rerun"),
			loggateway.Str("task_id", task.ID),
			loggateway.Err(turnErr))
		if s.orch.v2Seq != nil {
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer persistCancel()
			now := time.Now()
			s.orch.v2Seq.Publish(persistCtx, biz.NewTaskFailedEvent(biz.Task{
				ID:          task.ID,
				SessionID:   task.SessionID,
				Status:      biz.TaskStatusFailed,
				CompletedAt: &now,
			}))
		}
	})
}
