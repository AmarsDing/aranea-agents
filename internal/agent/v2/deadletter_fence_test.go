package v2

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- P1 FR-10：complete_task_terminal 死信重放终态 fencing ---
//
// 外来脏文件 deadletter_replay_test.go 保持不动；此文件嵌入其同包 fakes
// （fakeDeadLetterStore / fakeRepoSet / fakeBus）并叠加窄口与围栏假件。

// fr10Store 在 fakeDeadLetterStore 上叠加 biz.EventDeadLetterDeferMarker 窄
// 口实现，并同步 pending 行 LastError（与真实 repo 行为一致）。
type fr10Store struct {
	*fakeDeadLetterStore
	deferLog map[int64]string
}

func newFR10Store() *fr10Store {
	return &fr10Store{fakeDeadLetterStore: newFakeDeadLetterStore(), deferLog: map[int64]string{}}
}

func (f *fr10Store) DeferEventDeadLetter(_ context.Context, id int64, marker string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deferLog[id] = marker
	for i := range f.pending {
		if f.pending[i].ID == id {
			f.pending[i].LastError = marker
		}
	}
	return nil
}

// fr10Fence 实现 DeadLetterTerminalFence：按 taskID 喂状态、按 sessionID 喂
// 活跃 run 判读。
type fr10Fence struct {
	statuses map[string]biz.TaskStatus
	found    map[string]bool
	hasRun   map[string]bool
	readErr  error
}

func (f *fr10Fence) TaskStatusByID(_ context.Context, taskID string) (biz.TaskStatus, bool, error) {
	if f.readErr != nil {
		return "", false, f.readErr
	}
	s := f.statuses[taskID]
	return s, f.found[taskID], nil
}

func (f *fr10Fence) HasActiveRun(sessionID string) bool { return f.hasRun[sessionID] }

func fr10TerminalRec(id int64, taskID, sessionID string, requested biz.TaskStatus) biz.EventDeadLetter {
	payload, _ := json.Marshal(biz.Task{ID: taskID, SessionID: sessionID, Status: requested})
	return biz.EventDeadLetter{
		ID:          id,
		EventKind:   string(biz.EventKindTaskCompleted),
		EntityKind:  EntityKindTask,
		EntityOp:    PersistOpCompleteTaskTerminal,
		EntityID:    taskID,
		SessionID:   sessionID,
		PayloadJSON: string(payload),
		State:       biz.EventDeadLetterStatePending,
	}
}

func newFR10Sequencer(t *testing.T, rs RepoSet, store biz.EventDeadLetterRepo, fence DeadLetterTerminalFence) *Sequencer {
	t.Helper()
	opts := []Option{
		WithPublishBuffer(16), WithPersistBuffer(16),
		WithDeadLetterStore(store),
		// 后台 replay worker 与手动 replayDeadLettersOnce 竞争（同既有测试）。
		WithDeadLetterReplayLoopDisabled(),
	}
	if fence != nil {
		opts = append(opts, WithDeadLetterTerminalFence(fence))
	}
	s := NewSequencer(rs, &fakeBus{}, loggateway.NewNoop(), opts...)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// 1. 当前=其他终态（failed）× 请求终态 completed → abandoned(terminal_diverged)，
// 不落库、不标记 replayed——重放不得覆盖 live 真实结局。
func TestSequencer_DLFence_TerminalDiverged_Abandoned(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusFailed},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 0 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 0 (diverged must not overwrite)", len(rs.terminal))
	}
	if got := store.abandoned[1]; got == "" {
		t.Fatalf("P1 FR-10: expected abandoned, got %+v", store)
	} else if want := "terminal_diverged"; got[:len(want)] != want {
		t.Fatalf("P1 FR-10: abandon reason = %q, want prefix %q", got, want)
	}
	if len(store.replayed) != 0 {
		t.Fatalf("P1 FR-10: replayed = %v, want empty", store.replayed)
	}
}

// 2. 当前终态==请求终态 → 幂等结清（mark replayed），不落库。
func TestSequencer_DLFence_SameTerminal_MarkedReplayed(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusCompleted},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 0 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 0 (already terminal)", len(rs.terminal))
	}
	if len(store.replayed) != 1 || store.replayed[0] != 1 {
		t.Fatalf("P1 FR-10: replayed = %v, want [1]", store.replayed)
	}
	if len(store.abandoned) != 0 {
		t.Fatalf("P1 FR-10: abandoned = %v, want empty", store.abandoned)
	}
}

// 3. 当前=running 且会话有活跃 run → defer：attempts 不消耗、defer 标记
// 累计、不落库——live 终态会自己落账。
func TestSequencer_DLFence_RunningWithActiveRun_Deferred(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusRunning},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{"s-1": true},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 0 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 0 (active run must not be clobbered)", len(rs.terminal))
	}
	if n := store.attempts[1]; n != 0 {
		t.Fatalf("P1 FR-10: attempts = %d, want 0 (defer never burns attempt budget)", n)
	}
	if got := store.deferLog[1]; got != "deferred_active_run:1" {
		t.Fatalf("P1 FR-10: defer marker = %q, want deferred_active_run:1", got)
	}
	if len(store.replayed) != 0 || len(store.abandoned) != 0 {
		t.Fatalf("P1 FR-10: replayed=%v abandoned=%v, want both empty", store.replayed, store.abandoned)
	}
}

// 4. 当前=running 且无活跃 run（僵尸 running）→ 允许终态化（宕后合法收账）。
func TestSequencer_DLFence_RunningNoActiveRun_Applied(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusRunning},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 1 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 1 (zombie running may be terminalized)", len(rs.terminal))
	}
	if len(store.replayed) != 1 {
		t.Fatalf("P1 FR-10: replayed = %v, want [1]", store.replayed)
	}
}

// 5. 当前=interrupted → 允许终态化（宕后 legal 收账路径）。
func TestSequencer_DLFence_Interrupted_Applied(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusInterrupted},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 1 || len(store.replayed) != 1 {
		t.Fatalf("P1 FR-10: terminal=%d replayed=%v, want 1/[1] (interrupted legal terminalization)", len(rs.terminal), store.replayed)
	}
}

// 6. defer 计满（last_error=deferred_active_run:12）仍遇 running+活跃 run →
// abandoned(deferred_active_run_exceeded)——"活跃 run"超 1h 判定泄漏，真死
// 信不得永久滞留（FR-10 风险行 ≤12 次≈1h）。
func TestSequencer_DLFence_DeferCapExceeded_Abandoned(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	rec := fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)
	rec.LastError = "deferred_active_run:12"
	store.pending = []biz.EventDeadLetter{rec}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusRunning},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{"s-1": true},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if got := store.abandoned[1]; got != "deferred_active_run_exceeded" {
		t.Fatalf("P1 FR-10: abandon reason = %q, want deferred_active_run_exceeded", got)
	}
	if _, ok := store.deferLog[1]; ok {
		t.Fatalf("P1 FR-10: defer marker written past cap: %q", store.deferLog[1])
	}
	if len(rs.terminal) != 0 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 0", len(rs.terminal))
	}
}

// 7. 窄口缺席（fakeDeadLetterStore 未实现 DeferEventDeadLetter）→ defer 静默
// 退化：无 panic、不消耗 attempts、不写标记（类型断言静默跳过，FR-9 窄口同
// 构策略）。
func TestSequencer_DLFence_NarrowMarkerAbsent_SilentlySkipped(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFakeDeadLetterStore()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	fence := &fr10Fence{
		statuses: map[string]biz.TaskStatus{"t-1": biz.TaskStatusRunning},
		found:    map[string]bool{"t-1": true},
		hasRun:   map[string]bool{"s-1": true},
	}
	s := newFR10Sequencer(t, rs, store, fence)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if n := store.attempts[1]; n != 0 {
		t.Fatalf("P1 FR-10: attempts = %d, want 0 (defer without marker port still skips attempts)", n)
	}
	if got := store.pending[0].LastError; got != "" {
		t.Fatalf("P1 FR-10: last_error = %q, want empty (no marker port → no persisted count)", got)
	}
	if len(rs.terminal) != 0 {
		t.Fatalf("P1 FR-10: terminal apply count = %d, want 0", len(rs.terminal))
	}
}

// 8. fence=nil → 旧语义：盲放（proceed），行缺失与否都不查活态。
func TestSequencer_DLFence_NilFence_LegacyBlindApply(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	store := newFR10Store()
	store.pending = []biz.EventDeadLetter{fr10TerminalRec(1, "t-1", "s-1", biz.TaskStatusCompleted)}
	s := newFR10Sequencer(t, rs, store, nil)
	s.persist.replayDeadLettersOnce()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.terminal) != 1 || len(store.replayed) != 1 {
		t.Fatalf("P1 FR-10: terminal=%d replayed=%v, want 1/[1] (legacy blind apply)", len(rs.terminal), store.replayed)
	}
}

// 9. TaskRunTerminalFence 适配器：GetTask→status、NotFound→found=false、其
// 他错误→err 原样上抛。
func TestTaskRunTerminalFence_Adapter(t *testing.T) {
	t.Parallel()
	reader := &fr10TaskReader{tasks: map[string]biz.Task{"t-1": {ID: "t-1", Status: biz.TaskStatusRunning, Version: 1}}}
	fence := &TaskRunTerminalFence{Tasks: reader, HasActive: func(string) bool { return true }}

	st, found, err := fence.TaskStatusByID(context.Background(), "t-1")
	if err != nil || !found || st != biz.TaskStatusRunning {
		t.Fatalf("existing task → (%q,%v,%v), want (running,true,nil)", st, found, err)
	}
	if _, found, err := fence.TaskStatusByID(context.Background(), "missing"); err != nil || found {
		t.Fatalf("missing task → (found=%v,err=%v), want (false,nil)", found, err)
	}
	wantErr := apierror.Internal("TASK_V2", "boom")
	reader.err = wantErr
	if _, _, err := fence.TaskStatusByID(context.Background(), "t-1"); err != wantErr {
		t.Fatalf("read error → err=%v, want passthrough %v", err, wantErr)
	}
	if !fence.HasActiveRun("s-1") {
		t.Fatal("HasActiveRun should delegate")
	}
}

// fr10TaskReader 实现 biz.TaskV2Reader 的最小桩（GetTask 可喂行/错误）。
type fr10TaskReader struct {
	biz.TaskV2Repo
	mu    sync.Mutex
	tasks map[string]biz.Task
	err   error
}

func (r *fr10TaskReader) GetTask(_ context.Context, id string) (biz.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return biz.Task{}, r.err
	}
	if t, ok := r.tasks[id]; ok {
		return t, nil
	}
	return biz.Task{}, apierror.NotFound("TASK_V2", "not found")
}
