package knowledge_test

import (
	"strings"
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
	// 真 BPE 计数（G-06）：窗口按未裁剪 token 序列切分，TrimSpace 重编码边界至多
	// ±2 漂移；ParentIndex 归一 -1。
	for _, ch := range chunks {
		if n := knowledge.CountTokens(ch.Content); n > 3+2 {
			t.Errorf("chunk %q exceeds token size: %d tokens", ch.Content, n)
		}
		if ch.ParentIndex != -1 {
			t.Errorf("flat token chunk must have ParentIndex=-1, got %d", ch.ParentIndex)
		}
	}
}

// G-06 回归锚点：中文长无空格段在 token 策略下必须切窗（旧空格分词会塌缩成 1 块）。
func TestChunkerByTokenCJKLongRun(t *testing.T) {
	c := knowledge.NewChunker(16, 4, knowledge.ChunkByToken)
	text := strings.Repeat("中文段落连续书写没有任何空格间隔", 8)
	chunks := c.Split(text)
	if len(chunks) < 2 {
		t.Fatalf("CJK long run must be windowed by true tokens, got %d chunk", len(chunks))
	}
	for _, ch := range chunks {
		if n := knowledge.CountTokens(ch.Content); n > 16+2 {
			t.Errorf("chunk exceeds token size: %d tokens", n)
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
