package service

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"path/filepath"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newTestL2Service 搭建内存 L2 栈（repo + L1 + L2）。L2 接真实 L1Service，使 ArchiveL1Task 经
// SnapshotForEpisode 贯通。不需要归档的测试可忽略返回的 L1Service。
func newTestL2Service(t *testing.T) (*MemoryL2Service, *MemoryL1Service, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "l2.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	l1 := NewMemoryL1Service(repo)
	l2 := NewMemoryL2Service(repo)
	l2.SetL1Source(l1)
	return l2, l1, repo
}

// seedAgentSettings 打开 L2 开关以跑特性门控路径。规范默认 L2 episode 开、recall 关；测试
// 显式开启 recall。
func seedAgentSettings(t *testing.T, repo repository.Store, agentID string, recall bool) {
	t.Helper()
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:                agentID,
		L2EpisodeEnabled:       true,
		L2EpisodeMinImportance: 0.3,
		L2IndexEnabled:         true,
		L2RecallEnabled:        recall,
		L2RecallMax:            3,
		L2RetentionDays:        90,
		L2ArchiveAfterDays:     30,
	}); err != nil {
		t.Fatalf("upsert agent runtime settings failed: %v", err)
	}
}

func TestL2ArchiveL1TaskCreatesEpisode(t *testing.T) {
	l2, l1, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-l2-a", "sess-l2-a")
	seedAgentSettings(t, repo, "agent-l2-a", false)

	task, err := l1.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-l2-a",
		AgentID:   "agent-l2-a",
		TaskKey:   "default",
		TaskTitle: "ship dark mode",
		TaskGoal:  "deliver a dark-mode toggle",
	})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}
	if _, err = l1.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "decisions.theme", FieldKind: "string", Value: "tailwind dark variant",
	}); err != nil {
		t.Fatalf("set decision failed: %v", err)
	}
	if err = l1.EndTask(context.Background(), task.ID, mem.L1TaskCompleted); err != nil {
		t.Fatalf("end task failed: %v", err)
	}

	episode, err := l2.ArchiveL1Task(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if episode.ID == "" {
		t.Fatalf("expected episode to be created")
	}
	if episode.SessionID != "sess-l2-a" || episode.AgentID != "agent-l2-a" {
		t.Fatalf("unexpected scope: session=%s agent=%s", episode.SessionID, episode.AgentID)
	}
	if episode.Kind != mem.EpisodeKindTask {
		t.Fatalf("expected task kind, got %s", episode.Kind)
	}
	if episode.Outcome != "success" {
		t.Fatalf("expected success outcome, got %s", episode.Outcome)
	}
	if episode.Title != "ship dark mode" {
		t.Fatalf("expected title 'ship dark mode', got %q", episode.Title)
	}
	if episode.L1TaskID != task.ID {
		t.Fatalf("expected l1_task_id linkage, got %q", episode.L1TaskID)
	}
	if episode.Importance < 0.5 {
		// 已完成任务 + 至少一字段 => 基线 0.3 + 0.2（完成）= 0.5 下限。更低表示
		// 未套用重要性公式。
		t.Fatalf("expected importance >= 0.5, got %f", episode.Importance)
	}
	if episode.ConsolidationStatus != "pending" {
		t.Fatalf("expected consolidation pending, got %q", episode.ConsolidationStatus)
	}

	// 幂等：再次归档返回同一条 episode，无重复行。
	again, err := l2.ArchiveL1Task(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("second archive failed: %v", err)
	}
	if again.ID != episode.ID {
		t.Fatalf("expected idempotent archive, got %s vs %s", again.ID, episode.ID)
	}
	list, _, err := repo.ListEpisodes("sess-l2-a", "", 50, 0)
	if err != nil {
		t.Fatalf("list episodes failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one episode after re-archive, got %d", len(list))
	}
}

func TestL2ArchiveSkippedWhenDisabled(t *testing.T) {
	l2, l1, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-l2-off", "sess-l2-off")
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:          "agent-l2-off",
		L2EpisodeEnabled: false,
	}); err != nil {
		t.Fatalf("upsert settings failed: %v", err)
	}
	task, err := l1.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-l2-off", AgentID: "agent-l2-off", TaskKey: "default",
	})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}
	if err = l1.EndTask(context.Background(), task.ID, mem.L1TaskCompleted); err != nil {
		t.Fatalf("end task failed: %v", err)
	}
	ep, err := l2.ArchiveL1Task(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("archive should not error when disabled: %v", err)
	}
	if ep.ID != "" {
		t.Fatalf("expected empty episode when disabled, got %#v", ep)
	}
	list, _, err := repo.ListEpisodes("sess-l2-off", "", 50, 0)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected zero episodes when disabled, got %d", len(list))
	}
}

func TestL2CreateMilestoneEpisode(t *testing.T) {
	l2, _, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-ms", "sess-ms")
	seedAgentSettings(t, repo, "agent-ms", false)

	ep, err := l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID:      "sess-ms",
		AgentID:        "agent-ms",
		Title:          "alpha launch",
		Goal:           "ship v1 to internal users",
		OutcomeSummary: "rolled out behind a feature flag",
		Importance:     0.8,
	})
	if err != nil {
		t.Fatalf("create milestone failed: %v", err)
	}
	if ep.Kind != mem.EpisodeKindMilestone {
		t.Fatalf("expected milestone kind, got %s", ep.Kind)
	}
	if ep.Importance != 0.8 {
		t.Fatalf("expected importance preserved, got %f", ep.Importance)
	}
	if ep.Outcome != "success" {
		t.Fatalf("expected default outcome 'success', got %q", ep.Outcome)
	}

	// 空标题应由校验拒绝。
	if _, err = l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-ms", AgentID: "agent-ms",
	}); err == nil {
		t.Fatalf("expected validation error for missing title")
	}
}

func TestL2ListEventsReturnsMessages(t *testing.T) {
	l2, _, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-ev", "sess-ev")

	for _, m := range []domain.Message{
		{ID: "m-user", SessionID: "sess-ev", Role: "user", Content: "hello L2", Status: "ok", CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "m-asst", SessionID: "sess-ev", Role: "assistant", Content: "hi back", Status: "ok", TokenIn: 10, TokenOut: 20, CreatedAt: "2026-01-01T00:00:01Z"},
	} {
		if _, err := repo.AddMessage(m); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	result, err := l2.ListEvents(context.Background(), mem.MemoryL2EventQuery{
		SessionID: "sess-ev",
	})
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if result.Total < 2 {
		t.Fatalf("expected at least 2 events, got %d", result.Total)
	}
	var sawUser, sawAsst bool
	for _, ev := range result.Items {
		if ev.Kind != "message" {
			continue
		}
		switch ev.ID {
		case "m-user":
			sawUser = true
		case "m-asst":
			sawAsst = true
		}
	}
	if !sawUser || !sawAsst {
		t.Fatalf("expected both messages in event stream, got user=%v asst=%v", sawUser, sawAsst)
	}

	// 关键词过滤应缩小结果集。
	filtered, err := l2.ListEvents(context.Background(), mem.MemoryL2EventQuery{
		SessionID: "sess-ev", Keyword: "hi back",
	})
	if err != nil {
		t.Fatalf("filtered list failed: %v", err)
	}
	if filtered.Total == 0 {
		t.Fatalf("expected at least one keyword hit")
	}
	for _, ev := range filtered.Items {
		if ev.Kind == "message" && ev.ID == "m-user" {
			t.Fatalf("user message should have been filtered out by keyword")
		}
	}
}

func TestL2MarkBumpsImportance(t *testing.T) {
	l2, _, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-mk", "sess-mk")
	seedAgentSettings(t, repo, "agent-mk", false)

	ep, err := l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-mk", AgentID: "agent-mk", Title: "candidate", Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("create episode failed: %v", err)
	}

	stored, err := l2.Mark(context.Background(), MarkInput{
		EpisodeID: ep.ID,
		RefKind:   "episode",
		RefID:     ep.ID,
		MarkType:  "star",
		MarkedBy:  "user:alice",
		Reason:    "great example",
	})
	if err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if stored.ID == "" {
		t.Fatalf("expected mark id to be assigned")
	}
	if stored.SessionID != "sess-mk" {
		t.Fatalf("expected session resolved from episode, got %q", stored.SessionID)
	}

	updated, err := repo.GetEpisode(ep.ID)
	if err != nil {
		t.Fatalf("get episode failed: %v", err)
	}
	if updated.Importance < 0.69 || updated.Importance > 0.71 {
		// 0.5 + 0.2 (star) = 0.7
		t.Fatalf("expected importance ~0.7, got %f", updated.Importance)
	}

	marks, err := l2.ListMarks(context.Background(), "sess-mk", "", 50)
	if err != nil {
		t.Fatalf("list marks failed: %v", err)
	}
	if len(marks) != 1 || marks[0].ID != stored.ID {
		t.Fatalf("unexpected marks: %#v", marks)
	}

	// UnMark 为软删；重要度保持，避免再次标星重复加算。
	if err = l2.UnMark(context.Background(), stored.ID); err != nil {
		t.Fatalf("unmark failed: %v", err)
	}
	after, err := l2.ListMarks(context.Background(), "sess-mk", "", 50)
	if err != nil {
		t.Fatalf("list marks after unmark failed: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected marks to be soft-deleted, got %d", len(after))
	}
}

func TestL2RecallByQueryFromBM25(t *testing.T) {
	l2, _, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-rc", "sess-rc")
	seedAgentSettings(t, repo, "agent-rc", true)

	// 两条 episode：对「dark theme」查询，dark-mode 应排在 auth 之上。
	dark, err := l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-rc", AgentID: "agent-rc",
		Title: "dark mode rollout",
		Goal:  "implement dark theme toggle across the app",
		OutcomeSummary: "shipped tailwind dark variant",
		Importance: 0.7,
	})
	if err != nil {
		t.Fatalf("create dark episode failed: %v", err)
	}
	if _, err = l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-rc", AgentID: "agent-rc",
		Title: "auth migration",
		Goal:  "swap auth provider to clerk",
		OutcomeSummary: "completed handoff",
		Importance: 0.6,
	}); err != nil {
		t.Fatalf("create auth episode failed: %v", err)
	}

	results, err := l2.RecallByQuery(context.Background(), mem.MemoryL2RecallQuery{
		SessionID: "sess-rc",
		AgentID:   "agent-rc",
		Query:     "dark theme",
		TopK:      3,
	})
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one recall result")
	}
	if results[0].Episode.ID != dark.ID {
		t.Fatalf("expected dark mode episode at the top, got %q (%s)",
			results[0].Episode.Title, results[0].Episode.ID)
	}
	if results[0].FinalRank <= 0 {
		t.Fatalf("expected positive fused rank, got %f", results[0].FinalRank)
	}

	// recall 开启且有命中时应渲染 L0 片段。
	seg, ok := l2.RecallSegmentForL0(context.Background(), "sess-rc", "agent-rc", "dark theme")
	if !ok {
		t.Fatalf("expected an L0 segment from recall")
	}
	if seg.Section != "memory.l2" {
		t.Fatalf("expected memory.l2 section, got %q", seg.Section)
	}
	if seg.Tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", seg.Tokens)
	}
	_ = repo
}

func TestL2BuildIndexForUpsertsFTS(t *testing.T) {
	l2, _, repo := newTestL2Service(t)
	seedAgentAndSession(t, repo, "agent-ix", "sess-ix")
	seedAgentSettings(t, repo, "agent-ix", true)

	ep, err := l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-ix", AgentID: "agent-ix",
		Title: "indexable", Goal: "verify reindex pipeline",
		OutcomeSummary: "the reindex round-trips text",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("create episode failed: %v", err)
	}

	// CreateMilestoneEpisode 在索引开启时已调 BuildIndexFor，
	// 再显式调用须为安全 upsert。
	if err = l2.BuildIndexFor(context.Background(), ep.ID); err != nil {
		t.Fatalf("reindex failed: %v", err)
	}
	if err = l2.BuildIndexFor(context.Background(), ep.ID); err != nil {
		t.Fatalf("second reindex (upsert) failed: %v", err)
	}

	// 该 episode 现应可通过 BM25 搜到。
	hits, err := repo.SearchL2BM25("sess-ix", "reindex", 0, 5)
	if err != nil {
		t.Fatalf("bm25 search failed: %v", err)
	}
	if len(hits) == 0 || hits[0].Episode.ID != ep.ID {
		t.Fatalf("expected the indexed episode to surface, got %#v", hits)
	}

	// SoftDeleteEpisode 清 FTS 行；后续搜索不应再命中该 episode。
	if err = l2.DeleteEpisode(context.Background(), ep.ID); err != nil {
		t.Fatalf("delete episode failed: %v", err)
	}
	post, err := repo.SearchL2BM25("sess-ix", "reindex", 0, 5)
	if err != nil {
		t.Fatalf("post-delete search failed: %v", err)
	}
	if len(post) != 0 {
		t.Fatalf("expected zero hits after soft delete, got %d", len(post))
	}
}
