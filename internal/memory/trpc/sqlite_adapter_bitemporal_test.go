package trpcmem

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// bitemporalMockStore is an in-memory mock that simulates the bi-temporal
// filtering behavior of the real data layer (memory_shim_l3.go).
// ListFactRowsForUser returns only valid facts (valid_until == ""),
// while ListFactRowsForUserAll returns all facts including invalidated ones.
type bitemporalMockStore struct {
	mu    sync.Mutex
	facts map[string]map[string]any // factID -> row
}

func newBitemporalMockStore() *bitemporalMockStore {
	return &bitemporalMockStore{facts: make(map[string]map[string]any)}
}

func (s *bitemporalMockStore) rowToJSON(row map[string]any) []byte {
	b, _ := json.Marshal(row)
	return b
}

func (s *bitemporalMockStore) upsert(in biz.FactUpsert) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := in.ID
	if id == "" {
		id = "fact-" + in.Statement
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := in.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := in.UpdatedAt
	if updatedAt == "" {
		updatedAt = now
	}
	validFrom := in.ValidFrom
	if validFrom == "" {
		validFrom = createdAt
	}
	row := map[string]any{
		"id":            id,
		"scope_type":    in.ScopeType,
		"scope_id":      in.ScopeID,
		"user_id":       in.UserID,
		"agent_id":      in.AgentID,
		"statement":     in.Statement,
		"fact_kind":     in.FactKind,
		"tags_json":     in.TagsJSON,
		"importance":    in.Importance,
		"status":        "active",
		"metadata_json": in.MetadataJSON,
		"created_at":    createdAt,
		"updated_at":    updatedAt,
		"valid_from":    validFrom,
		"valid_until":   in.ValidUntil,
		"deleted_at":    "",
	}
	s.facts[id] = row
	return s.rowToJSON(row), nil
}

func (s *bitemporalMockStore) invalidate(factID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.facts[factID]
	if !ok {
		return nil, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	row["valid_until"] = now
	row["updated_at"] = now
	return s.rowToJSON(row), nil
}

func (s *bitemporalMockStore) listForUser(scopeType, scopeID, userID, keyword string, includeInvalidated bool) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, row := range s.facts {
		if scopeType != "" && row["scope_type"] != scopeType {
			continue
		}
		if scopeID != "" && row["scope_id"] != scopeID {
			continue
		}
		if userID != "" && row["user_id"] != userID {
			continue
		}
		if row["status"] != "active" || row["deleted_at"] != "" {
			continue
		}
		if !includeInvalidated {
			if vu, _ := row["valid_until"].(string); vu != "" {
				continue
			}
		}
		if keyword != "" {
			stmt, _ := row["statement"].(string)
			if !strings.Contains(strings.ToLower(stmt), strings.ToLower(keyword)) {
				continue
			}
		}
		out = append(out, s.rowToJSON(row))
	}
	return out
}

// --- L3FactReader implementation ---

func (s *bitemporalMockStore) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	rows := s.listForUser(scopeType, scopeID, "", keyword, true)
	return rows, int32(len(rows)), int32(len(rows)), 0, nil
}

func (s *bitemporalMockStore) CountFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string) (int32, error) {
	return int32(len(s.listForUser(scopeType, scopeID, "", keyword, true))), nil
}

func (s *bitemporalMockStore) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return s.listForUser(scopeType, scopeID, userID, keyword, false), nil
}

func (s *bitemporalMockStore) ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return s.listForUser(scopeType, scopeID, userID, keyword, true), nil
}

func (s *bitemporalMockStore) GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, id := range factIDs {
		row, ok := s.facts[id]
		if !ok {
			continue
		}
		if row["status"] != "active" || row["deleted_at"] != "" {
			continue
		}
		if vu, _ := row["valid_until"].(string); vu != "" {
			continue
		}
		out = append(out, s.rowToJSON(row))
	}
	return out, nil
}

func (s *bitemporalMockStore) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return s.listForUser(scopeType, scopeID, userID, query, false), nil
}

// --- L3FactWriter implementation ---

func (s *bitemporalMockStore) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	return s.upsert(in)
}

func (s *bitemporalMockStore) DeleteFactRow(ctx context.Context, factID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row, ok := s.facts[factID]; ok {
		row["status"] = "deleted"
		row["deleted_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return nil
}

func (s *bitemporalMockStore) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range factIDs {
		if row, ok := s.facts[id]; ok {
			row["status"] = "deleted"
			row["deleted_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			n++
		}
	}
	return n, nil
}

func (s *bitemporalMockStore) ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, row := range s.facts {
		if row["scope_type"] == scopeType && row["scope_id"] == scopeID && row["user_id"] == userID {
			if row["status"] == "active" {
				row["status"] = "deleted"
				row["deleted_at"] = time.Now().UTC().Format(time.RFC3339Nano)
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (s *bitemporalMockStore) InvalidateFact(ctx context.Context, factID string) ([]byte, error) {
	return s.invalidate(factID)
}

// InvalidateAndUpsertFactTx atomically invalidates the old fact and upserts
// the new fact. In this mock, atomicity is simulated by holding the mutex
// across both operations.
func (s *bitemporalMockStore) InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if oldFactID != "" {
		if row, ok := s.facts[oldFactID]; ok {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			row["valid_until"] = now
			row["updated_at"] = now
		}
	}
	// Inline upsert (cannot call s.upsert because it re-locks the mutex).
	id := in.ID
	if id == "" {
		id = "fact-" + in.Statement
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt := in.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := in.UpdatedAt
	if updatedAt == "" {
		updatedAt = now
	}
	validFrom := in.ValidFrom
	if validFrom == "" {
		validFrom = createdAt
	}
	row := map[string]any{
		"id":            id,
		"scope_type":    in.ScopeType,
		"scope_id":      in.ScopeID,
		"user_id":       in.UserID,
		"agent_id":      in.AgentID,
		"statement":     in.Statement,
		"fact_kind":     in.FactKind,
		"tags_json":     in.TagsJSON,
		"importance":    in.Importance,
		"status":        "active",
		"metadata_json": in.MetadataJSON,
		"created_at":    createdAt,
		"updated_at":    updatedAt,
		"valid_from":    validFrom,
		"valid_until":   in.ValidUntil,
		"deleted_at":    "",
	}
	s.facts[id] = row
	return s.rowToJSON(row), nil
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader = (*bitemporalMockStore)(nil)
	_ biz.L3FactWriter = (*bitemporalMockStore)(nil)
)

// bitemporalSettingsLoader returns enabled settings so SearchMemories/ReadMemories proceed.
type bitemporalSettingsLoader struct{}

func (l *bitemporalSettingsLoader) GetAgentRuntimeSettings(_ context.Context, _ string) (*biz.AgentRuntimeSettings, error) {
	return &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true}, nil
}

func newBitemporalService() (trpcmemory.Service, *bitemporalMockStore) {
	store := newBitemporalMockStore()
	svc := NewMemoryService(
		store,                       // factReader
		store,                       // factWriter
		nil,                         // indexSync
		nil,                         // autoMemoryQueue
		nil,                         // vector
		&bitemporalSettingsLoader{}, // settingsLoader
		nil,                         // consistency
		nil,                         // linkEvolver
		loggateway.NewNoop(),
	)
	return svc, store
}

// TestSQLiteAdapter_AddMemory_BiTemporalConflict verifies that when UpdateMemory
// detects content conflict, the old memory is invalidated (ValidUntil set)
// rather than deleted, and a new memory is created.
func TestSQLiteAdapter_AddMemory_BiTemporalConflict(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-btc", UserID: "user-btc"}

	// Add initial memory.
	if err := svc.AddMemory(ctx, uk, "I live in London", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Verify the memory exists and is valid.
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatalf("ReadMemories: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	oldID := entries[0].ID
	if entries[0].Memory.ValidUntil != nil {
		t.Fatal("expected nil ValidUntil for new memory")
	}

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

	// Verify the old fact is invalidated (valid_until set) but still in store.
	store.mu.Lock()
	oldRow, oldExists := store.facts[oldID]
	store.mu.Unlock()
	if !oldExists {
		t.Fatal("expected old fact to still exist in store after invalidation")
	}
	if vu, _ := oldRow["valid_until"].(string); vu == "" {
		t.Fatal("expected old fact to have valid_until set after conflict invalidation")
	}

	// Verify only the new (valid) fact appears in ReadMemories.
	entries, err = svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatalf("ReadMemories after update: %v", err)
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
}

// TestSQLiteAdapter_SearchMemories_FilterInvalidated verifies that
// SearchMemories filters out invalidated memories by default.
func TestSQLiteAdapter_SearchMemories_FilterInvalidated(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-sf", UserID: "user-sf"}

	// Add two memories.
	if err := svc.AddMemory(ctx, uk, "I like coffee", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "I work as engineer", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Invalidate one directly via the store.
	store.mu.Lock()
	for _, row := range store.facts {
		if stmt, _ := row["statement"].(string); stmt == "I like coffee" {
			row["valid_until"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	store.mu.Unlock()

	// SearchMemories should only return the valid memory.
	entries, err := svc.SearchMemories(ctx, uk, "engineer")
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	for _, e := range entries {
		if e.Memory.Memory == "I like coffee" {
			t.Fatal("expected invalidated memory to be filtered out by default")
		}
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one valid result")
	}
}

// TestSQLiteAdapter_SearchMemories_IncludeInvalidated verifies that
// SearchMemories with IncludeInvalidated=true returns both valid and
// invalidated memories.
func TestSQLiteAdapter_SearchMemories_IncludeInvalidated(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-si", UserID: "user-si"}

	// Add two memories.
	if err := svc.AddMemory(ctx, uk, "I like coffee", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "I like tea", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Invalidate one directly via the store.
	store.mu.Lock()
	for _, row := range store.facts {
		if stmt, _ := row["statement"].(string); stmt == "I like coffee" {
			row["valid_until"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	store.mu.Unlock()

	// SearchMemories with IncludeInvalidated=true should return both.
	entries, err := svc.SearchMemories(ctx, uk, "", trpcmemory.WithSearchOptions(
		trpcmemory.SearchOptions{IncludeInvalidated: true},
	))
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}

	var foundCoffee, foundTea bool
	for _, e := range entries {
		if e.Memory.Memory == "I like coffee" {
			foundCoffee = true
		}
		if e.Memory.Memory == "I like tea" {
			foundTea = true
		}
	}
	if !foundCoffee {
		t.Fatal("expected invalidated memory 'I like coffee' to be included with IncludeInvalidated=true")
	}
	if !foundTea {
		t.Fatal("expected valid memory 'I like tea' to be included")
	}
}

// TestSQLiteAdapter_ReadMemories_FilterInvalidated verifies that
// ReadMemories filters out invalidated memories by default.
func TestSQLiteAdapter_ReadMemories_FilterInvalidated(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-rf", UserID: "user-rf"}

	// Add two memories.
	if err := svc.AddMemory(ctx, uk, "I speak English", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "I speak French", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Invalidate one directly via the store.
	store.mu.Lock()
	for _, row := range store.facts {
		if stmt, _ := row["statement"].(string); stmt == "I speak English" {
			row["valid_until"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	store.mu.Unlock()

	// ReadMemories should only return the valid memory.
	entries, err := svc.ReadMemories(ctx, uk, 10)
	if err != nil {
		t.Fatalf("ReadMemories: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].Memory.Memory == "I speak English" {
		t.Fatal("expected invalidated memory to be filtered out")
	}
	if entries[0].Memory.Memory != "I speak French" {
		t.Fatalf("expected 'I speak French', got %q", entries[0].Memory.Memory)
	}
}

// TestSQLiteAdapter_AddMemory_SetsValidFrom verifies that AddMemory
// sets ValidFrom on new memories.
func TestSQLiteAdapter_AddMemory_SetsValidFrom(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-vf", UserID: "user-vf"}

	before := time.Now().UTC()
	if err := svc.AddMemory(ctx, uk, "I like pizza", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	store.mu.Lock()
	var row map[string]any
	for _, r := range store.facts {
		row = r
		break
	}
	store.mu.Unlock()

	if row == nil {
		t.Fatal("expected fact to be stored")
	}
	vf, _ := row["valid_from"].(string)
	if vf == "" {
		t.Fatal("expected valid_from to be set")
	}
	vfTime, err := time.Parse(time.RFC3339Nano, vf)
	if err != nil {
		t.Fatalf("failed to parse valid_from: %v", err)
	}
	if vfTime.Before(before) {
		t.Fatal("expected valid_from to be >= time before AddMemory")
	}
	// valid_until should be empty (currently valid).
	if vu, _ := row["valid_until"].(string); vu != "" {
		t.Fatal("expected valid_until to be empty for new memory")
	}
}
