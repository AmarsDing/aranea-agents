package pgvector

import (
	"testing"
)

func TestTableNameForDimension(t *testing.T) {
	if got := TableNameForDimension(1536); got != "agent_memory_1536" {
		t.Fatalf("got %q", got)
	}
	if got := TableNameForDimension(768); got != "agent_memory_768" {
		t.Fatalf("got %q", got)
	}
}

func TestNewStore_DefaultDim(t *testing.T) {
	s := NewStore(nil, 0)
	if s.Dim() != 1536 {
		t.Fatalf("dim=%d", s.Dim())
	}
	if s.Table() != "agent_memory_1536" {
		t.Fatalf("table=%q", s.Table())
	}
}

func TestNewStore_CustomDim(t *testing.T) {
	s := NewStore(nil, 768)
	if s.Dim() != 768 {
		t.Fatalf("dim=%d", s.Dim())
	}
	if s.Table() != "agent_memory_768" {
		t.Fatalf("table=%q", s.Table())
	}
}

func TestStore_Dim_NilReceiver(t *testing.T) {
	var s *Store
	if s.Dim() != 1536 {
		t.Fatalf("nil Store.Dim()=%d", s.Dim())
	}
}

func TestStore_Table_NilReceiver(t *testing.T) {
	var s *Store
	if s.Table() != "agent_memory_1536" {
		t.Fatalf("nil Store.Table()=%q", s.Table())
	}
}

func TestStore_dbOrErr_NilReceiver(t *testing.T) {
	var s *Store
	_, err := s.dbOrErr()
	if err != ErrDBUnavailable {
		t.Fatalf("expected ErrDBUnavailable, got %v", err)
	}
}

func TestStore_dbOrErr_NilDB(t *testing.T) {
	s := &Store{}
	_, err := s.dbOrErr()
	if err != ErrDBUnavailable {
		t.Fatalf("expected ErrDBUnavailable, got %v", err)
	}
}

func TestStore_expectDim(t *testing.T) {
	s := NewStore(nil, 3)
	if err := s.expectDim([]float32{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.expectDim([]float32{1, 2}); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestFactVectorContentPrefix(t *testing.T) {
	got := FactVectorContentPrefix("abc-123")
	if got != "fact_id:abc-123\n" {
		t.Fatalf("got %q", got)
	}
}

func TestFactVectorContentPrefix_TrimsSpaces(t *testing.T) {
	got := FactVectorContentPrefix(" abc-123 ")
	if got != "fact_id:abc-123\n" {
		t.Fatalf("got %q", got)
	}
}

func TestParseFactVectorContent_WithNewline(t *testing.T) {
	id, stmt := ParseFactVectorContent("fact_id:abc-123\nI prefer tea")
	if id != "abc-123" || stmt != "I prefer tea" {
		t.Fatalf("id=%q stmt=%q", id, stmt)
	}
}

func TestParseFactVectorContent_NoNewline(t *testing.T) {
	id, stmt := ParseFactVectorContent("fact_id:abc-123")
	if id != "abc-123" || stmt != "" {
		t.Fatalf("id=%q stmt=%q", id, stmt)
	}
}

func TestParseFactVectorContent_LegacyContent(t *testing.T) {
	id, stmt := ParseFactVectorContent("plain text without prefix")
	if id != "" || stmt != "plain text without prefix" {
		t.Fatalf("id=%q stmt=%q", id, stmt)
	}
}

func TestParseFactVectorContent_WhitespaceTrimmed(t *testing.T) {
	id, stmt := ParseFactVectorContent("  fact_id: abc \n some statement  ")
	if id != "abc" || stmt != "some statement" {
		t.Fatalf("id=%q stmt=%q", id, stmt)
	}
}

func TestFactVectorContent_RoundTrip(t *testing.T) {
	content := factVectorContent("id-1", "hello world")
	id, stmt := ParseFactVectorContent(content)
	if id != "id-1" || stmt != "hello world" {
		t.Fatalf("round-trip failed: id=%q stmt=%q", id, stmt)
	}
}
