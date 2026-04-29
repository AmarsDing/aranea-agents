package service

import (
	mem "arenea/backend/internal/memory/domain"

	"arenea/backend/internal/kernel/errs"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newTestL1Service 在 t.TempDir 下新建 SQLite、执行迁移，并返回已接线的 MemoryL1Service
// 与底层 repo，便于测试直接造数。
func newTestL1Service(t *testing.T) (*MemoryL1Service, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "l1.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return NewMemoryL1Service(repo), repo
}

// seedAgentAndSession 为小型造数。每测使用唯一 (agent, session) 对，并行时
// 不违反 (session_id, task_key, agent_id) 唯一约束。
func seedAgentAndSession(t *testing.T, repo repository.Store, agentID, sessionID string) {
	t.Helper()
	if _, err := repo.CreateAgent(domain.Agent{
		ID: agentID, AgentKey: agentID, DisplayName: agentID,
		Provider: "openrouter", Model: "gpt-4.1-mini", Status: "active",
	}); err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	if _, err := repo.CreateSession(domain.Session{
		ID: sessionID, AgentID: agentID, Title: "session " + sessionID,
	}); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
}

func TestL1StartTaskIsIdempotentAndSeedsGoal(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-a", "sess-a")

	first, err := svc.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-a",
		AgentID:   "agent-a",
		TaskKey:   "default",
		TaskGoal:  "Implement L1",
	})
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	if first.Status != mem.L1TaskActive {
		t.Fatalf("expected active status, got %s", first.Status)
	}
	field, err := repo.GetL1Field(first.ID, "task_goal")
	if err != nil {
		t.Fatalf("expected task_goal field, got %v", err)
	}
	if field.ValueText != "Implement L1" {
		t.Fatalf("unexpected task_goal value: %q", field.ValueText)
	}

	again, err := svc.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-a", AgentID: "agent-a", TaskKey: "default", TaskGoal: "ignored",
	})
	if err != nil {
		t.Fatalf("idempotent start failed: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("expected same task id on second start, got %s vs %s", again.ID, first.ID)
	}
}

func TestL1SetFieldEnforcesPathGrammar(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-b", "sess-b")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{SessionID: "sess-b", AgentID: "agent-b", TaskKey: "default"})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	bad := []string{"", "1bad", "with space", "weird$char", strings.Repeat("a", 300)}
	for _, path := range bad {
		_, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
			FieldPath: path, FieldKind: "string", Value: "x",
		})
		if !errors.Is(err, errs.ErrInvalidFieldPath) {
			t.Fatalf("expected invalid path error for %q, got %v", path, err)
		}
	}
}

func TestL1SetFieldRevisionsAndOptimisticLock(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-c", "sess-c")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{SessionID: "sess-c", AgentID: "agent-c", TaskKey: "default"})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	first, err := svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "subtasks", FieldKind: "json", Value: []map[string]any{{"id": "s1", "status": "pending"}},
	})
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if first.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", first.Revision)
	}

	expected := 1
	second, err := svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "subtasks", FieldKind: "json", Value: []map[string]any{{"id": "s1", "status": "running"}},
		IfRevision: &expected,
	})
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if second.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", second.Revision)
	}

	stale := 1
	_, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "subtasks", FieldKind: "json", Value: []map[string]any{{"id": "s1", "status": "done"}},
		IfRevision: &stale,
	})
	if !errors.Is(err, errs.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

func TestL1SetFieldRejectsTooLargeFieldAndOverflow(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-d", "sess-d")
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:                "agent-d",
		L1Enabled:              true,
		L1BudgetTokens:         100,
		L1FieldMaxTokens:       40,
		L1HistoryKeepRevisions: 4,
	}); err != nil {
		t.Fatalf("upsert agent settings failed: %v", err)
	}
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-d", AgentID: "agent-d", TaskKey: "default", BudgetTokens: 100,
	})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	huge := strings.Repeat("x", 50*4) // ~50 tokens via the 4-rune heuristic
	_, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "huge", FieldKind: "string", Value: huge,
	})
	if !errors.Is(err, errs.ErrFieldTooLarge) {
		t.Fatalf("expected ErrFieldTooLarge, got %v", err)
	}

	mid := strings.Repeat("y", 35*4) // ~35 tokens
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "alpha", FieldKind: "string", Value: mid,
	}); err != nil {
		t.Fatalf("first mid write failed: %v", err)
	}
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "beta", FieldKind: "string", Value: mid,
	}); err != nil {
		t.Fatalf("second mid write failed: %v", err)
	}
	_, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "gamma", FieldKind: "string", Value: mid,
	})
	if !errors.Is(err, errs.ErrL1Overflow) {
		t.Fatalf("expected ErrL1Overflow, got %v", err)
	}

	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "internal_note", FieldKind: "string", Visibility: "internal", Value: mid,
	}); err != nil {
		t.Fatalf("internal write should bypass budget, got %v", err)
	}
}

func TestL1RenderForPromptAndMissingFields(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-e", "sess-e")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-e", AgentID: "agent-e", TaskKey: "default", TaskTitle: "Dark mode",
	})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	required := true
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "task_goal", FieldKind: "string", Value: "Add dark mode toggle", IsRequired: &required,
	}); err != nil {
		t.Fatalf("set task_goal failed: %v", err)
	}
	pinFalse := false
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "secret_note", FieldKind: "string", Value: "internal", Visibility: "internal", PinToPrompt: &pinFalse,
	}); err != nil {
		t.Fatalf("set internal field failed: %v", err)
	}
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "open_questions", FieldKind: "string", Value: "", IsRequired: &required,
	}); err != nil {
		t.Fatalf("set open_questions placeholder failed: %v", err)
	}

	block, err := svc.RenderForPrompt(context.Background(), task.ID, "agent-e")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(block.Content, "task_goal") || !strings.Contains(block.Content, "Add dark mode toggle") {
		t.Fatalf("expected task_goal in rendered content, got %q", block.Content)
	}
	if strings.Contains(block.Content, "secret_note") {
		t.Fatalf("internal field leaked into prompt: %q", block.Content)
	}
	if !strings.Contains(strings.Join(block.MissingFields, ","), "open_questions") {
		t.Fatalf("expected open_questions to be reported missing, got %v", block.MissingFields)
	}
	if block.TaskID != task.ID {
		t.Fatalf("expected TaskID %s, got %s", task.ID, block.TaskID)
	}
}

func TestL1EndTaskAndSnapshot(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-f", "sess-f")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{SessionID: "sess-f", AgentID: "agent-f", TaskKey: "default"})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "decision", FieldKind: "string", Value: "ship",
	}); err != nil {
		t.Fatalf("set decision failed: %v", err)
	}

	if err = svc.EndTask(context.Background(), task.ID, mem.L1TaskCompleted); err != nil {
		t.Fatalf("end task failed: %v", err)
	}
	got, err := repo.GetL1TaskByID(task.ID)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if got.Status != mem.L1TaskCompleted {
		t.Fatalf("expected completed status, got %s", got.Status)
	}
	if got.EndedAt == "" {
		t.Fatalf("expected ended_at to be set")
	}

	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "decision", FieldKind: "string", Value: "rollback",
	}); !errors.Is(err, errs.ErrTaskNotWritable) {
		t.Fatalf("expected ErrTaskNotWritable on terminal task, got %v", err)
	}

	episode, err := svc.SnapshotForEpisode(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if episode.TaskID != task.ID || episode.Stats["fields"] == 0 {
		t.Fatalf("unexpected snapshot %#v", episode)
	}
	snap, ok := episode.Snapshot["decision"].(map[string]any)
	if !ok || snap["value"] != "ship" {
		t.Fatalf("expected decision in snapshot, got %#v", episode.Snapshot)
	}
}

func TestL1RollbackFieldRestoresValue(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-g", "sess-g")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{SessionID: "sess-g", AgentID: "agent-g", TaskKey: "default"})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "plan", FieldKind: "string", Value: "v1",
	}); err != nil {
		t.Fatalf("set v1 failed: %v", err)
	}
	if _, err = svc.SetField(context.Background(), task.ID, mem.L1FieldPatch{
		FieldPath: "plan", FieldKind: "string", Value: "v2",
	}); err != nil {
		t.Fatalf("set v2 failed: %v", err)
	}

	rolled, err := svc.RollbackField(context.Background(), task.ID, "plan", 1, "user:test")
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if rolled.ValueText != "v1" {
		t.Fatalf("expected rolled value v1, got %q", rolled.ValueText)
	}
	if rolled.Revision != 3 {
		t.Fatalf("rollback should produce a new revision (got %d)", rolled.Revision)
	}
}

func TestL1RenderActiveTaskForPromptHonoursAgent(t *testing.T) {
	svc, repo := newTestL1Service(t)
	seedAgentAndSession(t, repo, "agent-h", "sess-h")
	task, err := svc.StartTask(context.Background(), StartL1TaskInput{
		SessionID: "sess-h", AgentID: "agent-h", TaskKey: "default", TaskGoal: "answer the question",
	})
	if err != nil {
		t.Fatalf("start task failed: %v", err)
	}

	block, ok, err := svc.RenderActiveTaskForPrompt(context.Background(), "sess-h", "agent-h")
	if err != nil {
		t.Fatalf("render active failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected active block")
	}
	if block.TaskID != task.ID {
		t.Fatalf("unexpected task id: %s", block.TaskID)
	}

	if _, _, err = svc.RenderActiveTaskForPrompt(context.Background(), "missing-session", "agent-h"); err != nil {
		t.Fatalf("missing session should be a no-op, got %v", err)
	}
}
