package knowledge_test

import (
	"strings"
	"testing"

	"aranea-agents/internal/knowledge"
)

// 父子分块（G-06）：小块召回（子块），父块只作语境回填。

func cjkDoc() string {
	par := "核心机房夜间值守必须双人同时在场，禁止单人进入配电区；任何巡检变更须留痕审批。"
	return strings.Repeat(par, 30)
}

func TestSplitParentChildShape(t *testing.T) {
	t.Parallel()
	chunks := knowledge.SplitParentChild(cjkDoc(), 16, 4, 0)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	var parents, children []knowledge.Chunk
	for _, ch := range chunks {
		if ch.IsParent {
			parents = append(parents, ch)
		} else {
			children = append(children, ch)
		}
	}
	if len(parents) == 0 {
		t.Fatal("expected parent rows for long text")
	}
	if len(children) == 0 {
		t.Fatal("expected child rows")
	}
	// 父块序号稠密、子块序号全文稠密。
	for i, p := range parents {
		if p.ChunkIndex != i || p.ParentIndex != -1 {
			t.Fatalf("parent %d must have ChunkIndex=%d and ParentIndex=-1, got %+v", i, i, p)
		}
	}
	for i, c := range children {
		if c.ChunkIndex != i {
			t.Fatalf("child index must be dense, want %d got %d", i, c.ChunkIndex)
		}
		if c.ParentIndex < 0 || c.ParentIndex >= len(parents) {
			t.Fatalf("child %d has invalid ParentIndex %d", c.ChunkIndex, c.ParentIndex)
		}
		// 子块窗口按未裁剪 token 序列切分；TrimSpace 后重编码在边界至多 ±2 token 漂移。
		if n := knowledge.CountTokens(c.Content); n > 16+2 {
			t.Fatalf("child chunk exceeds token size: %d > 16(+2 trim drift)", n)
		}
		// 子块必属于其声明的父块（子块⊂父块）。
		if !strings.Contains(parents[c.ParentIndex].Content, c.Content) {
			t.Fatalf("child %d not contained in parent %d", c.ChunkIndex, c.ParentIndex)
		}
	}
	// 每个父块至少一个子块。
	childCountByParent := map[int]int{}
	for _, c := range children {
		childCountByParent[c.ParentIndex]++
	}
	for i := range parents {
		if childCountByParent[i] == 0 {
			t.Fatalf("parent %d has no children", i)
		}
	}
}

func TestSplitParentChildEnglish(t *testing.T) {
	t.Parallel()
	text := "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen"
	chunks := knowledge.SplitParentChild(text, 3, 1, 6)
	var parents, children []knowledge.Chunk
	for _, ch := range chunks {
		if ch.IsParent {
			parents = append(parents, ch)
		} else {
			children = append(children, ch)
		}
	}
	// 15 tokens，parentSize=6 → 3 父块（6/6/3）。
	if len(parents) != 3 {
		t.Fatalf("expected 3 parents, got %d", len(parents))
	}
	// 每个子块 ≤3 token（+2 裁剪漂移余量）且落入声明父块。
	for _, c := range children {
		if n := knowledge.CountTokens(c.Content); n > 3+2 {
			t.Fatalf("child exceeds size: %d", n)
		}
		if !strings.Contains(parents[c.ParentIndex].Content, c.Content) {
			t.Fatalf("child %q not in its parent %q", c.Content, parents[c.ParentIndex].Content)
		}
	}
	// 子块不跨父块：相邻子块的 ParentIndex 单调不减。
	prev := -1
	for _, c := range children {
		if c.ParentIndex < prev {
			t.Fatalf("child windows must not cross parents (got %d after %d)", c.ParentIndex, prev)
		}
		prev = c.ParentIndex
	}
}

func TestSplitParentChildShortText(t *testing.T) {
	t.Parallel()
	chunks := knowledge.SplitParentChild("hello world", 512, 64, 0)
	if len(chunks) != 1 {
		t.Fatalf("short text must be one chunk, got %d", len(chunks))
	}
	if chunks[0].IsParent || chunks[0].ParentIndex != -1 {
		t.Fatalf("short text must be a single flat child, got %+v", chunks[0])
	}
}

func TestSplitParentChildDefaultsAndClamp(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("ab cd ", 200) // 400 tokens 量级
	// parentSize<2×size → 归一为 2×size：size=10 → 父窗口 20 tokens。
	chunks := knowledge.SplitParentChild(text, 10, 2, 15)
	var parents []knowledge.Chunk
	for _, ch := range chunks {
		if ch.IsParent {
			parents = append(parents, ch)
		}
	}
	if len(parents) == 0 {
		t.Fatal("expected parents")
	}
	for _, p := range parents[:len(parents)-1] {
		if n := knowledge.CountTokens(p.Content); n != 20 {
			t.Fatalf("non-tail parent must be clamped to 20 tokens, got %d", n)
		}
	}
}

func TestSplitWithStrategyParentChildRoutes(t *testing.T) {
	t.Parallel()
	text := cjkDoc()
	a, err := knowledge.SplitWithStrategy(knowledge.ChunkByParentChild, text, 16, 4)
	if err != nil {
		t.Fatal(err)
	}
	b := knowledge.SplitParentChild(text, 16, 4, 0)
	if len(a) != len(b) {
		t.Fatalf("SplitWithStrategy must route to SplitParentChild: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("chunk %d mismatch: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestSplitWithStrategyNormalizesParentIndex(t *testing.T) {
	t.Parallel()
	text := "# Title\n\n" + strings.Repeat("Paragraph one sentence. ", 30)
	chunks, err := knowledge.SplitWithStrategy(knowledge.ChunkByMarkdown, text, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, ch := range chunks {
		if ch.IsParent || ch.ParentIndex != -1 {
			t.Fatalf("non-parent_child strategy must normalize ParentIndex=-1, got %+v", ch)
		}
	}
}

func TestParseChunkStrategyParentChild(t *testing.T) {
	t.Parallel()
	if s := knowledge.ParseChunkStrategy("parent_child"); s != knowledge.ChunkByParentChild {
		t.Fatalf("parent_child must parse, got %q", s)
	}
	if s := knowledge.ParseChunkStrategy("PARENT_CHILD "); s != knowledge.ChunkByParentChild {
		t.Fatalf("parse must normalize case/space, got %q", s)
	}
}
