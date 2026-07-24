package data_test

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"
	trpcmem "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// openTestDataForMemory opens an isolated Postgres test schema with the Ent
// schema plus the raw-SQL-managed memory tables (DDL-migration managed in
// production, created here directly for test speed).
func openTestDataForMemory(t *testing.T) (*data.Data, *ent.Client) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS memory_facts (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '',
 statement TEXT NOT NULL, statement_normalized TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', details_markdown TEXT NOT NULL DEFAULT '',
 fact_kind TEXT NOT NULL DEFAULT 'fact', tags_json TEXT NOT NULL DEFAULT '[]',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 use_count INTEGER NOT NULL DEFAULT 0, hit_count INTEGER NOT NULL DEFAULT 0,
 positive_feedback_count INTEGER NOT NULL DEFAULT 0, negative_feedback_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
 source_kind TEXT NOT NULL DEFAULT 'manual', source_episode_id TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', source_external TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active', superseded_by TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BYTEA, embedding_norm REAL NOT NULL DEFAULT 0,
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '', pii_types TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', quality_score REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 valid_from TEXT NOT NULL DEFAULT '', valid_until TEXT NOT NULL DEFAULT '',
 links TEXT NOT NULL DEFAULT '[]', keywords TEXT NOT NULL DEFAULT '[]', tags TEXT NOT NULL DEFAULT '[]',
 decay_score REAL NOT NULL DEFAULT 1.0, context_note TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, fingerprint))`,
		`CREATE TABLE IF NOT EXISTS memory_action_log (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
 source_event_ids_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memory_entities (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '', workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '',
 entity_type TEXT NOT NULL, name TEXT NOT NULL, name_normalized TEXT NOT NULL, aliases_json TEXT NOT NULL DEFAULT '[]',
 description TEXT NOT NULL DEFAULT '', attributes_json TEXT NOT NULL DEFAULT '{}',
 importance REAL NOT NULL DEFAULT 0.5, confidence REAL NOT NULL DEFAULT 0.7, use_count INTEGER NOT NULL DEFAULT 0, source_kind TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BYTEA, embedding_norm REAL NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'active', merged_into TEXT NOT NULL DEFAULT '', metadata_json TEXT NOT NULL DEFAULT '{}',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 activation REAL NOT NULL DEFAULT 0, activation_updated_at TEXT NOT NULL DEFAULT '', source_type TEXT NOT NULL DEFAULT '',
 valence REAL NOT NULL DEFAULT 0, arousal REAL NOT NULL DEFAULT 0,
 UNIQUE(scope_type, scope_id, entity_type, name_normalized))`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
 version INTEGER PRIMARY KEY NOT NULL,
 name TEXT NOT NULL,
 applied_at TEXT NOT NULL)`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	// Create a minimal Data with the test client
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return d, client
}

// enabledSettingsLoader is a test stub that returns memory-enabled runtime
// settings so resolveMemoryToolSearchLimits honours the default topK/minScore
// policy. Without it, a nil loader causes MasterEnabled=false and ReadMemories
// returns empty (see commit ee9691ec8b).
type enabledSettingsLoader struct{}

func (enabledSettingsLoader) GetAgentRuntimeSettings(_ context.Context, _ string) (*biz.AgentRuntimeSettings, error) {
	return &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true}, nil
}

func TestMemoryService_AddMemoryWritesFactVisibleToAdmin(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.AddMemory(ctx, uk, "My name is Alice", []string{"profile"}); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	l3 := data.NewL3FactReaderForUser(d)
	rows, total, _, _, err := l3.ListFactRows(ctx, "agent", "agent-1", "", "active", "", 20, 0)
	if err != nil {
		t.Fatalf("ListFactRows: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 fact, got total=%d rows=%d", total, len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if got := m["statement"]; got != "My name is Alice" {
		t.Fatalf("statement=%v", got)
	}
	if got := m["source_kind"]; got != "trpc_memory" {
		t.Fatalf("source_kind=%v", got)
	}
}

func TestMemoryService_AddMemoryDedupByFingerprint(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-dedup", UserID: "user-dedup"}
	stmt := "I prefer tea in the morning"
	if err := svc.AddMemory(ctx, uk, stmt, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMemory(ctx, uk, stmt, nil); err != nil {
		t.Fatal(err)
	}
	l3 := data.NewL3FactReaderForUser(d)
	rows, total, _, _, err := l3.ListFactRows(ctx, "agent", "agent-dedup", "", "active", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 deduped fact, got total=%d rows=%d", total, len(rows))
	}
}

func TestMemoryService_ReadMemoriesFromFacts(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-2", UserID: "user-2"}
	if err := svc.AddMemory(ctx, uk, "I prefer dark mode", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Memory.Memory != "I prefer dark mode" {
		t.Fatalf("memory=%q", entries[0].Memory.Memory)
	}
}

func TestMemoryService_DeleteAndClear(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-3", UserID: "user-3"}
	if err := svc.AddMemory(ctx, uk, "fact one", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read: err=%v len=%d", err, len(entries))
	}
	mk := trpcmemory.Key{AppName: uk.AppName, UserID: uk.UserID, MemoryID: entries[0].ID}
	if err := svc.DeleteMemory(ctx, mk); err != nil {
		t.Fatal(err)
	}
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(entries))
	}
	if err := svc.AddMemory(ctx, uk, "fact two", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearMemories(ctx, uk); err != nil {
		t.Fatal(err)
	}
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 after clear, got %d", len(entries))
	}
}

func TestMemoryAdminUsecase_RequireAdminWhenStoreMissing(t *testing.T) {
	uc := biz.NewMemoryAdminUsecase(nil, &biz.MemoryUsecase{}, nil, nil, loggateway.NewNoop())
	if uc == nil {
		t.Fatal("expected vec-only usecase")
	}
	_, _, _, _, err := uc.ListFactRows(context.Background(), biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   "a1",
		Limit:     10,
		Offset:    0,
	})
	if err == nil {
		t.Fatal("expected error when admin store missing")
	}
}

// TestAddMemory_SetsValidFrom verifies that AddMemory populates ValidFrom
// on the stored fact (bi-temporal P3-8).
func TestAddMemory_SetsValidFrom(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-vf", UserID: "user-vf"}
	if err := svc.AddMemory(ctx, uk, "I live in Paris", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ValidFrom == nil {
		t.Fatal("expected ValidFrom to be set on entry")
	}
	if entries[0].Memory.ValidFrom == nil {
		t.Fatal("expected Memory.ValidFrom to be set")
	}
	if entries[0].ValidUntil != nil {
		t.Fatalf("expected ValidUntil to be nil for new memory, got %v", entries[0].ValidUntil)
	}
}

// TestSearchMemories_FiltersInvalidated verifies that facts with ValidUntil
// set are excluded from SearchMemories results (bi-temporal P3-8).
func TestSearchMemories_FiltersInvalidated(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-inv", UserID: "user-inv"}
	if err := svc.AddMemory(ctx, uk, "I like coffee", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("setup: err=%v len=%d", err, len(entries))
	}
	factID := entries[0].ID
	// Invalidate the fact directly via the writer.
	if _, err := factWriter.InvalidateFact(ctx, factID); err != nil {
		t.Fatalf("InvalidateFact: %v", err)
	}
	// SearchMemories should not return the invalidated fact.
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after invalidation, got %d", len(entries))
	}
}

// TestSearchMemories_IncludesValid verifies that facts without ValidUntil
// (currently valid) appear in search results.
func TestSearchMemories_IncludesValid(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-valid", UserID: "user-valid"}
	if err := svc.AddMemory(ctx, uk, "I like tea", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMemory(ctx, uk, "I like hiking", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.ValidUntil != nil {
			t.Fatalf("expected nil ValidUntil for valid entry %s, got %v", e.ID, e.ValidUntil)
		}
	}
}

// TestUpdateMemory_InvalidatesOldOnConflict verifies that updating a memory
// with different content invalidates the old fact (sets ValidUntil) and
// creates a new fact, preserving history (bi-temporal P3-8).
func TestUpdateMemory_InvalidatesOldOnConflict(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	svc := trpcmem.NewMemoryService(data.NewL3FactReaderForUser(d), factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-conf", UserID: "user-conf"}
	// Add initial memory.
	if err := svc.AddMemory(ctx, uk, "I live in London", nil); err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("setup: err=%v len=%d", err, len(entries))
	}
	oldID := entries[0].ID
	// Update with different content — should invalidate old, create new.
	result := &trpcmemory.UpdateResult{}
	mk := trpcmemory.Key{AppName: uk.AppName, UserID: uk.UserID, MemoryID: oldID}
	if err := svc.UpdateMemory(ctx, mk, "I live in Berlin", nil, trpcmemory.WithUpdateResult(result)); err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}
	if result.MemoryID == "" {
		t.Fatal("expected non-empty new memory ID in UpdateResult")
	}
	if result.MemoryID == oldID {
		t.Fatal("expected new memory ID to differ from old ID on content conflict")
	}
	// SearchMemories should return only the new (valid) fact.
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry after update, got %d", len(entries))
	}
	if entries[0].ID != result.MemoryID {
		t.Fatalf("expected entry ID %q, got %q", result.MemoryID, entries[0].ID)
	}
	if entries[0].Memory.Memory != "I live in Berlin" {
		t.Fatalf("expected updated content, got %q", entries[0].Memory.Memory)
	}
	// The old fact should still exist in the DB but with ValidUntil set.
	// Use the admin reader (ListFactRows) which does NOT filter valid_until.
	l3 := data.NewL3FactReaderForUser(d)
	rows, _, _, _, err := l3.ListFactRows(ctx, "agent", "agent-conf", "", "", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	var foundOld bool
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m["id"] == oldID {
			foundOld = true
			if vu, _ := m["valid_until"].(string); vu == "" {
				t.Fatal("expected old fact to have valid_until set after conflict invalidation")
			}
		}
	}
	if !foundOld {
		t.Fatal("expected old fact to still exist in DB after invalidation")
	}
}

// TestInvalidateFact_DataLayer verifies the data-layer InvalidateFact method
// sets valid_until and preserves the row.
func TestInvalidateFact_DataLayer(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	reader := data.NewL3FactReaderForUser(d)
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-inv-dl", UserID: "user-inv-dl"}
	if err := trpcmem.NewMemoryService(reader, factWriter, nil, nil, nil, enabledSettingsLoader{}, nil, nil, loggateway.NewNoop()).
		AddMemory(ctx, uk, "I like running", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := reader.ListFactRowsForUser(ctx, "agent", "agent-inv-dl", "user-inv-dl", "", 10, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("setup: err=%v len=%d", err, len(rows))
	}
	var m map[string]any
	_ = json.Unmarshal(rows[0], &m)
	factID, _ := m["id"].(string)
	if factID == "" {
		t.Fatal("expected non-empty fact ID")
	}
	raw, err := factWriter.InvalidateFact(ctx, factID)
	if err != nil {
		t.Fatalf("InvalidateFact: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty raw row from InvalidateFact")
	}
	var inv map[string]any
	if json.Unmarshal(raw, &inv) != nil {
		t.Fatal("failed to unmarshal invalidated row")
	}
	if vu, _ := inv["valid_until"].(string); vu == "" {
		t.Fatal("expected valid_until to be set after invalidation")
	}
	// The fact should no longer appear in user-facing queries.
	rows, err = reader.ListFactRowsForUser(ctx, "agent", "agent-inv-dl", "user-inv-dl", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after invalidation, got %d", len(rows))
	}
}
