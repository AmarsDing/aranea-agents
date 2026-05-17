package knowledge_test

import (
	"testing"

	"aranea-agents/internal/knowledge"
)

func TestChunkerByChar(t *testing.T) {
	c := knowledge.NewChunker(10, 2, knowledge.ChunkByChar)
	text := "abcdefghij klmnopqrst uvwxyz"
	chunks := c.Split(text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	// First chunk must start with text beginning
	if len(chunks[0].Content) == 0 {
		t.Error("first chunk is empty")
	}
	// Check indices
	for i, ch := range chunks {
		if ch.ChunkIndex != i {
			t.Errorf("chunk %d has wrong ChunkIndex %d", i, ch.ChunkIndex)
		}
	}
}

func TestChunkerByToken(t *testing.T) {
	c := knowledge.NewChunker(3, 1, knowledge.ChunkByToken)
	text := "one two three four five six seven"
	chunks := c.Split(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	// Each chunk should be 3 tokens or fewer
	for _, ch := range chunks {
		words := splitWords(ch.Content)
		if len(words) > 3 {
			t.Errorf("chunk %q exceeds token size: %d words", ch.Content, len(words))
		}
	}
}

func TestChunkerShortText(t *testing.T) {
	c := knowledge.NewChunker(512, 64, knowledge.ChunkByChar)
	chunks := c.Split("hello world")
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for short text, got %d", len(chunks))
	}
	if chunks[0].Content != "hello world" {
		t.Errorf("unexpected content: %q", chunks[0].Content)
	}
}

func TestChunkerEmpty(t *testing.T) {
	c := knowledge.NewChunker(512, 64, knowledge.ChunkByChar)
	if chunks := c.Split(""); len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
	if chunks := c.Split("   "); len(chunks) != 0 {
		t.Errorf("expected 0 chunks for whitespace, got %d", len(chunks))
	}
}

func TestNewChunkerDefaults(t *testing.T) {
	c := knowledge.NewChunker(0, -1, "")
	if c.ChunkSize != 512 {
		t.Errorf("expected default ChunkSize=512, got %d", c.ChunkSize)
	}
	if c.ChunkOverlap < 0 {
		t.Errorf("expected non-negative overlap")
	}
	if c.Strategy != knowledge.ChunkByChar {
		t.Errorf("expected default strategy ChunkByChar")
	}
}

func splitWords(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		if r != ' ' && r != '\t' && r != '\n' {
			if start == -1 {
				start = i
			}
		} else if start != -1 {
			out = append(out, s[start:i])
			start = -1
		}
	}
	if start != -1 {
		out = append(out, s[start:])
	}
	return out
}
