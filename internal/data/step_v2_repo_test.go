package data

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestStepV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.CreateStep(ctx, biz.Step{
		ID: "st-1", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1,
		Content: "thinking...", Status: biz.StepStatusRunning,
		StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	got, err := repo.GetStep(ctx, "st-1")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if got.Kind != biz.StepKindThinking || got.Content != "thinking..." {
		t.Fatalf("step mismatch: %+v", got)
	}
}

func TestStepV2Repo_Upsert_JSONArgs(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	args, _ := json.Marshal(map[string]any{"command": "ls -la"})
	_, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-2", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindAction, Seq: 2,
		ToolName: "shell", ToolArgs: args,
		Status: biz.StepStatusToolRunning, StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	got, _ := repo.GetStep(ctx, "st-2")
	if got.ToolName != "shell" {
		t.Fatalf("tool name: expected shell, got %s", got.ToolName)
	}
	var argsGot map[string]any
	if err := json.Unmarshal(got.ToolArgs, &argsGot); err != nil {
		t.Fatalf("unmarshal tool args: %v", err)
	}
	if argsGot["command"] != "ls -la" {
		t.Fatalf("tool args mismatch: %v", argsGot)
	}
}

// TestStepV2Repo_Upsert_VersionGuard verifies production CAS: stale Version
// (incoming < stored) is a no-op returning the stored row (same as Task/Turn).
func TestStepV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "v1",
		Status: biz.StepStatusRunning, StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertStep v1: %v", err)
	}
	stale, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "stale",
		Status: biz.StepStatusPending, StartedAt: now, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertStep stale: %v", err)
	}
	if stale.Content != "v1" || stale.Status != biz.StepStatusRunning {
		t.Fatalf("stale version overwrote: %+v", stale)
	}
	_, err = repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "v2",
		Status: biz.StepStatusCompleted, StartedAt: now, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertStep v2: %v", err)
	}
	got, _ := repo.GetStep(ctx, "st-cas")
	if got.Content != "v2" || got.Status != biz.StepStatusCompleted || got.Version != 2 {
		t.Fatalf("newer version not applied: %+v", got)
	}
}

func TestStepV2Repo_ListByTurn_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateStep(ctx, biz.Step{
			ID: "ord-" + string(rune('a'+i)), TurnID: "turn-x", TaskID: "t-1",
			SessionID: "s-1", SpiritSessionID: "s-1",
			Kind: biz.StepKindThinking, Seq: seq,
			Status: biz.StepStatusCompleted, StartedAt: now, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateStep[%d]: %v", i, err)
		}
	}
	steps, err := repo.ListStepsByTurn(ctx, "turn-x")
	if err != nil {
		t.Fatalf("ListStepsByTurn: %v", err)
	}
	if len(steps) != 3 || steps[0].Seq != 1 || steps[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{steps[0].Seq, steps[1].Seq, steps[2].Seq})
	}
}

func TestStepV2Repo_GetStep_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetStep(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent step, got nil")
	}
}

// ListStepsBySessionID filters by the exact session_id column: steps of a
// sibling session under the same spirit session must not leak in, and the
// team session's own steps must not appear in the spirit session's result.
func TestStepV2Repo_ListStepsBySessionID_ExactSession(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seed := []biz.Step{
		{ID: "st-spirit", TurnID: "turn-1", TaskID: "t-1", SessionID: "spirit-1", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 1, Status: biz.StepStatusCompleted, StartedAt: now, Version: 1},
		{ID: "st-team", TurnID: "turn-2", TaskID: "t-1", SessionID: "team-1", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 2, Status: biz.StepStatusCompleted, StartedAt: now.Add(time.Second), Version: 1},
		{ID: "st-member", TurnID: "turn-3", TaskID: "t-1", SessionID: "member-1", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 3, Status: biz.StepStatusCompleted, StartedAt: now.Add(2 * time.Second), Version: 1},
	}
	for i, s := range seed {
		if _, err := repo.CreateStep(ctx, s); err != nil {
			t.Fatalf("CreateStep[%d]: %v", i, err)
		}
	}

	teamSteps, err := repo.ListStepsBySessionID(ctx, "team-1")
	if err != nil {
		t.Fatalf("ListStepsBySessionID: %v", err)
	}
	if len(teamSteps) != 1 || teamSteps[0].ID != "st-team" {
		t.Fatalf("exact semantics: expected [st-team], got %+v", teamSteps)
	}

	// Exact semantics applies to the spirit session too: only its own step.
	spiritSteps, err := repo.ListStepsBySession(ctx, "spirit-1")
	if err != nil {
		t.Fatalf("ListStepsBySession: %v", err)
	}
	if len(spiritSteps) != 1 || spiritSteps[0].ID != "st-spirit" {
		t.Fatalf("exact semantics: expected [st-spirit], got %+v", spiritSteps)
	}

	// Tree semantics lives in ListStepsBySpiritSession: the spirit root sees
	// the whole tree (spirit + team + member steps).
	treeSteps, err := repo.ListStepsBySpiritSession(ctx, "spirit-1")
	if err != nil {
		t.Fatalf("ListStepsBySpiritSession: %v", err)
	}
	if len(treeSteps) != 3 {
		t.Fatalf("tree semantics: expected 3 steps, got %d", len(treeSteps))
	}
}

// seedPagedSteps inserts n steps (seq 1..n) into sessionID via the repo.
func seedPagedSteps(t *testing.T, repo biz.StepV2Repo, sessionID string, n int) {
	t.Helper()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		_, err := repo.CreateStep(context.Background(), biz.Step{
			ID:              fmt.Sprintf("step-%s-%d", sessionID, i),
			SessionID:       sessionID,
			SpiritSessionID: sessionID,
			Kind:            biz.StepKindReply,
			AuthorAgentKey:  "agent-1",
			Seq:             int64(i),
			Status:          biz.StepStatusCompleted,
			StartedAt:       base.Add(time.Duration(i) * time.Second),
			Version:         1,
		})
		if err != nil {
			t.Fatalf("seed step %d: %v", i, err)
		}
	}
}

func stepSeqs(steps []biz.Step) []int64 {
	out := make([]int64, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Seq)
	}
	return out
}

func TestStepV2Repo_ListStepsBySessionPaged(t *testing.T) {
	d := newTestDataPG(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	seedPagedSteps(t, repo, "sess-paged", 10)
	ctx := context.Background()

	t.Run("limit window returns latest N asc with hasMore", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		got := stepSeqs(steps)
		want := []int64{8, 9, 10}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("seqs=%v, want %v (ascending)", got, want)
		}
	})

	t.Run("before_seq walks towards older", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 8})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[5 6 7]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("last page hasMore=false", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[1 2]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("limit=0 degrades to full list", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if len(steps) != 10 {
			t.Errorf("len=%d, want 10", len(steps))
		}
	})
}

// TestStepV2Repo_LatestStepActivityAt 锚定 F1（2026-09-03 lbg-verify-planner
// 复盘）idle 团队超时的活动探测 + P4-2 信号升级：团队主会话 + 成员子会话
// 集合的 MAX(updated_at) 单查询聚合（updated_at 由 ent UpdateDefault 每次
// 写入刷新，语义 ≡ GREATEST(started_at, updated_at)）；存量行经迁移回填
// updated_at=started_at 后保持原始新鲜度；原地更新刷新 updated_at 使
// 探测信号提升；无 step 返回零值；空会话列表快速返回。
func TestStepV2Repo_LatestStepActivityAt(t *testing.T) {
	client, rawDB := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, rawDB, loggateway.NewNoop())
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	activity, ok := repo.(biz.SpiritStepActivityReader)
	if !ok {
		t.Fatal("stepV2Repo must implement biz.SpiritStepActivityReader")
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seed := []biz.Step{
		{ID: "st-a1", TurnID: "turn-1", TaskID: "t-1", SessionID: "team-1", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 1, Status: biz.StepStatusCompleted, StartedAt: now.Add(-time.Hour), Version: 1},
		{ID: "st-a2", TurnID: "turn-2", TaskID: "t-1", SessionID: "member-1", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 2, Status: biz.StepStatusCompleted, StartedAt: now.Add(-time.Minute), Version: 1},
		{ID: "st-other", TurnID: "turn-3", TaskID: "t-9", SessionID: "other-team", SpiritSessionID: "spirit-1", Kind: biz.StepKindReply, Seq: 3, Status: biz.StepStatusCompleted, StartedAt: now, Version: 1},
	}
	for i, s := range seed {
		if _, err := repo.CreateStep(ctx, s); err != nil {
			t.Fatalf("CreateStep[%d]: %v", i, err)
		}
	}
	// 模拟迁移 20261276 回填：存量行 updated_at = started_at（CreateStep 走
	// ent Default 会把 updated_at 填成插入时刻 ≈now，掩盖 started_at 语义，
	// 回填恢复「既有行保留原始新鲜度」的迁移语义后再断言）。
	if _, err := rawDB.ExecContext(ctx, `UPDATE steps_v2 SET updated_at = started_at`); err != nil {
		t.Fatalf("backfill updated_at: %v", err)
	}

	// 团队会话树（主会话 + 成员子会话）：聚合取最新，不混入其他团队的会话。
	latest, err := activity.LatestStepActivityAt(ctx, []string{"team-1", "member-1"})
	if err != nil {
		t.Fatalf("LatestStepActivityAt: %v", err)
	}
	if !latest.Equal(now.Add(-time.Minute)) {
		t.Fatalf("latest=%v, want %v (member-1 step, not other-team)", latest, now.Add(-time.Minute))
	}

	// P4-2 核心新语义：原地更新（无新 step 启动）刷新 updated_at → 探测
	// 信号提升到 ≈现在。
	if _, err := repo.UpdateStep(ctx, biz.Step{ID: "st-a2", Content: "progress", Status: biz.StepStatusCompleted, Version: 2}); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	bumped, err := activity.LatestStepActivityAt(ctx, []string{"team-1", "member-1"})
	if err != nil {
		t.Fatalf("LatestStepActivityAt after update: %v", err)
	}
	if bumped.Before(now) {
		t.Fatalf("after in-place update latest=%v, want >= %v (updated_at refreshed by UpdateDefault)", bumped, now)
	}

	// 无 step 的会话集合 → 零值无错误。
	zero, err := activity.LatestStepActivityAt(ctx, []string{"no-such-session"})
	if err != nil {
		t.Fatalf("LatestStepActivityAt empty: %v", err)
	}
	if !zero.IsZero() {
		t.Fatalf("latest=%v, want zero for unknown sessions", zero)
	}

	// 空会话列表 → 零值无查询。
	if v, err := activity.LatestStepActivityAt(ctx, nil); err != nil || !v.IsZero() {
		t.Fatalf("LatestStepActivityAt(nil) = %v, %v; want zero, nil", v, err)
	}
}
