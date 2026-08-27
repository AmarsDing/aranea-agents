package biz

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/session"
)

type stubStepReader struct {
	steps []Step
}

func (s stubStepReader) GetStep(_ context.Context, id string) (Step, error) {
	for _, st := range s.steps {
		if st.ID == id {
			return st, nil
		}
	}
	return Step{}, nil
}

func (s stubStepReader) ListStepsBySession(context.Context, string) ([]Step, error) {
	return s.steps, nil
}

func (s stubStepReader) ListStepsByTurn(context.Context, string) ([]Step, error) {
	return s.steps, nil
}

func (s stubStepReader) ListStepsByTask(context.Context, string) ([]Step, error) {
	return s.steps, nil
}

func (s stubStepReader) ListStepsBySessionPaged(_ context.Context, _ string, _ StepListOptions) ([]Step, bool, error) {
	return s.steps, false, nil
}

func (s stubStepReader) ListStepsBySessionID(context.Context, string) ([]Step, error) {
	return s.steps, nil
}

func (s stubStepReader) ListStepsBySpiritSession(context.Context, string) ([]Step, error) {
	return s.steps, nil
}

func (s stubStepReader) MaxSeqBySpiritSession(context.Context, string) (int64, error) {
	return 0, nil
}

type stubTaskReader struct {
	tasks []Task
	err   error
}

func (s stubTaskReader) GetTask(_ context.Context, id string) (Task, error) {
	for _, t := range s.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return Task{}, nil
}

func (s stubTaskReader) ListTasksBySession(context.Context, string) ([]Task, error) {
	return s.tasks, s.err
}

// Regression: steps_v2 persists only agent-side steps; user inputs live in
// tasks_v2. ListBySession must merge both or every role=user message is
// dropped from the reconstructed timeline (breaks auto-memory L4 extraction,
// L3 consolidation quality, and the frontend chat history).
func TestSessionActivityLister_ListBySession_MergesTasks(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	lister := NewSessionActivityLister(
		stubStepReader{steps: []Step{
			{ID: "st1", Kind: StepKindReply, Status: "completed", SessionID: "s1", StartedAt: now.Add(time.Second), Content: "你好，张三！"},
		}},
		stubTaskReader{tasks: []Task{
			{ID: "tk1", SessionID: "s1", UserMessage: "我叫张三", Status: TaskStatusCompleted, CreatedAt: now},
		}},
	)
	if lister == nil {
		t.Fatal("lister is nil")
	}
	entries, err := lister.ListBySession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (1 step + 1 task)", len(entries))
	}
	var userEntry *session.ActivityEntry
	for i := range entries {
		if entries[i].Kind == "task" {
			userEntry = &entries[i]
		}
	}
	if userEntry == nil {
		t.Fatal("no kind=task entry merged from tasks_v2")
	}
	if userEntry.Content != "我叫张三" {
		t.Fatalf("user content = %q, want %q", userEntry.Content, "我叫张三")
	}
	if userEntry.ID != "tk1" {
		t.Fatalf("user entry id = %q, want tk1", userEntry.ID)
	}
}

// Nil taskReader keeps the legacy steps-only behavior (tests/CLI fallback).
func TestSessionActivityLister_ListBySession_NilTaskReader(t *testing.T) {
	lister := NewSessionActivityLister(stubStepReader{steps: []Step{
		{ID: "st1", Kind: StepKindReply, Status: "completed", SessionID: "s1"},
	}}, nil)
	entries, err := lister.ListBySession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (steps only)", len(entries))
	}
}

// Nil stepReader returns nil lister so callers fall back to the legacy path.
func TestNewSessionActivityLister_NilStepReader(t *testing.T) {
	if got := NewSessionActivityLister(nil, stubTaskReader{}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// F2: Step.NoticeType must survive the Step→Activity→ActivityEntry chain so
// downstream consumers (message view filter, frontend) can distinguish
// system-internal notices from user-facing ones.
func TestActivitiesToSessionEntries_PropagatesNoticeType(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acts := []Activity{
		{
			ID:        "n1",
			Kind:      ActivityKindNotice,
			Status:    ActivityStatusCompleted,
			SessionID: "s1",
			Timestamp: now,
			Seq:       1,
			Content:   `{"hits":[]}`,
			Meta:      map[string]any{"notice_type": "memory_recalled"},
		},
		{
			ID:        "n2",
			Kind:      ActivityKindNotice,
			Status:    ActivityStatusCompleted,
			SessionID: "s1",
			Timestamp: now,
			Seq:       2,
			Content:   "model switched",
			// no notice_type meta → empty
		},
	}
	entries := activitiesToSessionEntries(acts)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].NoticeType != "memory_recalled" {
		t.Fatalf("entries[0].NoticeType = %q, want memory_recalled", entries[0].NoticeType)
	}
	if entries[1].NoticeType != "" {
		t.Fatalf("entries[1].NoticeType = %q, want empty", entries[1].NoticeType)
	}
}

// The full chain: merged task entries must surface as role=user ChatMessages
// through ActivityMessageReader (the ListSessionMessages RPC path).
func TestActivityMessageReader_UserMessagesFromTasks(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	lister := NewSessionActivityLister(
		stubStepReader{steps: []Step{
			{ID: "st1", Kind: StepKindReply, Status: "completed", SessionID: "s1", StartedAt: now.Add(time.Second), Content: "reply"},
		}},
		stubTaskReader{tasks: []Task{
			{ID: "tk1", SessionID: "s1", UserMessage: "user text", Status: TaskStatusCompleted, CreatedAt: now},
		}},
	)
	reader := session.NewActivityMessageReader(lister)
	msgs, err := reader.ListMessagesRecent(context.Background(), "s1", 10)
	if err != nil {
		t.Fatalf("ListMessagesRecent: %v", err)
	}
	var userMsgs int
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgs++
			if m.ContentMarkdown != "user text" {
				t.Fatalf("user content = %q, want %q", m.ContentMarkdown, "user text")
			}
		}
	}
	if userMsgs != 1 {
		t.Fatalf("role=user messages = %d, want 1", userMsgs)
	}
}

// 79-runtime-governance fix2：user(task) 行必须回填首 turn id（空 TurnID 会被
// synthesizeTurnNumbers 编为 0，压缩 body 窗口恒排除用户消息）。
func TestFirstTurnIDByTask(t *testing.T) {
	steps := []Step{
		// tk1 两个 turn：最小 seq 的 turn-1a 胜出。
		{ID: "s1", TaskID: "tk1", TurnID: "turn-1a", Seq: 1},
		{ID: "s2", TaskID: "tk1", TurnID: "turn-1b", Seq: 2},
		// tk2 仅续跑 turn 有 step（首 turn 崩溃未产出 step）：归属续跑 turn。
		{ID: "s3", TaskID: "tk2", TurnID: "turn-2b", Seq: 3},
		// 缺 TaskID/TurnID 的 step 不参与映射。
		{ID: "s4", TurnID: "turn-x", Seq: 4},
		{ID: "s5", TaskID: "tk9", Seq: 5},
	}
	got := firstTurnIDByTask(steps)
	want := map[string]string{"tk1": "turn-1a", "tk2": "turn-2b"}
	if len(got) != len(want) {
		t.Fatalf("firstTurnIDByTask = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("firstTurn[%s] = %q, want %q", k, got[k], v)
		}
	}
}

// fix2 接线验证：ListBySession 把回填结果写到 user 行；无 step 的 task 保持空。
func TestSessionActivityLister_ListBySession_BackfillsTaskTurnID(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	lister := NewSessionActivityLister(
		stubStepReader{steps: []Step{
			{ID: "st1", Kind: StepKindReply, Status: "completed", SessionID: "s1",
				TaskID: "tk1", TurnID: "turn-1", Seq: 1, StartedAt: now.Add(time.Second)},
		}},
		stubTaskReader{tasks: []Task{
			{ID: "tk1", SessionID: "s1", UserMessage: "有 step", Status: TaskStatusCompleted, CreatedAt: now},
			{ID: "tk2", SessionID: "s1", UserMessage: "无 step 残留", Status: TaskStatusInterrupted, CreatedAt: now.Add(2 * time.Second)},
		}},
	)
	entries, err := lister.ListBySession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	turnOf := map[string]string{}
	for _, e := range entries {
		if e.Kind == string(ActivityKindTask) {
			turnOf[e.ID] = e.TurnID
		}
	}
	if turnOf["tk1"] != "turn-1" {
		t.Fatalf("tk1 TurnID = %q, want turn-1", turnOf["tk1"])
	}
	if turnOf["tk2"] != "" {
		t.Fatalf("tk2 TurnID = %q, want empty (no steps)", turnOf["tk2"])
	}
}
