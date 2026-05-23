package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
)

// GraphTaskRuntime wires graph execution events to the task board (M54).
type GraphTaskRuntime struct {
	graphUC  *biz.GraphUsecase
	taskUC   *biz.TaskUsecase
	orch     *GraphOrchestrationProjector
	webhooks *biz.WebhookDispatcher
	dispatch *biz.TaskDispatcher
	log      *log.Helper
}

func NewGraphTaskRuntime(
	graphUC *biz.GraphUsecase,
	taskUC *biz.TaskUsecase,
	orch *GraphOrchestrationProjector,
	webhooks *biz.WebhookDispatcher,
) *GraphTaskRuntime {
	rt := &GraphTaskRuntime{
		graphUC:  graphUC,
		taskUC:   taskUC,
		orch:     orch,
		webhooks: webhooks,
		log:      log.NewHelper(log.DefaultLogger),
	}
	if taskUC != nil {
		taskUC.SetStatusPublisher(rt)
		taskUC.SetCompletionHandler(rt)
	}
	if graphUC != nil {
		graphUC.SetTaskCoordinator(rt)
	}
	rt.dispatch = biz.NewTaskDispatcher(taskUC, rt)
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
		r.log.Warnf("graph task status publish: execution=%s: %v", task.ExecutionID, err)
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

func (r *GraphTaskRuntime) OnGraphNodeStart(ctx context.Context, exec *biz.GraphExecution, node *biz.NodeDef, inputPreview string) error {
	if r == nil || r.taskUC == nil || exec == nil || node == nil {
		return nil
	}
	if !biz.ShouldCreateTaskForNode(node) {
		return nil
	}
	mode := strings.TrimSpace(node.AssignmentMode)
	if mode == "" {
		mode = "static"
	}
	strategy := strings.TrimSpace(node.AssignmentStrategy)
	input := inputPreview
	if input == "" {
		input = node.Description
	}
	_, err := r.taskUC.CreateTask(ctx, node.ID, exec.ID, node.RequiredRole, mode, strategy, input, "{}")
	return err
}

func (r *GraphTaskRuntime) OnTaskCompleted(ctx context.Context, task *biz.GraphTask) error {
	if r == nil || r.graphUC == nil || task == nil || task.Status != biz.TaskStatusComplete {
		return nil
	}
	exec, err := r.graphUC.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return err
	}
	resumeValue := map[string]any{
		"task_id": task.TaskID,
		"node_id": task.NodeID,
		"output":  task.Output,
		"summary": task.Summary,
	}
	if b, err := json.Marshal(resumeValue); err == nil {
		resumeValue["task_result_json"] = string(b)
	}
	if exec.Status == "waiting_human" && (exec.InterruptNode == task.NodeID || exec.CurrentNode == task.NodeID) {
		_, err = r.graphUC.ResumeExecution(ctx, task.ExecutionID, resumeValue)
		if err != nil {
			r.log.Warnf("graph resume after task complete: execution=%s task=%s: %v", task.ExecutionID, task.TaskID, err)
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
) *GraphTaskRuntime {
	if taskUC != nil && linkRepo != nil {
		taskUC.SetLinkRepo(linkRepo)
	}
	rt := NewGraphTaskRuntime(graphUC, taskUC, orch, webhooks)
	rt.Start()
	return rt
}
