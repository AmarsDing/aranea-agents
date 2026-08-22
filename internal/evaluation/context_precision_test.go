package evaluation

import (
	"testing"
)

func TestLexicalContextPrecision(t *testing.T) {
	q := "where is the server rack"
	chunks := []string{
		"the server rack is in room A next to the UPS",
		"today's weather is sunny and warm",
	}
	got := lexicalContextPrecision(q, chunks)
	if got <= 0 || got >= 1 {
		t.Fatalf("expected mixed precision, got %v", got)
	}
}

func TestParseRelevantIndices(t *testing.T) {
	got := parseRelevantIndices("relevant: 0, 2, 2, 9", 3)
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("got %#v", got)
	}
	if n := parseRelevantIndices("relevant:", 3); len(n) != 0 {
		t.Fatalf("empty relevant must be none, got %#v", n)
	}
}

func TestScoreContextPrecisionNoChunks(t *testing.T) {
	if _, ok := scoreContextPrecision(nil, nil, "q", nil); ok {
		t.Fatal("no chunks must be N/A")
	}
}

func TestScoreContextPrecisionEmptyQuery(t *testing.T) {
	if _, ok := scoreContextPrecision(nil, nil, "", []string{"the server rack is in room A"}); ok {
		t.Fatal("empty query must be N/A")
	}
}
