package service

import (
	"context"
	"testing"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- stub task repo for graph task IDOR tests ---

type idorTaskRepo struct {
	biz.TaskRepo
	tasks map[string]*biz.GraphTask
}

func (r *idorTaskRepo) GetTask(_ context.Context, id string) (*biz.GraphTask, error) {
	if t, ok := r.tasks[id]; ok {
		return t, nil
	}
	return nil, apierror.NotFound("TASK", "task not found")
}

func (r *idorTaskRepo) ListTasksByExecution(_ context.Context, executionID string, _ biz.GraphTaskStatus, _ int, _ string) ([]*biz.GraphTask, string, error) {
	var out []*biz.GraphTask
	for _, t := range r.tasks {
		if t.ExecutionID == executionID {
			out = append(out, t)
		}
	}
	return out, "", nil
}

func newIDORGraphTaskService() *GraphService {
	repo := &idorGraphRepo{defs: map[string]*biz.GraphDefinition{
		"graph-private-a": {ID: "graph-private-a", Name: "tenant A graph", WorkspaceID: "ws-a"},
		"graph-shared":    {ID: "graph-shared", Name: "shared graph", WorkspaceID: ""},
	}}
	runRepo := &idorGraphRunRepo{execs: map[string]*biz.GraphExecution{
		"exec-a1": biz.NewGraphExecution(context.Background(), "exec-a1", "graph-private-a", "sess-1", string(biz.GraphExecRunning)),
		"exec-s1": biz.NewGraphExecution(context.Background(), "exec-s1", "graph-shared", "sess-2", string(biz.GraphExecRunning)),
	}}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo:    repo,
		RunRepo: runRepo,
		Lg:      loggateway.NewNoop(),
	})
	taskRepo := &idorTaskRepo{tasks: map[string]*biz.GraphTask{
		"task-a1": {TaskID: "task-a1", ExecutionID: "exec-a1", NodeID: "n1"},
		"task-s1": {TaskID: "task-s1", ExecutionID: "exec-s1", NodeID: "n1"},
	}}
	taskUC := biz.NewTaskUsecase(taskRepo, uc, nil, loggateway.NewNoop())
	return &GraphService{uc: uc, taskUC: taskUC, lg: loggateway.NewNoop()}
}

// TestGraphTaskService_IDOR covers N4: task-plane RPCs must enforce the same
// workspace access check as definition/execution-plane RPCs (task → execution
// → graph → workspace).
func TestGraphTaskService_IDOR(t *testing.T) {
	svc := newIDORGraphTaskService()
	tenantB := wsCtx("ws-b")

	t.Run("ListTasks_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTasks(tenantB, &graphv1.ListTasksRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "ListTasks")
	})

	t.Run("GetTask_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.GetTask(tenantB, &graphv1.GetTaskRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "GetTask")
	})

	t.Run("ClaimTask_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ClaimTask(tenantB, &graphv1.ClaimTaskRequest{TaskId: "task-a1", AgentKey: "bot"})
		assertNotFound(t, err, "ClaimTask")
	})

	t.Run("SubmitTaskResult_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.SubmitTaskResult(tenantB, &graphv1.SubmitTaskResultRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "SubmitTaskResult")
	})

	t.Run("Heartbeat_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.Heartbeat(tenantB, &graphv1.HeartbeatRequest{TaskId: "task-a1", AgentKey: "bot"})
		assertNotFound(t, err, "Heartbeat")
	})

	t.Run("ReportBlocked_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ReportBlocked(tenantB, &graphv1.ReportBlockedRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "ReportBlocked")
	})

	t.Run("UnblockTask_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.UnblockTask(tenantB, &graphv1.UnblockTaskRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "UnblockTask")
	})

	t.Run("CreateTask_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.CreateTask(tenantB, &graphv1.CreateTaskRequest{ExecutionId: "exec-a1", NodeId: "n2"})
		assertNotFound(t, err, "CreateTask")
	})

	t.Run("LinkTasks_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.LinkTasks(tenantB, &graphv1.LinkTasksRequest{ParentTaskId: "task-a1", ChildTaskId: "task-s1"})
		assertNotFound(t, err, "LinkTasks(parent)")
		_, err = svc.LinkTasks(tenantB, &graphv1.LinkTasksRequest{ParentTaskId: "task-s1", ChildTaskId: "task-a1"})
		assertNotFound(t, err, "LinkTasks(child)")
	})

	t.Run("UnlinkTasks_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.UnlinkTasks(tenantB, &graphv1.UnlinkTasksRequest{ParentTaskId: "task-a1", ChildTaskId: "task-s1"})
		assertNotFound(t, err, "UnlinkTasks(parent)")
		_, err = svc.UnlinkTasks(tenantB, &graphv1.UnlinkTasksRequest{ParentTaskId: "task-s1", ChildTaskId: "task-a1"})
		assertNotFound(t, err, "UnlinkTasks(child)")
	})

	t.Run("ReviewTask_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ReviewTask(tenantB, &graphv1.ReviewTaskRequest{TaskId: "task-a1", Approved: true})
		assertNotFound(t, err, "ReviewTask")
	})

	t.Run("ListTaskComments_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskComments(tenantB, &graphv1.ListTaskCommentsRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "ListTaskComments")
	})

	t.Run("AddTaskComment_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.AddTaskComment(tenantB, &graphv1.AddTaskCommentRequest{TaskId: "task-a1", Content: "x"})
		assertNotFound(t, err, "AddTaskComment")
	})

	t.Run("ListTaskLogs_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskLogs(tenantB, &graphv1.ListTaskLogsRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "ListTaskLogs")
	})

	t.Run("ListTaskRuns_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskRuns(tenantB, &graphv1.ListTaskRunsRequest{TaskId: "task-a1"})
		assertNotFound(t, err, "ListTaskRuns")
	})

	t.Run("ListTaskEvents_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskEvents(tenantB, &graphv1.ListTaskEventsRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "ListTaskEvents")
	})

	t.Run("GetTask_OwnerWorkspace_Allowed", func(t *testing.T) {
		resp, err := svc.GetTask(wsCtx("ws-a"), &graphv1.GetTaskRequest{TaskId: "task-a1"})
		if err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
		if resp.GetTask().GetTaskId() != "task-a1" {
			t.Fatalf("unexpected task id: %s", resp.GetTask().GetTaskId())
		}
	})

	t.Run("GetTask_SharedGraph_Allowed", func(t *testing.T) {
		_, err := svc.GetTask(tenantB, &graphv1.GetTaskRequest{TaskId: "task-s1"})
		if err != nil {
			t.Fatalf("shared graph task should be visible to any tenant: %v", err)
		}
	})

	t.Run("ListTasks_OwnerWorkspace_Allowed", func(t *testing.T) {
		_, err := svc.ListTasks(wsCtx("ws-a"), &graphv1.ListTasksRequest{ExecutionId: "exec-a1"})
		if err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
	})
}
