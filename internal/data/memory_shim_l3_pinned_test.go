package data_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/data"
)

// insertPinnedFact inserts a memory_facts row for pinned-preference tests.
func insertPinnedFact(t *testing.T, d *data.Data, id, scopeType, scopeID, userID, agentID, kind, status string, importance float64, updatedAt time.Time) {
	t.Helper()
	_, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), `
INSERT INTO memory_facts (id, scope_type, scope_id, user_id, agent_id, statement, statement_normalized, fingerprint, fact_kind, importance, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9, $10, $11, $11)`,
		id, scopeType, scopeID, userID, agentID, "stmt-"+id, "fp-"+id, kind, importance, status,
		updatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
}

func pinnedFactIDs(t *testing.T, rows [][]byte) []string {
	t.Helper()
	var ids []string
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("bad row json: %v", err)
		}
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func TestListActivePreferenceFacts_FiltersAndOrders(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertPinnedFact(t, d, "f-pref-user", "user", "user-1", "user-1", "agent-1", "preference", "active", 0.90, now.Add(-time.Hour))
	insertPinnedFact(t, d, "f-con-agent", "agent", "agent-1", "user-1", "agent-1", "constraint", "active", 0.80, now)
	insertPinnedFact(t, d, "f-pref-agent-hi", "agent", "agent-1", "user-1", "agent-1", "preference", "active", 0.95, now.Add(-2*time.Hour))
	// Excluded rows:
	insertPinnedFact(t, d, "f-superseded", "user", "user-1", "user-1", "agent-1", "preference", "superseded", 0.99, now)
	insertPinnedFact(t, d, "f-plain-fact", "user", "user-1", "user-1", "agent-1", "fact", "active", 0.99, now)
	insertPinnedFact(t, d, "f-other-user", "user", "user-2", "user-2", "agent-1", "preference", "active", 0.99, now)

	lister := data.NewMemoryPreferenceLister(d)
	if lister == nil {
		t.Fatal("lister should not be nil for valid Data")
	}
	rows, err := lister.ListActivePreferenceFacts(ctx, "agent-1", "user-1", []string{"preference", "constraint"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := pinnedFactIDs(t, rows)
	want := []string{"f-pref-agent-hi", "f-pref-user", "f-con-agent"} // importance DESC
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %v, want %v", i, got, want)
		}
	}
}

func TestListActivePreferenceFacts_RespectsLimit(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	now := time.Now().UTC()
	insertPinnedFact(t, d, "p1", "agent", "agent-1", "user-1", "agent-1", "preference", "active", 0.95, now)
	insertPinnedFact(t, d, "p2", "agent", "agent-1", "user-1", "agent-1", "preference", "active", 0.85, now)

	lister := data.NewMemoryPreferenceLister(d)
	rows, err := lister.ListActivePreferenceFacts(ctx, "agent-1", "user-1", []string{"preference"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := pinnedFactIDs(t, rows); len(got) != 1 || got[0] != "p1" {
		t.Fatalf("limit=1 should return only p1, got %v", got)
	}
}

func TestListActivePreferenceFacts_EmptyKindsReturnsNil(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	lister := data.NewMemoryPreferenceLister(d)
	rows, err := lister.ListActivePreferenceFacts(context.Background(), "agent-1", "user-1", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty kinds must return no rows, got %d", len(rows))
	}
}
