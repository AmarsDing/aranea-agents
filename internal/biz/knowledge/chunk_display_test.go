package knowledge

import "testing"

// G-06：DisplayContent 单一展示规则——父块语境非空取其值，否则子块原文。
func TestChunkDisplayContent(t *testing.T) {
	t.Parallel()
	parented := Chunk{Content: "子块", ParentContent: "父块语境"}
	if got := parented.DisplayContent(); got != "父块语境" {
		t.Fatalf("parent content must win when non-empty, got %q", got)
	}
	childOnly := Chunk{Content: "子块"}
	if got := childOnly.DisplayContent(); got != "子块" {
		t.Fatalf("must fall back to child content, got %q", got)
	}
	emptyParent := Chunk{Content: "子块", ParentContent: ""}
	if got := emptyParent.DisplayContent(); got != "子块" {
		t.Fatalf("empty parent content must not shadow child, got %q", got)
	}
}
