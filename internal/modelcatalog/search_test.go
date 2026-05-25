package modelcatalog

import (
	"fmt"
	"strings"
	"testing"
)

func TestSearchCatalogBlocksProviderQuery(t *testing.T) {
	cat := Catalog{
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
	blocks, total, truncated := SearchCatalogBlocks(cat, "deepseek", 10, 0)
	if truncated {
		t.Fatal("unexpected truncated")
	}
	if total != 1 || len(blocks) != 1 {
		t.Fatalf("total=%d len=%d", total, len(blocks))
	}
	if !IsCatalogJSONBlock(blocks[0]) {
		t.Fatalf("expected JSON block, got %q", blocks[0][:min(80, len(blocks[0]))])
	}
	if !strings.Contains(blocks[0], "deepseek-chat") {
		t.Fatalf("expected model in provider block")
	}
}

func TestSearchCatalogBlocksModelQuery(t *testing.T) {
	cat := Catalog{
		"openai": {
			ID: "openai",
			Models: map[string]Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	blocks, total, _ := SearchCatalogBlocks(cat, "gpt-4o", 10, 0)
	if total != 1 || len(blocks) != 1 {
		t.Fatalf("total=%d len=%d", total, len(blocks))
	}
	if !strings.Contains(blocks[0], `"model"`) {
		t.Fatalf("expected wrapper block")
	}
}

func TestSearchCatalogBlocksCap(t *testing.T) {
	cat := Catalog{}
	for i := 0; i < MaxCatalogSearchBlocks+5; i++ {
		id := fmt.Sprintf("provider-%d", i)
		cat[id] = Provider{ID: id, Name: id, Models: map[string]Model{}}
	}
	_, total, truncated := SearchCatalogBlocks(cat, "", MaxCatalogSearchBlocks+10, 0)
	if !truncated {
		t.Fatal("expected truncated")
	}
	if total != MaxCatalogSearchBlocks {
		t.Fatalf("total=%d want %d", total, MaxCatalogSearchBlocks)
	}
}

func TestIsCatalogJSONBlock(t *testing.T) {
	if IsCatalogJSONBlock(`{"a":1}`) != true {
		t.Fatal("object should pass")
	}
	if IsCatalogJSONBlock(`"deepseek-chat": {`) != false {
		t.Fatal("line fragment should fail")
	}
}