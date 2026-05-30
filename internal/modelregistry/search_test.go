package modelregistry

import (
	"fmt"
	"strings"
	"testing"
)

func TestSearchDirectoryBlocksProviderQuery(t *testing.T) {
	cat := Directory{
		"deepseek": {
			ID:   "deepseek",
			Name: "DeepSeek",
			Models: map[string]Model{
				"deepseek-chat": {ID: "deepseek-chat", Name: "DeepSeek Chat"},
			},
		},
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	blocks, total, truncated := SearchDirectoryBlocks(cat, "deepseek", 10, 0)
	if truncated {
		t.Fatal("unexpected truncated")
	}
	if total != 1 || len(blocks) != 1 {
		t.Fatalf("total=%d len=%d", total, len(blocks))
	}
	if !IsDirectoryJSONBlock(blocks[0]) {
		t.Fatalf("expected JSON block, got %q", blocks[0][:min(80, len(blocks[0]))])
	}
	if !strings.Contains(blocks[0], "deepseek-chat") {
		t.Fatalf("expected model in provider block")
	}
}

func TestSearchDirectoryBlocksModelQuery(t *testing.T) {
	cat := Directory{
		"openai": {
			ID: "openai",
			Models: map[string]Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	blocks, total, _ := SearchDirectoryBlocks(cat, "gpt-4o", 10, 0)
	if total != 1 || len(blocks) != 1 {
		t.Fatalf("total=%d len=%d", total, len(blocks))
	}
	if !strings.Contains(blocks[0], `"model"`) {
		t.Fatalf("expected wrapper block")
	}
}

func TestSearchDirectoryBlocksCap(t *testing.T) {
	cat := Directory{}
	for i := 0; i < MaxCatalogSearchBlocks+5; i++ {
		id := fmt.Sprintf("provider-%d", i)
		cat[id] = Provider{ID: id, Name: id, Models: map[string]Model{}}
	}
	_, total, truncated := SearchDirectoryBlocks(cat, "", MaxCatalogSearchBlocks+10, 0)
	if !truncated {
		t.Fatal("expected truncated")
	}
	if total != MaxCatalogSearchBlocks {
		t.Fatalf("total=%d want %d", total, MaxCatalogSearchBlocks)
	}
}

func TestIsDirectoryJSONBlock(t *testing.T) {
	if IsDirectoryJSONBlock(`{"a":1}`) != true {
		t.Fatal("object should pass")
	}
	if IsDirectoryJSONBlock(`"deepseek-chat": {`) != false {
		t.Fatal("line fragment should fail")
	}
}
