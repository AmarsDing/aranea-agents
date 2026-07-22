package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubTaskV2RepoForResume embeds biz.TaskV2Repo so only the methods exercised
// by ResumeInterruptedTask need to be overridden.
type stubTaskV2RepoForResume struct {
	biz.TaskV2Repo
	task       biz.Task
	getErr     error
	resumeTask biz.Task
	resumeOK   bool
	resumeErr  error
	resumeN    int
}

func (s *stubTaskV2RepoForResume) GetTask(_ context.Context, id string) (biz.Task, error) {
	if s.getErr != nil {
		return biz.Task{}, s.getErr
	}
	if s.task.ID != id {
		return biz.Task{}, apierror.NotFound("TASK_V2", "task not found")
	}
	return s.task, nil
}

func (s *stubTaskV2RepoForResume) ResumeInterruptedTask(_ context.Context, id string, resumeAt time.Time) (biz.Task, bool, error) {
	s.resumeN++
	if s.resumeErr != nil || !s.resumeOK {
		return biz.Task{}, s.resumeOK, s.resumeErr
	}
	return s.resumeTask, true, nil
}

// stubStepV2ReaderForResume returns a fixed step list.
type stubStepV2ReaderForResume struct {
	biz.StepV2Reader
	steps []biz.Step
	err   error
}

func (s *stubStepV2ReaderForResume) ListStepsByTask(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, s.err
}

func newResumeTestService(taskV2 biz.TaskV2Repo, steps biz.StepV2Reader) *ChatService {
	return &ChatService{
		taskV2:     taskV2,
		stepReader: steps,
		lg:         loggateway.NewNoop(),
	}
}

func TestResumeInterruptedTask_EmptyArgs(t *testing.T) {
	svc := newResumeTestService(nil, nil)
	if err := svc.ResumeInterruptedTask(context.Background(), " ", "t-1"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty session: %v", err)
	}
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", ""); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty task: %v", err)
	}
}

func TestResumeInterruptedTask_NilDeps(t *testing.T) {
	svc := newResumeTestService(nil, nil)
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil taskV2: %v", err)
	}
	svc = newResumeTestService(&stubTaskV2RepoForResume{}, nil)
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil stepReader: %v", err)
	}
}

func TestResumeInterruptedTask_TaskNotFound(t *testing.T) {
	repo := &stubTaskV2RepoForResume{getErr: apierror.NotFound("TASK_V2", "task not found")}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{})
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-x"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestResumeInterruptedTask_WrongSession(t *testing.T) {
	repo := &stubTaskV2RepoForResume{
		task: biz.Task{ID: "t-1", SessionID: "s-other", Status: biz.TaskStatusInterrupted},
	}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{})
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("want BadRequest, got %v", err)
	}
	if repo.resumeN != 0 {
		t.Fatalf("CAS must not run on session mismatch, got %d calls", repo.resumeN)
	}
}

func TestResumeInterruptedTask_NotInterruptedPreCheck(t *testing.T) {
	repo := &stubTaskV2RepoForResume{
		task: biz.Task{ID: "t-1", SessionID: "s-1", Status: biz.TaskStatusCompleted},
	}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{})
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("want Conflict, got %v", err)
	}
	if repo.resumeN != 0 {
		t.Fatalf("CAS must be skipped when pre-check fails, got %d calls", repo.resumeN)
	}
}

func TestResumeInterruptedTask_CASConflict(t *testing.T) {
	// Pre-check passes (interrupted) but CAS loses the race (concurrent click).
	repo := &stubTaskV2RepoForResume{
		task:     biz.Task{ID: "t-1", SessionID: "s-1", Status: biz.TaskStatusInterrupted},
		resumeOK: false,
	}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{})
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("want Conflict, got %v", err)
	}
	if repo.resumeN != 1 {
		t.Fatalf("CAS must be attempted once, got %d", repo.resumeN)
	}
}

func TestResumeInterruptedTask_CASError(t *testing.T) {
	repo := &stubTaskV2RepoForResume{
		task:      biz.Task{ID: "t-1", SessionID: "s-1", Status: biz.TaskStatusInterrupted},
		resumeErr: errors.New("db down"),
	}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{})
	if err := svc.ResumeInterruptedTask(context.Background(), "s-1", "t-1"); err == nil {
		t.Fatal("want error propagation")
	}
}

// TestPrepareInterruptedResume_Happy verifies the full prepare path: CAS
// transitions to running, the resume content embeds the execution trace and
// the original user message.
func TestPrepareInterruptedResume_Happy(t *testing.T) {
	started := time.Now().UTC().Add(-time.Minute)
	repo := &stubTaskV2RepoForResume{
		task: biz.Task{
			ID: "t-1", SessionID: "s-1", UserMessage: "写一份报告",
			Status: biz.TaskStatusInterrupted,
		},
		resumeOK: true,
		resumeTask: biz.Task{
			ID: "t-1", SessionID: "s-1", UserMessage: "写一份报告",
			Status: biz.TaskStatusRunning, Version: 4,
		},
	}
	steps := &stubStepV2ReaderForResume{steps: []biz.Step{
		{ID: "st-1", Kind: biz.StepKindAction, Status: biz.StepStatusCompleted, ToolName: "web_search", StartedAt: started},
		{ID: "st-2", Kind: biz.StepKindThinking, Status: biz.StepStatusCompleted, Content: "noise", StartedAt: started},
		{ID: "st-3", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted, Content: "已找到 3 条资料", StartedAt: started.Add(time.Second)},
	}}
	svc := newResumeTestService(repo, steps)

	task, content, err := svc.prepareInterruptedResume(context.Background(), "s-1", "t-1")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if task.Status != biz.TaskStatusRunning {
		t.Errorf("task status=%s, want running", task.Status)
	}
	if repo.resumeN != 1 {
		t.Errorf("CAS calls=%d, want 1", repo.resumeN)
	}
	if want := "写一份报告"; !contains(content, want) {
		t.Errorf("content missing original message %q:\n%s", want, content)
	}
	if !contains(content, "web_search") {
		t.Errorf("content missing action trace:\n%s", content)
	}
	if !contains(content, "已找到 3 条资料") {
		t.Errorf("content missing reply trace:\n%s", content)
	}
	if contains(content, "noise") {
		t.Errorf("thinking steps must be excluded from trace:\n%s", content)
	}
}

// TestPrepareInterruptedResume_StepReadDegrades verifies that a step read
// failure does not block the resume — the trace degrades to empty.
func TestPrepareInterruptedResume_StepReadDegrades(t *testing.T) {
	repo := &stubTaskV2RepoForResume{
		task:       biz.Task{ID: "t-1", SessionID: "s-1", UserMessage: "hi", Status: biz.TaskStatusInterrupted},
		resumeOK:   true,
		resumeTask: biz.Task{ID: "t-1", SessionID: "s-1", Status: biz.TaskStatusRunning},
	}
	svc := newResumeTestService(repo, &stubStepV2ReaderForResume{err: errors.New("read fail")})
	_, content, err := svc.prepareInterruptedResume(context.Background(), "s-1", "t-1")
	if err != nil {
		t.Fatalf("prepare must not fail on step read error: %v", err)
	}
	if !contains(content, "hi") {
		t.Errorf("content missing original message:\n%s", content)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
