package deferred

import (
	"testing"
)

func TestRegistryNamesForBizKeys_Basic(t *testing.T) {
	keys := []string{"read_file", "save_file", "web_fetch", "datetime", "memory_search"}
	names := RegistryNamesForBizKeys(keys)

	// read_file + save_file → file (去重)
	// web_fetch → httpfetch
	// datetime → datetime
	// memory_search → 无映射（CustomTool），跳过
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d: %v", len(names), names)
	}
	assertContainsAll(t, names, []string{"file", "httpfetch", "datetime"})
}

func TestRegistryNamesForBizKeys_Dedup(t *testing.T) {
	keys := []string{"read_file", "save_file", "list_file", "search_file"}
	names := RegistryNamesForBizKeys(keys)
	if len(names) != 1 {
		t.Errorf("expected 1 name (file), got %d: %v", len(names), names)
	}
	if names[0] != "file" {
		t.Errorf("expected 'file', got %q", names[0])
	}
}

func TestRegistryNamesForBizKeys_Empty(t *testing.T) {
	names := RegistryNamesForBizKeys(nil)
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestRegistryNamesForBizKeys_Sorted(t *testing.T) {
	keys := []string{"web_fetch", "datetime", "read_file"}
	names := RegistryNamesForBizKeys(keys)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "datetime" || names[1] != "file" || names[2] != "httpfetch" {
		t.Errorf("not sorted: %v", names)
	}
}

func TestRegistryNamesForBizKeys_SkipsUnmapped(t *testing.T) {
	keys := []string{"memory_search", "plan_and_execute", "synthesize_results", "custom_tool"}
	names := RegistryNamesForBizKeys(keys)
	if len(names) != 0 {
		t.Errorf("expected empty for unmapped keys, got %v", names)
	}
}

func TestBizKeysForRegistryName_File(t *testing.T) {
	keys := BizKeysForRegistryName("file")
	if len(keys) != 9 {
		t.Errorf("expected 9 file keys, got %d: %v", len(keys), keys)
	}
	assertContainsAll(t, keys, []string{
		"read_file", "save_file", "list_file", "search_file", "search_content",
		"replace_content", "diff_edit", "patch_file", "read_multiple_files",
	})
}

func TestBizKeysForRegistryName_Unknown(t *testing.T) {
	keys := BizKeysForRegistryName("nonexistent")
	if len(keys) != 0 {
		t.Errorf("expected empty for unknown registry name, got %v", keys)
	}
}
