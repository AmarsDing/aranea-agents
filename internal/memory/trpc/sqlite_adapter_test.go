package trpcmem

import (
	"encoding/json"
	"testing"
	"time"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

func TestTopicsJSON_Empty(t *testing.T) {
	if got := topicsJSON(nil); got != "[]" {
		t.Fatalf("expected [], got %s", got)
	}
	if got := topicsJSON([]string{}); got != "[]" {
		t.Fatalf("expected [], got %s", got)
	}
}

func TestTopicsJSON_Single(t *testing.T) {
	got := topicsJSON([]string{"preference"})
	var out []string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "preference" {
		t.Fatalf("unexpected %v", out)
	}
}

func TestTopicsJSON_TrimsWhitespace(t *testing.T) {
	got := topicsJSON([]string{"  a  ", " ", ""})
	var out []string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "a" {
		t.Fatalf("expected [a], got %v", out)
	}
}

func TestDecodeTagsJSON_Empty(t *testing.T) {
	if got := decodeTagsJSON(""); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := decodeTagsJSON("[]"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := decodeTagsJSON("  "); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDecodeTagsJSON_Valid(t *testing.T) {
	got := decodeTagsJSON(`["a","b"]`)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestDecodeTagsJSON_Invalid(t *testing.T) {
	if got := decodeTagsJSON("not-json"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDecodeMetadataTopics_Empty(t *testing.T) {
	for _, s := range []string{"", "[]", "{}", "  "} {
		if got := decodeMetadataTopics(s); got != nil {
			t.Fatalf("expected nil for %q, got %v", s, got)
		}
	}
}

func TestDecodeMetadataTopics_Array(t *testing.T) {
	got := decodeMetadataTopics(`["x","y"]`)
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestDecodeMetadataTopics_ObjectWithTopics(t *testing.T) {
	got := decodeMetadataTopics(`{"topics":["z"]}`)
	if len(got) != 1 || got[0] != "z" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestDecodeMetadataTopics_ObjectNoTopics(t *testing.T) {
	if got := decodeMetadataTopics(`{"other":1}`); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDecodeMetadataTopics_Invalid(t *testing.T) {
	if got := decodeMetadataTopics("not-json"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestParseRFC3339_Valid(t *testing.T) {
	fallback := time.Now()
	s := "2026-05-31T12:00:00Z"
	got := parseRFC3339(s, fallback)
	if got.Year() != 2026 || got.Month() != 5 || got.Day() != 31 {
		t.Fatalf("unexpected %v", got)
	}
}

func TestParseRFC3339_Nano(t *testing.T) {
	fallback := time.Now()
	s := "2026-05-31T12:00:00.123456789Z"
	got := parseRFC3339(s, fallback)
	if got.Year() != 2026 {
		t.Fatalf("unexpected %v", got)
	}
}

func TestParseRFC3339_Invalid(t *testing.T) {
	fallback := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	got := parseRFC3339("not-a-date", fallback)
	if !got.Equal(fallback) {
		t.Fatalf("expected fallback, got %v", got)
	}
}

func TestFactRowToEntry_Valid(t *testing.T) {
	raw := `{"id":"f1","statement":"likes dark mode","tags_json":"[\"preference\"]","importance":0.8,"metadata_json":"{}","created_at":"2026-05-31T00:00:00Z","updated_at":"2026-05-31T01:00:00Z"}`
	entry, err := factRowToEntry([]byte(raw), trpcmemory.UserKey{AppName: "app1", UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "f1" {
		t.Fatalf("id=%q", entry.ID)
	}
	if entry.Memory.Memory != "likes dark mode" {
		t.Fatalf("memory=%q", entry.Memory.Memory)
	}
	if len(entry.Memory.Topics) != 1 || entry.Memory.Topics[0] != "preference" {
		t.Fatalf("topics=%v", entry.Memory.Topics)
	}
}

func TestFactRowToEntry_EmptyStatement(t *testing.T) {
	raw := `{"id":"f2","statement":"  ","tags_json":"[]","importance":0,"metadata_json":"{}","created_at":"","updated_at":""}`
	_, err := factRowToEntry([]byte(raw), trpcmemory.UserKey{})
	if err == nil {
		t.Fatal("expected error for empty statement")
	}
}

func TestFactRowToEntry_InvalidJSON(t *testing.T) {
	_, err := factRowToEntry([]byte("{bad"), trpcmemory.UserKey{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFactRowToEntry_MetadataFallbackTopics(t *testing.T) {
	raw := `{"id":"f3","statement":"hello","tags_json":"[]","importance":0,"metadata_json":"{\"topics\":[\"x\"]}","created_at":"","updated_at":""}`
	entry, err := factRowToEntry([]byte(raw), trpcmemory.UserKey{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Memory.Topics) != 1 || entry.Memory.Topics[0] != "x" {
		t.Fatalf("topics=%v", entry.Memory.Topics)
	}
}

func TestTrpcFactUpsert(t *testing.T) {
	uk := trpcmemory.UserKey{AppName: "app1", UserID: "u1"}
	up := trpcFactUpsert(uk, "id1", "  fact text  ", []string{"a", "b"}, "episodic", "2026-01-01", "2026-01-02")
	if up.ID != "id1" {
		t.Fatalf("id=%q", up.ID)
	}
	if up.Statement != "fact text" {
		t.Fatalf("statement=%q", up.Statement)
	}
	if up.FactKind != "episodic" {
		t.Fatalf("factKind=%q", up.FactKind)
	}
	if up.ScopeID != "app1" {
		t.Fatalf("scopeID=%q", up.ScopeID)
	}
	if up.UserID != "u1" {
		t.Fatalf("userID=%q", up.UserID)
	}
}
