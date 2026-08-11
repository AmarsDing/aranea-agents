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
// schema plus the raw-SQL-managed memory tables. The memory chain tables are
// created from the PRODUCTION DDL (sql/memory_chain.sql via
// EnsureSessionMemorySchema) instead of a hand-written mirror, so tests can
// never drift from the columns production code actually reads/writes
// (regression guard for the 2026-08-05 "recalled_count does not exist"
// failures caused by a stale mirror).
func openTestDataForMemory(t *testing.T) (*data.Data, *ent.Client) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if err := data.EnsureSessionMemorySchema(ctx, client, data.DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ensure session memory schema: %v", err)
	}
	// schema_migrations is managed by the DDL migration registry, not by
	// memory_chain.sql; the legacy TRPC memory migration tests gate on it.
	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
 version INTEGER PRIMARY KEY NOT NULL,
 name TEXT NOT NULL,
 applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
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
	rows, total, _, _, err := l3.ListFactRows(ctx, "agent", "agent-1", "", "active", "", "", 20, 0)
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
	rows, total, _, _, err := l3.ListFactRows(ctx, "agent", "agent-dedup", "", "active", "", "", 20, 0)
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
	rows, _, _, _, err := l3.ListFactRows(ctx, "agent", "agent-conf", "", "", "", "", 20, 0)
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

// TestReviewFactRow_DataLayer verifies the column-targeted review path
// (memory.md §9.4): feedback actions bump counters/confidence without touching
// links/keywords/metadata, and refine replaces content with a version bump and
// a recomputed fingerprint.
func TestReviewFactRow_DataLayer(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	factWriter := data.NewL3FactWriterAdapter(d, nil)
	admin := data.NewSessionAdminStoreAdapter(d, nil)
	ctx := context.Background()

	raw, err := factWriter.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType: "agent", ScopeID: "agent-review-dl",
		Statement: "User likes tea", FactKind: "preference",
		TagsJSON: `["drink"]`, LinksJSON: `["fact-x"]`, KeywordsJSON: `["tea"]`,
		MetadataJSON: `{"origin":"test"}`, Confidence: 0.95, Importance: 0.6,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	parse := func(raw []byte) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return m
	}
	m := parse(raw)
	factID, _ := m["id"].(string)
	if factID == "" {
		t.Fatal("expected non-empty fact ID")
	}
	if v, _ := m["version"].(float64); v != 1 {
		t.Fatalf("expected version 1 after insert, got %v", v)
	}

	// confirm: positive+1, confidence clamped at 1.0 (0.95 + 0.10 > 1).
	raw, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewConfirm})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	m = parse(raw)
	if v, _ := m["positive_feedback_count"].(float64); v != 1 {
		t.Fatalf("expected positive_feedback_count 1, got %v", v)
	}
	if v, _ := m["confidence"].(float64); v != 1.0 {
		t.Fatalf("expected confidence clamped to 1.0, got %v", v)
	}
	// Regression guard: feedback must not wipe A-MEM linkages/metadata.
	if v, _ := m["links"].(string); v != `["fact-x"]` {
		t.Fatalf("links wiped by confirm: %q", v)
	}
	if v, _ := m["keywords"].(string); v != `["tea"]` {
		t.Fatalf("keywords wiped by confirm: %q", v)
	}
	if v, _ := m["metadata_json"].(string); v != `{"origin":"test"}` {
		t.Fatalf("metadata wiped by confirm: %q", v)
	}

	// reject ×5: confidence 1.0 − 0.20×5 clamps at 0.0.
	for i := 0; i < 5; i++ {
		raw, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewReject})
		if err != nil {
			t.Fatalf("reject %d: %v", i, err)
		}
	}
	m = parse(raw)
	if v, _ := m["negative_feedback_count"].(float64); v != 5 {
		t.Fatalf("expected negative_feedback_count 5, got %v", v)
	}
	// 0.20 is not exactly representable in binary float; after 5 subtractions
	// a tiny positive residue (~3e-08) is expected. Assert the clamp floor.
	if v, _ := m["confidence"].(float64); v < 0.0 || v > 1e-6 {
		t.Fatalf("expected confidence clamped near 0.0, got %v", v)
	}

	// refine: content replace, version 1→2, fingerprint recomputed, links kept.
	raw, err = admin.ReviewFactRow(ctx, biz.FactReview{
		FactID: factID, Action: biz.FactReviewRefine,
		Statement: "User likes coffee", DetailsMarkdown: "edited by user",
		FactKind: "preference", TagsJSON: `["drink","coffee"]`,
	})
	if err != nil {
		t.Fatalf("refine: %v", err)
	}
	m = parse(raw)
	if v, _ := m["statement"].(string); v != "User likes coffee" {
		t.Fatalf("expected refined statement, got %q", v)
	}
	if v, _ := m["version"].(float64); v != 2 {
		t.Fatalf("expected version 2 after refine, got %v", v)
	}
	if v, _ := m["fingerprint"].(string); v == "" || v == biz.FactFingerprint("User likes tea", "agent", "agent-review-dl") {
		t.Fatalf("expected recomputed fingerprint, got %q", v)
	}
	if v, _ := m["embedding_status"].(string); v != "stale" {
		t.Fatalf("expected embedding_status stale after refine, got %q", v)
	}
	if v, _ := m["links"].(string); v != `["fact-x"]` {
		t.Fatalf("links wiped by refine: %q", v)
	}

	// refine with empty statement → error.
	if _, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewRefine}); err == nil {
		t.Fatal("expected error for refine with empty statement")
	}
	// unknown action → error.
	if _, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: "explode"}); err == nil {
		t.Fatal("expected error for unknown action")
	}

	// dispute / deprecate: status transitions.
	raw, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewDispute})
	if err != nil {
		t.Fatalf("dispute: %v", err)
	}
	if v, _ := parse(raw)["status"].(string); v != "disputed" {
		t.Fatalf("expected status disputed, got %q", v)
	}
	raw, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewDeprecate})
	if err != nil {
		t.Fatalf("deprecate: %v", err)
	}
	if v, _ := parse(raw)["status"].(string); v != "deprecated" {
		t.Fatalf("expected status deprecated, got %q", v)
	}

	// archive: status + archived_at.
	raw, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewArchive})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	m = parse(raw)
	if v, _ := m["status"].(string); v != "archived" {
		t.Fatalf("expected status archived, got %q", v)
	}
	if v, _ := m["archived_at"].(string); v == "" {
		t.Fatal("expected archived_at to be set")
	}

	// deleted fact → not found.
	if err := factWriter.DeleteFactRow(ctx, factID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err = admin.ReviewFactRow(ctx, biz.FactReview{FactID: factID, Action: biz.FactReviewConfirm}); err == nil {
		t.Fatal("expected error reviewing a deleted fact")
	}
}
