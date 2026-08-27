package trpcmem

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// fusedRecallerStub implements biz.MemoryL3Recaller, capturing the fused
// query and returning preset rows/err.
type fusedRecallerStub struct {
	rows  [][]byte
	err   error
	calls []biz.L3FusedRecallQuery
}

func (s *fusedRecallerStub) RecallFacts(_ context.Context, _ biz.L3RecallQuery) ([][]byte, error) {
	return nil, nil
}

func (s *fusedRecallerStub) RecallFactsFused(_ context.Context, q biz.L3FusedRecallQuery) ([][]byte, error) {
	s.calls = append(s.calls, q)
	return s.rows, s.err
}

func factRowJSON(id, statement string, importance float64, updatedAt time.Time) []byte {
	b, _ := json.Marshal(map[string]any{
		"id":         id,
		"scope_type": "agent",
		"scope_id":   "agent-x",
		"user_id":    "user-x",
		"agent_id":   "agent-x",
		"statement":  statement,
		"fact_kind":  "general",
		"importance": importance,
		"status":     "active",
		"created_at": updatedAt.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
		"updated_at": updatedAt.UTC().Format(time.RFC3339Nano),
		"valid_from": updatedAt.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
		"valid_until": "",
		"deleted_at":  "",
	})
	return b
}

func newFusedService(store *bitemporalMockStore, loader AgentRuntimeSettingsLoader, fused biz.MemoryL3Recaller) trpcmemory.Service {
	svc := NewMemoryService(
		store, store, nil, nil, nil, loader, nil, nil, loggateway.NewNoop(),
	)
	if fused != nil {
		WireFusedRecall(svc, fused)
	}
	return svc
}

// TestSearchMemories_FusedPathPreferred verifies the C3 unified recall layer:
// with a fused recaller wired, query search delegates to it (same chain as
// the prompt-injection path) instead of the legacy vector/LIKE path.
func TestSearchMemories_FusedPathPreferred(t *testing.T) {
	store := newBitemporalMockStore()
	// Legacy path would match this row via LIKE — if the result contains it,
	// the search did NOT go through fused recall.
	if _, err := store.upsert(biz.FactUpsert{
		ID: "legacy-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "预算 口径 legacy hit", Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fused := &fusedRecallerStub{rows: [][]byte{
		factRowJSON("fused-1", "Q3 预算口径 80 万", 0.8, now),
	}}
	svc := newFusedService(store, &bitemporalSettingsLoader{}, fused)

	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "预算 口径")
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(fused.calls) != 1 {
		t.Fatalf("expected 1 fused call, got %d", len(fused.calls))
	}
	q := fused.calls[0]
	if q.Query != "预算 口径" {
		t.Errorf("fused query = %q", q.Query)
	}
	if q.Runtime.AgentID != "agent-x" || q.Runtime.UserID != "user-x" {
		t.Errorf("fused runtime = %+v", q.Runtime)
	}
	if q.Limit <= 0 {
		t.Errorf("fused limit = %d, want > 0", q.Limit)
	}
	// Default scopes from the policy fallback (["agent","team"] when the agent
	// has no explicit L3RecallScopesJSON).
	if len(q.Scopes) == 0 {
		t.Error("fused scopes empty — policy fallback not applied")
	}
	if len(entries) != 1 || entries[0].ID != "fused-1" {
		t.Fatalf("entries = %+v, want fused-1 only", entries)
	}
}

// TestSearchMemories_FusedZeroHitsNoLegacyFallback pins the C3 contract: a
// fused zero-hit is the authoritative zero-hit (same as the injection path);
// falling back to LIKE would reintroduce dual-path drift.
func TestSearchMemories_FusedZeroHitsNoLegacyFallback(t *testing.T) {
	store := newBitemporalMockStore()
	if _, err := store.upsert(biz.FactUpsert{
		ID: "legacy-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "预算 口径 legacy hit", Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	fused := &fusedRecallerStub{rows: nil}
	svc := newFusedService(store, &bitemporalSettingsLoader{}, fused)

	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "预算")
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("fused zero-hit must not fall back to legacy LIKE, got %+v", entries)
	}
}

// TestSearchMemories_FusedErrorFallsBackToLegacy verifies graceful
// degradation: when the fused chain errors, the legacy path still serves.
func TestSearchMemories_FusedErrorFallsBackToLegacy(t *testing.T) {
	store := newBitemporalMockStore()
	if _, err := store.upsert(biz.FactUpsert{
		ID: "legacy-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "预算 口径 legacy hit", Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	fused := &fusedRecallerStub{err: errors.New("fused boom")}
	svc := newFusedService(store, &bitemporalSettingsLoader{}, fused)

	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "预算")
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "legacy-row" {
		t.Fatalf("legacy fallback entries = %+v", entries)
	}
}

// TestSearchMemories_IncludeInvalidatedSkipsFused: the archaeology option is
// unsupported by fused recall (active-only) and must use the legacy path.
func TestSearchMemories_IncludeInvalidatedSkipsFused(t *testing.T) {
	store := newBitemporalMockStore()
	raw, err := store.upsert(biz.FactUpsert{
		ID: "old-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "预算 口径 old", Importance: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	if _, err := store.invalidate(row["id"].(string)); err != nil {
		t.Fatal(err)
	}
	fused := &fusedRecallerStub{}
	svc := newFusedService(store, &bitemporalSettingsLoader{}, fused)

	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "预算",
		trpcmemory.WithSearchOptions(trpcmemory.SearchOptions{IncludeInvalidated: true}))
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(fused.calls) != 0 {
		t.Fatalf("IncludeInvalidated must skip fused recall, got %d calls", len(fused.calls))
	}
	if len(entries) != 1 || entries[0].ID != "old-row" {
		t.Fatalf("entries = %+v, want invalidated old-row", entries)
	}
}

// minScoreSettingsLoader enables memory with a high MemoryToolMinScore
// (importance floor post-filter).
type minScoreSettingsLoader struct{ minScore float64 }

func (l *minScoreSettingsLoader) GetAgentRuntimeSettings(_ context.Context, _ string) (*biz.AgentRuntimeSettings, error) {
	return &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, MemoryMinScore: l.minScore}, nil
}

// TestSearchMemories_FusedImportancePostFilter keeps the memory-tool-specific
// MemoryToolMinScore policy as an importance post-filter on fused results.
func TestSearchMemories_FusedImportancePostFilter(t *testing.T) {
	store := newBitemporalMockStore()
	now := time.Now()
	fused := &fusedRecallerStub{rows: [][]byte{
		factRowJSON("high", "重要事实", 0.95, now),
		factRowJSON("low", "次要事实", 0.5, now),
	}}
	svc := newFusedService(store, &minScoreSettingsLoader{minScore: 0.9}, fused)

	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "事实")
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "high" {
		t.Fatalf("importance post-filter entries = %+v", entries)
	}
}

// userScopeSettingsLoader enables memory with L3RecallScopes=["agent","user"].
type userScopeSettingsLoader struct{}

func (l *userScopeSettingsLoader) GetAgentRuntimeSettings(_ context.Context, _ string) (*biz.AgentRuntimeSettings, error) {
	return &biz.AgentRuntimeSettings{
		MemoryEnabled: true, L3Enabled: true,
		L3RecallScopesJSON: `["agent","user"]`,
	}, nil
}

// TestReadMemories_MultiScopeMergeSortedByRecency verifies the C3 key-space
// alignment on the load path: with L3RecallScopes=["agent","user"], facts
// written to the user scope (preference/identity per mapFactKindToScope)
// become visible to memory_load, merged and ordered by updated_at DESC.
func TestReadMemories_MultiScopeMergeSortedByRecency(t *testing.T) {
	store := newBitemporalMockStore()
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	if _, err := store.upsert(biz.FactUpsert{
		ID: "agent-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "agent 域事实", Importance: 0.9, UpdatedAt: old.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.upsert(biz.FactUpsert{
		ID: "user-row", ScopeType: "user", ScopeID: "user-x", UserID: "user-x",
		Statement: "用户偏好事实", Importance: 0.9, UpdatedAt: recent.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	svc := newFusedService(store, &userScopeSettingsLoader{}, nil)

	entries, err := svc.ReadMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, 10)
	if err != nil {
		t.Fatalf("ReadMemories: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (agent+user scopes), got %d: %+v", len(entries), entries)
	}
	if entries[0].ID != "user-row" {
		t.Fatalf("recency order broken: first = %s, want user-row (newer)", entries[0].ID)
	}
}

// TestReadMemories_DefaultScopesUnchanged pins backward compatibility:
// without an explicit user scope in L3RecallScopes, user-scope facts stay
// invisible (the legacy single agent-scope behavior).
func TestReadMemories_DefaultScopesUnchanged(t *testing.T) {
	store := newBitemporalMockStore()
	if _, err := store.upsert(biz.FactUpsert{
		ID: "agent-row", ScopeType: "agent", ScopeID: "agent-x", UserID: "user-x",
		Statement: "agent 域事实", Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.upsert(biz.FactUpsert{
		ID: "user-row", ScopeType: "user", ScopeID: "user-x", UserID: "user-x",
		Statement: "用户偏好事实", Importance: 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	svc := newFusedService(store, &bitemporalSettingsLoader{}, nil)

	entries, err := svc.ReadMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, 10)
	if err != nil {
		t.Fatalf("ReadMemories: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "agent-row" {
		t.Fatalf("default scopes must stay agent-only, got %+v", entries)
	}
}

// TestWireFusedRecall_NilSafety: wiring must tolerate foreign service
// implementations and nil recallers without panicking.
func TestWireFusedRecall_NilSafety(t *testing.T) {
	WireFusedRecall(nil, &fusedRecallerStub{})
	svc, _ := newBitemporalService()
	WireFusedRecall(svc, nil)
	// A service with nil fusedRecall must keep working on the legacy path.
	entries, err := svc.SearchMemories(context.Background(),
		trpcmemory.UserKey{AppName: "agent-x", UserID: "user-x"}, "anything")
	if err != nil {
		t.Fatalf("legacy search with nil fused: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %+v", entries)
	}
}
