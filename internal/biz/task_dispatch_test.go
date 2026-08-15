package biz

import (
	"context"
	"testing"
	"time"
)

type stubTaskRepo struct {
	tasks map[string]*GraphTask
}

func (s *stubTaskRepo) SaveTask(context.Context, *GraphTask) error { return nil }
func (s *stubTaskRepo) GetTask(_ context.Context, taskID string) (*GraphTask, error) {
	if t, ok := s.tasks[taskID]; ok {
		return t, nil
	}
	return nil, errTaskNotFound(taskID)
}
func (s *stubTaskRepo) GetActiveTaskByExecutionNode(context.Context, string, string) (*GraphTask, error) {
	return nil, errTaskNotFound("")
}
func (s *stubTaskRepo) ListTasksByExecution(context.Context, string, GraphTaskStatus, int, string) ([]*GraphTask, string, error) {
	return nil, "", nil
}
func (s *stubTaskRepo) ListTasksByStatuses(context.Context, []GraphTaskStatus, int) ([]*GraphTask, error) {
	return nil, nil
}
func (s *stubTaskRepo) GetTasksByIDs(_ context.Context, ids []string) ([]*GraphTask, error) {
	var result []*GraphTask
	for _, id := range ids {
		if t, ok := s.tasks[id]; ok {
			result = append(result, t)
		}
	}
	return result, nil
}
func (s *stubTaskRepo) UpdateTask(context.Context, *GraphTask) error { return nil }
func (s *stubTaskRepo) BatchUpdateGraphTaskStatus(context.Context, []string, GraphTaskStatus) error {
	return nil
}
func (s *stubTaskRepo) ClaimTaskWhereStatus(_ context.Context, taskID string, agentKey string, _ []GraphTaskStatus) (*GraphTask, bool, error) {
	if t, ok := s.tasks[taskID]; ok && t.Status == GraphTaskStatusPending {
		now := time.Now()
		t.Assignee = agentKey
		t.Status = GraphTaskStatusClaimed
		t.ClaimedAt = &now
		return t, true, nil
	}
	return nil, false, nil
}
func (s *stubTaskRepo) CompleteTaskWhereStatus(_ context.Context, taskID string, submitter string, output string, summary string, metadata string, toStatus GraphTaskStatus) (*GraphTask, bool, error) {
	if t, ok := s.tasks[taskID]; ok && (t.Status == GraphTaskStatusClaimed || t.Status == GraphTaskStatusReviewRequired) {
		if submitter != "" && t.Assignee != submitter {
			return nil, false, nil
		}
		now := time.Now()
		t.Output = output
		t.Summary = summary
		t.Metadata = metadata
		t.CompletedAt = &now
		t.Status = toStatus
		return t, true, nil
	}
	return nil, false, nil
}
func (s *stubTaskRepo) TransitionTaskStatusWhere(_ context.Context, taskID string, fromStatuses []GraphTaskStatus, toStatus GraphTaskStatus) (*GraphTask, bool, error) {
	if t, ok := s.tasks[taskID]; ok {
		for _, from := range fromStatuses {
			if t.Status == from {
				t.Status = toStatus
				return t, true, nil
			}
		}
	}
	return nil, false, nil
}
func (s *stubTaskRepo) SaveTaskComment(context.Context, *TaskComment) error { return nil }
func (s *stubTaskRepo) ListTaskComments(context.Context, string) ([]*TaskComment, error) {
	return nil, nil
}
func (s *stubTaskRepo) SaveTaskLog(context.Context, *TaskLog) error { return nil }
func (s *stubTaskRepo) ListTaskLogs(context.Context, string, string, string, int) ([]*TaskLog, error) {
	return nil, nil
}
func (s *stubTaskRepo) SaveTaskRun(context.Context, *TaskRun) error { return nil }
func (s *stubTaskRepo) ListTaskRuns(context.Context, string) ([]*TaskRun, error) {
	return nil, nil
}
func (s *stubTaskRepo) SaveTaskEvent(context.Context, *TaskEvent) error { return nil }
func (s *stubTaskRepo) ListTaskEvents(context.Context, string, string, string, int) ([]*TaskEvent, error) {
	return nil, nil
}

type stubTaskLinkRepo struct {
	parents map[string][]*TaskLink
}

func (s *stubTaskLinkRepo) SaveLink(context.Context, *TaskLink) error        { return nil }
func (s *stubTaskLinkRepo) DeleteLink(context.Context, string, string) error { return nil }
func (s *stubTaskLinkRepo) ListParentLinks(_ context.Context, childTaskID string) ([]*TaskLink, error) {
	return s.parents[childTaskID], nil
}
func (s *stubTaskLinkRepo) ListChildLinks(context.Context, string) ([]*TaskLink, error) {
	return nil, nil
}
func (s *stubTaskLinkRepo) ListParentLinksByChildren(_ context.Context, childIDs []string) ([]*TaskLink, error) {
	var result []*TaskLink
	for _, id := range childIDs {
		result = append(result, s.parents[id]...)
	}
	return result, nil
}

func errTaskNotFound(id string) error {
	return &taskNotFoundErr{id: id}
}

type taskNotFoundErr struct{ id string }

func (e *taskNotFoundErr) Error() string { return "task not found: " + e.id }

func TestAllParentTasksComplete(t *testing.T) {
	repo := &stubTaskRepo{tasks: map[string]*GraphTask{
		"parent-done": {TaskID: "parent-done", Status: GraphTaskStatusComplete},
		"parent-open": {TaskID: "parent-open", Status: GraphTaskStatusPending},
		"child":       {TaskID: "child", Status: GraphTaskStatusPending},
	}}
	links := &stubTaskLinkRepo{parents: map[string][]*TaskLink{
		"child": {
			{ParentTaskID: "parent-done", ChildTaskID: "child"},
			{ParentTaskID: "parent-open", ChildTaskID: "child"},
		},
	}}
	uc := &TaskUsecase{reader: repo, writer: repo, comments: repo, logs: repo, runs: repo, events: repo, linkRepo: links}

	ready, err := uc.allParentTasksComplete(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ready {
		t.Fatal("expected child blocked by incomplete parent")
	}

	links.parents["child"] = []*TaskLink{{ParentTaskID: "parent-done", ChildTaskID: "child"}}
	ready, err = uc.allParentTasksComplete(context.Background(), "child")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ready {
		t.Fatal("expected child ready when sole parent complete")
	}
}

func TestIsTaskReadyForDispatch(t *testing.T) {
	repo := &stubTaskRepo{tasks: map[string]*GraphTask{
		"parent": {TaskID: "parent", Status: GraphTaskStatusPending},
		"child":  {TaskID: "child", Status: GraphTaskStatusPending},
	}}
	links := &stubTaskLinkRepo{parents: map[string][]*TaskLink{
		"child": {{ParentTaskID: "parent", ChildTaskID: "child"}},
	}}
	uc := &TaskUsecase{reader: repo, writer: repo, comments: repo, logs: repo, runs: repo, events: repo, linkRepo: links}
	task := &GraphTask{TaskID: "child", CreatedAt: time.Now()}

	if uc.IsTaskReadyForDispatch(context.Background(), task) {
		t.Fatal("dispatch should wait for parent completion")
	}
}
