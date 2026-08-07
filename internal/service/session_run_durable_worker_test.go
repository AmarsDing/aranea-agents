package service

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// durableWorkerRepoStub 是 SessionRun 仓储的内存实现。ResumeDurableSessionRun
// 的 goroutine 会异步读写本 stub，因此所有方法必须持锁（本文件的 panic 路径测试
// 依赖并发安全）。
type durableWorkerRepoStub struct {
	mu   sync.Mutex
	runs map[string]biz.SessionRun
}

func (s *durableWorkerRepoStub) get(id string) biz.SessionRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id]
}

func (s *durableWorkerRepoStub) Create(_ context.Context, run biz.SessionRun) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = map[string]biz.SessionRun{}
	}
	s.runs[run.ID] = run
	return run.ID, nil
}
func (s *durableWorkerRepoStub) UpdatePhase(_ context.Context, id, phase string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[id]
	run.Phase = biz.NormalizeSessionRunPhase(phase)
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) UpdateCheckpointID(_ context.Context, id, checkpointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[id]
	run.CheckpointID = checkpointID
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) MarkTerminal(_ context.Context, id, phase, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[id]
	run.Phase = biz.NormalizeSessionRunPhase(phase)
	run.ErrorMessage = errMsg
	run.ResumeStartedAt = ""
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id], nil
}
func (s *durableWorkerRepoStub) GetActiveForSession(_ context.Context, _ string) (biz.SessionRun, error) {
	return biz.SessionRun{}, nil
}
func (s *durableWorkerRepoStub) ListByPhase(_ context.Context, phase string, _ int) ([]biz.SessionRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []biz.SessionRun
	for _, run := range s.runs {
		if run.Phase == biz.NormalizeSessionRunPhase(phase) && run.FinishedAt == "" {
			out = append(out, run)
		}
	}
	return out, nil
}
func (s *durableWorkerRepoStub) ListForJobs(_ context.Context, _ biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *durableWorkerRepoStub) ListBySession(_ context.Context, _ string, _, _ int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *durableWorkerRepoStub) TryClaimDurableResume(_ context.Context, id, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || run.Phase != biz.SessionRunPhaseDurable || run.CheckpointID == "" {
		return false, nil
	}
	if run.ResumeStartedAt != "" {
		return false, nil
	}
	run.ResumeStartedAt = "claimed"
	s.runs[id] = run
	return true, nil
}
func (s *durableWorkerRepoStub) ClearResumeClaim(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[id]
	run.ResumeStartedAt = ""
	s.runs[id] = run
	return nil
}

func (s *durableWorkerRepoStub) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, nil
}
func (s *durableWorkerRepoStub) TransitionPhase(_ context.Context, id, fromPhase, toPhase string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || string(run.Phase) != fromPhase {
		return false, nil
	}
	run.Phase = biz.NormalizeSessionRunPhase(toPhase)
	s.runs[id] = run
	return true, nil
}
func (s *durableWorkerRepoStub) MarkTerminalWherePhase(_ context.Context, id, fromPhase, toPhase, errMsg string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok || string(run.Phase) != fromPhase {
		return false, nil
	}
	run.Phase = biz.NormalizeSessionRunPhase(toPhase)
	run.ErrorMessage = errMsg
	s.runs[id] = run
	return true, nil
}

type durableWorkerCheckpointStub struct {
	cps map[string]biz.SessionRunCheckpoint
}

func (s *durableWorkerCheckpointStub) Create(_ context.Context, cp biz.SessionRunCheckpoint) (string, error) {
	if s.cps == nil {
		s.cps = map[string]biz.SessionRunCheckpoint{}
	}
	s.cps[cp.ID] = cp
	return cp.ID, nil
}
func (s *durableWorkerCheckpointStub) Get(_ context.Context, id string) (biz.SessionRunCheckpoint, error) {
	return s.cps[id], nil
}
func (s *durableWorkerCheckpointStub) GetBySessionRunID(_ context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	for _, cp := range s.cps {
		if cp.SessionRunID == sessionRunID {
			return cp, nil
		}
	}
	return biz.SessionRunCheckpoint{}, nil
}

// TestResumeDurableSessionRun_panicMarksRunFailed 复现基线 panic：ChatService.lg 为 nil
// 时，goroutine 内 ExecuteTurn 触发 nil-interface panic。recover 必须将 run 落为 failed
// 并清除 resume claim（防卡死），再 re-panic 交由 safego 记录堆栈并触发 PanicHook。
func TestResumeDurableSessionRun_panicMarksRunFailed(t *testing.T) {
	repo := &durableWorkerRepoStub{runs: map[string]biz.SessionRun{
		"run-1": {
			ID:           "run-1",
			SessionID:    "sess-1",
			Phase:        biz.SessionRunPhaseDurable,
			CheckpointID: "cp-1",
		},
	}}
	cps := &durableWorkerCheckpointStub{
		cps: map[string]biz.SessionRunCheckpoint{
			"cp-1": {ID: "cp-1", SessionRunID: "run-1", PayloadJSON: `{"session_id":"sess-1","turn_id":"t1","agent_id":"a1"}`},
		},
	}
	uc := biz.NewSessionRunUsecase(repo, cps, loggateway.NewNoop())
	// lg 故意留 nil：goroutine 内 s.lg.With(...) 触发 panic（基线回归场景）。
	chat := &ChatService{orch: &ChatOrchestrator{channelDeps: ChatChannelDeps{ChJobs: ChannelTurnJobDeps{SessionRuns: uc}}}}

	var hooked atomic.Int32
	safego.RegisterPanicHook(func(string, interface{}, []byte) { hooked.Add(1) })
	defer safego.RegisterPanicHook(nil)

	if err := chat.ResumeDurableSessionRun(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	// 轮询等待后台 goroutine：recover → Fail → re-panic → safego hook。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run := repo.get("run-1")
		if run.Phase == biz.SessionRunPhaseFailed && hooked.Load() > 0 {
			if run.ResumeStartedAt != "" {
				t.Fatalf("resume claim not cleared: %q", run.ResumeStartedAt)
			}
			if !strings.Contains(run.ErrorMessage, "durable resume panic") {
				t.Fatalf("error message %q should contain panic detail", run.ErrorMessage)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run := repo.get("run-1")
	t.Fatalf("run not marked failed after goroutine panic: phase=%q claimed=%q hooked=%d",
		run.Phase, run.ResumeStartedAt, hooked.Load())
}

// durableRunCtrlStub 实现 biz.TurnRunControlGateway，会话永远无活跃运行。
type durableRunCtrlStub struct{}

func (durableRunCtrlStub) HasActiveRun(string) bool               { return false }
func (durableRunCtrlStub) CancelRun(context.Context, string) bool { return false }
func (durableRunCtrlStub) SetRunStatus(context.Context, string, string, string, string) {
}
func (durableRunCtrlStub) LastPendingMessageID(string) string { return "" }

// claimOnlyResumerStub 实现 biz.DurableResumeGateway：仅执行 claim 段，不启动真实
// turn goroutine。worker 去重语义不依赖 LLM 执行；真实 resume 链路（含 goroutine
// panic 防卡死）由 TestResumeDurableSessionRun_panicMarksRunFailed 覆盖。
type claimOnlyResumerStub struct{ runs *biz.SessionRunUsecase }

func (s claimOnlyResumerStub) ResumeDurableSessionRun(ctx context.Context, sessionRunID string) error {
	_, err := s.runs.TryClaimDurableResume(ctx, sessionRunID)
	return err
}

func TestSessionRunDurableWorker_skipsUnclaimedDuplicate(t *testing.T) {
	repo := &durableWorkerRepoStub{runs: map[string]biz.SessionRun{
		"run-1": {
			ID:           "run-1",
			SessionID:    "sess-1",
			Phase:        biz.SessionRunPhaseDurable,
			CheckpointID: "cp-1",
		},
	}}
	cps := &durableWorkerCheckpointStub{
		cps: map[string]biz.SessionRunCheckpoint{
			"cp-1": {ID: "cp-1", SessionRunID: "run-1", PayloadJSON: `{"session_id":"sess-1","turn_id":"t1","agent_id":"a1"}`},
		},
	}
	uc := biz.NewSessionRunUsecase(repo, cps, loggateway.NewNoop())
	w := NewSessionRunDurableWorker(uc, durableRunCtrlStub{}, claimOnlyResumerStub{runs: uc}, loggateway.NewNoop())
	w.processOnce(context.Background())
	if repo.runs["run-1"].ResumeStartedAt == "" {
		t.Fatal("expected claim on first process")
	}
	before := repo.runs["run-1"].ResumeStartedAt
	w.processOnce(context.Background())
	if repo.runs["run-1"].ResumeStartedAt != before {
		t.Fatal("second processOnce should not re-claim")
	}
}
