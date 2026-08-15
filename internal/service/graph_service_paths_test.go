package service

import (
	"context"
	"errors"
	"testing"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubGraphRuntime is a no-op GraphRuntime for service-layer path tests.
type stubGraphRuntime struct{ lineage string }

func (r *stubGraphRuntime) Run(_ context.Context, _ map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	ch := make(chan biz.GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *stubGraphRuntime) Resume(_ context.Context, _ string, _ map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	ch := make(chan biz.GraphRuntimeEvent)
	close(ch)
	return ch, nil
}
func (r *stubGraphRuntime) Cancel() error        { return nil }
func (r *stubGraphRuntime) GetLineageID() string { return r.lineage }
func (r *stubGraphRuntime) TimeTravelGetState(context.Context, string, string, string) (*biz.GraphCheckpointState, error) {
	return &biz.GraphCheckpointState{}, nil
}
func (r *stubGraphRuntime) TimeTravelHistory(context.Context, string, string, int) (biz.GraphCheckpointList, error) {
	return nil, nil
}
func (r *stubGraphRuntime) TimeTravelEditState(context.Context, string, string, string, map[string]any) (*biz.GraphEditedState, error) {
	return &biz.GraphEditedState{Ref: biz.GraphCheckpointRef{CheckpointID: "cp-new"}}, nil
}
func (r *stubGraphRuntime) ListCheckpoints(context.Context, string, string, int) (biz.GraphCheckpointList, error) {
	return nil, nil
}

var _ biz.GraphRuntime = (*stubGraphRuntime)(nil)

type stubGraphFactory struct {
	validateResult *biz.GraphValidationResult
	buildErr       error
	resumeErr      error
}

func (f *stubGraphFactory) BuildAndRun(_ context.Context, _ biz.GraphBuildConfig, _, _, _, _ string, _ map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	if f.buildErr != nil {
		return nil, nil, f.buildErr
	}
	ch := make(chan biz.GraphRuntimeEvent)
	close(ch)
	return &stubGraphRuntime{lineage: "lin-start"}, ch, nil
}
func (f *stubGraphFactory) BuildAndResume(_ context.Context, _ biz.GraphBuildConfig, _, _, _, _, _ string, _ map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	if f.resumeErr != nil {
		return nil, nil, f.resumeErr
	}
	ch := make(chan biz.GraphRuntimeEvent)
	close(ch)
	return &stubGraphRuntime{lineage: "lin-resume"}, ch, nil
}
func (f *stubGraphFactory) BuildRuntime(context.Context, biz.GraphBuildConfig, string, string, string, string, string) (biz.GraphRuntime, error) {
	return &stubGraphRuntime{lineage: "lin-rt"}, nil
}
func (f *stubGraphFactory) Visualize(context.Context, biz.GraphBuildConfig) (*biz.GraphVisualization, error) {
	return &biz.GraphVisualization{}, nil
}
func (f *stubGraphFactory) Validate(context.Context, biz.GraphBuildConfig) (*biz.GraphValidationResult, error) {
	if f.validateResult != nil {
		return f.validateResult, nil
	}
	return &biz.GraphValidationResult{}, nil
}
func (f *stubGraphFactory) ListTemplates() []biz.GraphTemplateRef { return nil }
func (f *stubGraphFactory) GetTemplate(string) (biz.GraphTemplateRef, bool) {
	return biz.GraphTemplateRef{}, false
}
func (f *stubGraphFactory) TemplateToDef(biz.GraphTemplateRef, string, string) *biz.GraphDefinition {
	return &biz.GraphDefinition{}
}
func (f *stubGraphFactory) AgentExists(context.Context, string) bool { return false }
func (f *stubGraphFactory) FindNodeDef(biz.GraphBuildConfig, map[string]biz.NodeTaskMeta, string) *biz.NodeTaskMeta {
	return nil
}

var _ biz.GraphBuilderFactory = (*stubGraphFactory)(nil)

type pathTaskRepo struct {
	biz.TaskRepo
	tasks map[string]*biz.GraphTask
}

func (r *pathTaskRepo) GetTask(_ context.Context, id string) (*biz.GraphTask, error) {
	if t, ok := r.tasks[id]; ok {
		return t, nil
	}
	return nil, biz.ErrNotFound
}
func (r *pathTaskRepo) UpdateTask(_ context.Context, task *biz.GraphTask) error {
	r.tasks[task.TaskID] = task
	return nil
}

// CompleteTaskWhereStatus 内存版原子提交：与 data 层 CAS 语义一致，供
// SubmitTaskResult 测试路径使用（嵌入的 biz.TaskRepo 为 nil，未实现会 panic）。
func (r *pathTaskRepo) CompleteTaskWhereStatus(_ context.Context, taskID string, submitter string, output string, summary string, metadata string, toStatus biz.GraphTaskStatus) (*biz.GraphTask, bool, error) {
	t, ok := r.tasks[taskID]
	if !ok || (t.Status != biz.GraphTaskStatusClaimed && t.Status != biz.GraphTaskStatusReviewRequired) {
		return nil, false, nil
	}
	if submitter != "" && t.Assignee != submitter {
		return nil, false, nil
	}
	t.Output = output
	t.Summary = summary
	t.Metadata = metadata
	t.Status = toStatus
	return t, true, nil
}

func (r *pathTaskRepo) SaveTaskEvent(context.Context, *biz.TaskEvent) error { return nil }

func newPathGraphService(factory biz.GraphBuilderFactory, extra ...func(*pathGraphSetup)) *GraphService {
	setup := &pathGraphSetup{
		defs: map[string]*biz.GraphDefinition{
			"graph-path-a": {
				ID:          "graph-path-a",
				Name:        "path graph",
				WorkspaceID: "ws-a",
				EntryPoint:  "n1",
				FinishPoint: "n1",
				Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "agent-a"}},
			},
		},
		execs: map[string]*biz.GraphExecution{
			"exec-hitl-1": biz.NewGraphExecution(context.Background(), "exec-hitl-1", "graph-path-a", "sess-1", string(biz.GraphExecWaitingHuman)),
			"exec-run-1":  biz.NewGraphExecution(context.Background(), "exec-run-1", "graph-path-a", "sess-1", string(biz.GraphExecRunning)),
		},
		tasks: map[string]*biz.GraphTask{
			"task-hitl-1": {TaskID: "task-hitl-1", ExecutionID: "exec-hitl-1", NodeID: "n1", Status: biz.GraphTaskStatusClaimed, Assignee: "bot"},
		},
	}
	for _, fn := range extra {
		fn(setup)
	}
	repo := &idorGraphRepo{defs: setup.defs}
	runRepo := &idorGraphRunRepo{execs: setup.execs}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo:    repo,
		RunRepo: runRepo,
		Factory: factory,
		Lg:      loggateway.NewNoop(),
	})
	taskUC := biz.NewTaskUsecase(&pathTaskRepo{tasks: setup.tasks}, uc, nil, loggateway.NewNoop())
	return &GraphService{uc: uc, taskUC: taskUC, lg: loggateway.NewNoop()}
}

type pathGraphSetup struct {
	defs  map[string]*biz.GraphDefinition
	execs map[string]*biz.GraphExecution
	tasks map[string]*biz.GraphTask
}

func TestGraphService_ValidateGraph_CompileFailureMapped(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{validateResult: &biz.GraphValidationResult{
		Errors: []biz.GraphValidationIssue{{Code: "empty_graph", Message: "Graph 必须包含至少一个节点"}},
	}})
	resp, err := svc.ValidateGraph(wsCtx("ws-a"), &graphv1.ValidateGraphRequest{GraphId: "graph-path-a"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetValid() {
		t.Fatal("compile/validation errors must map to Valid=false")
	}
	if len(resp.Errors) == 0 || resp.Errors[0].Code != "empty_graph" {
		t.Fatalf("errors=%v", resp.Errors)
	}
}

func TestGraphService_ExecuteGraph_EmptyID(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{})
	if _, err := svc.ExecuteGraph(wsCtx("ws-a"), &graphv1.ExecuteGraphRequest{}); err == nil {
		t.Fatal("empty graph_id must error")
	}
}

func TestGraphService_ExecuteGraph_Start(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{})
	resp, err := svc.ExecuteGraph(wsCtx("ws-a"), &graphv1.ExecuteGraphRequest{GraphId: "graph-path-a", SessionId: "sess-1"})
	if err != nil {
		t.Fatalf("ExecuteGraph start: %v", err)
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("execution_id must be returned")
	}
	if resp.GetStatus() == "" {
		t.Fatal("status must be returned")
	}
}

func TestGraphService_ExecuteGraph_BuildFailureIsObservable(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{buildErr: errors.New("compile failed: missing agent")})
	_, err := svc.ExecuteGraph(wsCtx("ws-a"), &graphv1.ExecuteGraphRequest{GraphId: "graph-path-a", SessionId: "sess-1"})
	if err == nil {
		t.Fatal("build/compile failure must surface to the caller, not be swallowed")
	}
}

func TestGraphService_ResumeGraph_HITL(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{})
	resp, err := svc.ResumeGraph(wsCtx("ws-a"), &graphv1.ResumeGraphRequest{ExecutionId: "exec-hitl-1"})
	if err != nil {
		t.Fatalf("ResumeGraph HITL: %v", err)
	}
	if resp.GetExecutionId() != "exec-hitl-1" {
		t.Fatalf("execution_id=%q", resp.GetExecutionId())
	}
	if resp.GetStatus() == "" {
		t.Fatal("status must be returned after HITL resume")
	}
}

func TestGraphService_ResumeGraph_BuildFailureIsObservable(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{resumeErr: errors.New("resume compile failed")})
	if _, err := svc.ResumeGraph(wsCtx("ws-a"), &graphv1.ResumeGraphRequest{ExecutionId: "exec-hitl-1"}); err == nil {
		t.Fatal("HITL resume compile failure must surface")
	}
}

func TestGraphService_CancelGraphExecution(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{})
	resp, err := svc.CancelGraphExecution(wsCtx("ws-a"), &graphv1.CancelGraphExecutionRequest{ExecutionId: "exec-run-1"})
	if err != nil {
		t.Fatalf("CancelGraphExecution: %v", err)
	}
	if resp.GetStatus() != string(biz.GraphExecCancelled) {
		t.Fatalf("status=%q want cancelled", resp.GetStatus())
	}
}

func TestGraphService_SubmitTaskResult_HITLWriteback(t *testing.T) {
	svc := newPathGraphService(&stubGraphFactory{})
	resp, err := svc.SubmitTaskResult(wsCtx("ws-a"), &graphv1.SubmitTaskResultRequest{
		TaskId:  "task-hitl-1",
		Output:  `{"approved":true}`,
		Summary: "ok",
	})
	if err != nil {
		t.Fatalf("SubmitTaskResult: %v", err)
	}
	if resp.GetTask() == nil {
		t.Fatal("task must be returned")
	}
	if resp.GetTask().GetStatus() != graphv1.TaskStatus_TASK_COMPLETE {
		t.Fatalf("status=%v want COMPLETE (HITL writeback)", resp.GetTask().GetStatus())
	}
}
