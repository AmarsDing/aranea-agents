package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// GraphTaskRuntime wires graph execution events to the task board (M54).
type GraphTaskRuntime struct {
	graphUC         *biz.GraphUsecase
	taskUC          *biz.TaskUsecase
	orch            *GraphOrchestrationProjector
	webhooks        *biz.WebhookDispatcher
	dispatch        *biz.TaskDispatcher
	teamGraphResume team.TeamGraphTaskResumeHandler
	lg              loggateway.Logger
}

// SetTeamGraphTaskResumeHandler wires team graph task completion resume (M53 P1).
func (r *GraphTaskRuntime) SetTeamGraphTaskResumeHandler(h team.TeamGraphTaskResumeHandler) {
	if r == nil {
		return
	}
	r.teamGraphResume = h
}

func NewGraphTaskRuntime(
	graphUC *biz.GraphUsecase,
	taskUC *biz.TaskUsecase,
	orch *GraphOrchestrationProjector,
	webhooks *biz.WebhookDispatcher,
	lg loggateway.Logger,
) *GraphTaskRuntime {
	rt := &GraphTaskRuntime{
		graphUC:  graphUC,
		taskUC:   taskUC,
		orch:     orch,
		webhooks: webhooks,
		lg:       lg,
	}
	if taskUC != nil {
		taskUC.SetStatusPublisher(rt)
		taskUC.SetCompletionHandler(rt)
	}
	if graphUC != nil {
		graphUC.SetTaskCoordinator(rt)
	}
	rt.dispatch = biz.NewTaskDispatcher(taskUC, rt, lg)
	return rt
}

func (r *GraphTaskRuntime) Start() {
	if r == nil {
		return
	}
	if r.dispatch != nil {
		r.dispatch.Start()
	}
}

func (r *GraphTaskRuntime) Stop() {
	if r == nil || r.dispatch == nil {
		return
	}
	r.dispatch.Stop()
}

func (r *GraphTaskRuntime) PublishTaskStatus(ctx context.Context, task *biz.GraphTask, extra map[string]any) {
	if r == nil || task == nil {
		return
	}
	exec, err := r.graphUC.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		r.lg.Warn("graph task status publish failed", loggateway.StepID("graph.task_status_fail"), loggateway.Str("execution_id", task.ExecutionID), loggateway.Err(err))
		return
	}
	if r.orch != nil {
		r.orch.PublishGraphTaskStatus(ctx, exec.SessionID, task.ExecutionID, exec.GraphID, task, extra)
	}
	r.dispatchGraphTaskWebhook(ctx, exec.SessionID, exec, task, extra)
}

func (r *GraphTaskRuntime) dispatchGraphTaskWebhook(
	ctx context.Context,
	sessionID string,
	exec *biz.GraphExecution,
	task *biz.GraphTask,
	extra map[string]any,
) {
	if r == nil || r.webhooks == nil || task == nil {
		return
	}
	data := map[string]any{
		"task_id":      task.TaskID,
		"execution_id": task.ExecutionID,
		"node_id":      task.NodeID,
		"assignee":     task.Assignee,
		"summary":      task.Summary,
	}
	if exec != nil {
		data["graph_id"] = exec.GraphID
	}
	for k, v := range extra {
		data[k] = v
	}
	r.webhooks.Dispatch(
		ctx,
		biz.WebhookEventGraphTaskStatus,
		task.TaskID,
		sessionID,
		string(task.Status),
		data,
	)
}

func (r *GraphTaskRuntime) OnGraphNodeStart(ctx context.Context, exec *biz.GraphExecution, node *biz.NodeDef, meta biz.NodeTaskMeta, inputPreview string) error {
	if r == nil || r.taskUC == nil || exec == nil || node == nil {
		return nil
	}
	if !biz.ShouldCreateTaskForNode(node, meta) {
		return nil
	}
	role, mode, strategy, input := biz.GraphTaskInputFromNode(*node, meta)
	if strings.TrimSpace(inputPreview) != "" {
		input = inputPreview
	}
	_, err := r.taskUC.CreateTask(ctx, biz.CreateTaskParams{
		NodeID:             node.ID,
		ExecutionID:        exec.ID,
		RequiredRole:       role,
		AssignmentMode:     mode,
		AssignmentStrategy: strategy,
		Input:              input,
		Context:            "{}",
	})
	return err
}

func (r *GraphTaskRuntime) OnTaskCompleted(ctx context.Context, task *biz.GraphTask) error {
	if r == nil || r.graphUC == nil || task == nil || task.Status != biz.TaskStatusComplete {
		return nil
	}
	resumeValue := team.BuildTaskResumeValue(task)
	if r.teamGraphResume != nil {
		handled, err := r.teamGraphResume.HandleTeamGraphTaskCompleted(ctx, task, resumeValue)
		if handled {
			return err
		}
	}
	exec, err := r.graphUC.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return err
	}
	if exec.Status == "waiting_human" && (exec.GetInterruptNode() == task.NodeID || exec.CurrentNode == task.NodeID) {
		_, err = r.graphUC.ResumeExecution(ctx, task.ExecutionID, resumeValue)
		if err != nil {
			r.lg.Warn("graph resume after task complete failed", loggateway.StepID("graph.task_resume_fail"), loggateway.Str("execution_id", task.ExecutionID), loggateway.Str("task_id", task.TaskID), loggateway.Err(err))
		}
		return err
	}
	return nil
}

// DispatchTask records a task run dispatch (extension point for RunGateway spawn).
func (r *GraphTaskRuntime) DispatchTask(ctx context.Context, task *biz.GraphTask, agentKey string) error {
	if r == nil || r.taskUC == nil || task == nil {
		return nil
	}
	run := &biz.TaskRun{
		TaskID:    task.TaskID,
		RunID:     task.TaskID + "-dispatch-" + uuid.New().String()[:8],
		StartedAt: time.Now(),
		ExitCode:  0,
		LogRef:    "dispatch:" + agentKey,
	}
	return r.taskUC.SaveTaskRun(ctx, run)
}

// WireGraphTaskRuntime connects runtime hooks after both usecases exist.
func WireGraphTaskRuntime(
	graphUC *biz.GraphUsecase,
	taskUC *biz.TaskUsecase,
	orch *GraphOrchestrationProjector,
	linkRepo biz.TaskLinkRepo,
	webhooks *biz.WebhookDispatcher,
	teamGraphCoord *team.TeamGraphRunCoordinator,
	lg loggateway.Logger,
) *GraphTaskRuntime {
	if taskUC != nil && linkRepo != nil {
		taskUC.SetLinkRepo(linkRepo)
	}
	rt := NewGraphTaskRuntime(graphUC, taskUC, orch, webhooks, lg)
	if teamGraphCoord != nil {
		rt.SetTeamGraphTaskResumeHandler(teamGraphCoord)
	}
	rt.Start()
	return rt
}
