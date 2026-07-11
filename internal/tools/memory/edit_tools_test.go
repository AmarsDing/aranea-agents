package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// --- Mock implementations ---

type mockL3Store struct {
	facts map[string]map[string]any // factID → fact row
}

func newMockL3Store() *mockL3Store {
	return &mockL3Store{facts: make(map[string]map[string]any)}
}

func (m *mockL3Store) seedFact(factID, statement string) {
	m.facts[factID] = map[string]any{
		"id":           factID,
		"statement":    statement,
		"scope_type":   "agent",
		"scope_id":     "agent-1",
		"agent_id":     "agent-1",
		"user_id":      "user-1",
		"workspace_id": "",
	}
}

func (m *mockL3Store) GetFactRowsByIDs(_ context.Context, factIDs []string) ([][]byte, error) {
	var out [][]byte
	for _, id := range factIDs {
		if f, ok := m.facts[id]; ok {
			b, _ := json.Marshal(f)
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *mockL3Store) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	if in.ID == "" {
		return nil, apierror.Internal("MEMORY", "missing fact id")
	}
	m.facts[in.ID] = map[string]any{
		"id":           in.ID,
		"statement":    in.Statement,
		"scope_type":   in.ScopeType,
		"scope_id":     in.ScopeID,
		"agent_id":     in.AgentID,
		"user_id":      in.UserID,
		"workspace_id": in.WorkspaceID,
	}
	b, _ := json.Marshal(m.facts[in.ID])
	return b, nil
}

func (m *mockL3Store) getStatement(factID string) string {
	if f, ok := m.facts[factID]; ok {
		if s, ok := f["statement"].(string); ok {
			return s
		}
	}
	return ""
}

// mockIndexSyncer records SyncFactIndexFromRow calls.
type mockIndexSyncer struct {
	syncedIDs []string
	syncErr   error
}

func (m *mockIndexSyncer) SyncFactIndex(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (m *mockIndexSyncer) SyncFactIndexFromRow(_ context.Context, raw []byte) error {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	m.syncedIDs = append(m.syncedIDs, v["id"].(string))
	return m.syncErr
}

// mockActionLog records action log entries.
type mockActionLog struct {
	entries []biz.MemoryPolicyRecord
	logErr  error
}

func (m *mockActionLog) WriteMemoryActionLog(_ context.Context, rec biz.MemoryPolicyRecord) error {
	m.entries = append(m.entries, rec)
	return m.logErr
}

// --- Test helpers ---

func injectEditDeps(ctx context.Context, store *mockL3Store, syncer biz.MemoryFactIndexSyncer, log biz.MemoryActionLogWriter) context.Context {
	ctx = WithL3FactReader(ctx, store)
	ctx = WithL3FactWriter(ctx, store)
	ctx = WithFactIndexSyncer(ctx, syncer)
	ctx = WithActionLogWriter(ctx, log)
	ctx = WithEditAgentID(ctx, "agent-1")
	ctx = WithEditUserID(ctx, "user-1")
	return ctx
}

// --- Tests ---

func TestReplaceTool_Success(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python and JavaScript")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	out, err := replaceExecute(ctx, ReplaceInput{
		MemoryID: "fact-1",
		OldText:  "Python",
		NewText:  "TypeScript",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Success {
		t.Errorf("expected success=true")
	}
	if !strings.Contains(out.NewContent, "TypeScript") {
		t.Errorf("expected new content to contain TypeScript, got: %s", out.NewContent)
	}
	if strings.Contains(out.NewContent, "Python") {
		t.Errorf("expected old text Python to be replaced, got: %s", out.NewContent)
	}

	// Verify fact was updated in store
	if got := store.getStatement("fact-1"); !strings.Contains(got, "TypeScript") {
		t.Errorf("store not updated, got: %s", got)
	}

	// Verify index was re-synced
	if len(syncer.syncedIDs) != 1 || syncer.syncedIDs[0] != "fact-1" {
		t.Errorf("expected index sync for fact-1, got: %v", syncer.syncedIDs)
	}

	// Verify action log
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 action log entry, got %d", len(log.entries))
	}
	if log.entries[0].Action != "replace" {
		t.Errorf("expected action=replace, got: %s", log.entries[0].Action)
	}
}

func TestReplaceTool_OldTextNotFound(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	_, err := replaceExecute(ctx, ReplaceInput{
		MemoryID: "fact-1",
		OldText:  "Java",
		NewText:  "Go",
	})
	if err == nil {
		t.Fatal("expected error for old_text not found")
	}
}

func TestReplaceTool_FactNotFound(t *testing.T) {
	store := newMockL3Store()
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	_, err := replaceExecute(ctx, ReplaceInput{
		MemoryID: "nonexistent",
		OldText:  "a",
		NewText:  "b",
	})
	if err == nil {
		t.Fatal("expected error for fact not found")
	}
}

func TestRethinkTool_Success(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User is a beginner programmer who knows Python")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	out, err := rethinkExecute(ctx, RethinkInput{
		MemoryID:   "fact-1",
		NewContent: "User is an experienced developer proficient in Python, Go, and TypeScript",
		Reason:     "User demonstrated advanced knowledge in recent conversations",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Success {
		t.Errorf("expected success=true")
	}
	if out.NewContent != "User is an experienced developer proficient in Python, Go, and TypeScript" {
		t.Errorf("unexpected new content: %s", out.NewContent)
	}

	// Verify fact was updated
	if got := store.getStatement("fact-1"); got != "User is an experienced developer proficient in Python, Go, and TypeScript" {
		t.Errorf("store not updated correctly, got: %s", got)
	}

	// Verify index was re-synced
	if len(syncer.syncedIDs) != 1 {
		t.Errorf("expected index sync, got: %v", syncer.syncedIDs)
	}

	// Verify action log has reason
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 action log entry, got %d", len(log.entries))
	}
	if log.entries[0].Action != "rethink" {
		t.Errorf("expected action=rethink, got: %s", log.entries[0].Action)
	}
	if !strings.Contains(log.entries[0].Reason, "advanced knowledge") {
		t.Errorf("expected reason in action log, got: %s", log.entries[0].Reason)
	}
}

func TestRethinkTool_EmptyContent(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	_, err := rethinkExecute(ctx, RethinkInput{
		MemoryID:   "fact-1",
		NewContent: "",
		Reason:     "test",
	})
	if err == nil {
		t.Fatal("expected error for empty new_content")
	}
}

func TestInsertTool_Success(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User knows Python and JavaScript")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	out, err := insertExecute(ctx, InsertInput{
		MemoryID:   "fact-1",
		AfterText:  "JavaScript",
		InsertText: " and Go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Success {
		t.Errorf("expected success=true")
	}
	if !strings.Contains(out.NewContent, "Go") {
		t.Errorf("expected new content to contain Go, got: %s", out.NewContent)
	}
	if !strings.Contains(out.NewContent, "Python") {
		t.Errorf("expected new content to still contain Python, got: %s", out.NewContent)
	}

	// Verify action log
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 action log entry, got %d", len(log.entries))
	}
	if log.entries[0].Action != "insert" {
		t.Errorf("expected action=insert, got: %s", log.entries[0].Action)
	}
}

func TestInsertTool_AfterTextNotFound(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	_, err := insertExecute(ctx, InsertInput{
		MemoryID:   "fact-1",
		AfterText:  "Java",
		InsertText: " test",
	})
	if err == nil {
		t.Fatal("expected error for after_text not found")
	}
}

func TestEditTools_IndexSyncFailure_DoesNotBlockEdit(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python")
	syncer := &mockIndexSyncer{syncErr: errors.New("pgvector unavailable")}
	log := &mockActionLog{}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	out, err := replaceExecute(ctx, ReplaceInput{
		MemoryID: "fact-1",
		OldText:  "Python",
		NewText:  "Go",
	})
	if err != nil {
		t.Fatalf("edit should succeed even if index sync fails: %v", err)
	}

	if !out.Success {
		t.Errorf("expected successful edit despite index sync failure")
	}

	// Fact should still be updated
	if got := store.getStatement("fact-1"); !strings.Contains(got, "Go") {
		t.Errorf("store should be updated even if index sync fails, got: %s", got)
	}
}

func TestEditTools_ActionLogFailure_DoesNotBlockEdit(t *testing.T) {
	store := newMockL3Store()
	store.seedFact("fact-1", "User likes Python")
	syncer := &mockIndexSyncer{}
	log := &mockActionLog{logErr: errors.New("action log db down")}

	ctx := injectEditDeps(context.Background(), store, syncer, log)

	out, err := replaceExecute(ctx, ReplaceInput{
		MemoryID: "fact-1",
		OldText:  "Python",
		NewText:  "Go",
	})
	if err != nil {
		t.Fatalf("edit should succeed even if action log fails: %v", err)
	}
	if !out.Success {
		t.Errorf("expected successful edit despite action log failure")
	}
	// Fact should still be updated
	if got := store.getStatement("fact-1"); !strings.Contains(got, "Go") {
		t.Errorf("store should be updated even if action log fails, got: %s", got)
	}
	// Action log should have been attempted
	if len(log.entries) != 1 {
		t.Errorf("expected 1 action log attempt, got %d", len(log.entries))
	}
}

func TestEditTools_MissingDeps(t *testing.T) {
	// No deps injected — should return error
	_, err := replaceExecute(context.Background(), ReplaceInput{
		MemoryID: "fact-1",
		OldText:  "Python",
		NewText:  "Go",
	})
	if err == nil {
		t.Fatal("expected error when deps not injected")
	}
}

func TestAdvancedTools_ReturnsThreeTools(t *testing.T) {
	tools := AdvancedTools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 advanced tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tl := range tools {
		if tl == nil || tl.Declaration() == nil {
			t.Fatal("tool or declaration is nil")
		}
		names[tl.Declaration().Name] = true
	}

	expected := []string{"memory_replace", "memory_rethink", "memory_insert"}
	for _, n := range expected {
		if !names[n] {
			t.Errorf("expected tool %s not found", n)
		}
	}
}
