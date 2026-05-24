package pgvector_test

import (
	"testing"

	"aranea-agents/internal/data/pgvector"
)

func TestParseFactVectorContent(t *testing.T) {
	id, stmt := pgvector.ParseFactVectorContent("fact_id:abc-123\nI prefer tea")
	if id != "abc-123" || stmt != "I prefer tea" {
		t.Fatalf("got id=%q stmt=%q", id, stmt)
	}
	id, stmt = pgvector.ParseFactVectorContent("plain text")
	if id != "" || stmt != "plain text" {
		t.Fatalf("legacy content: id=%q stmt=%q", id, stmt)
	}
}
