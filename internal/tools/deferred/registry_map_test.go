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
	keys := []string{"memory_search", "plan_and_execute", "memory_remember", "custom_tool"}
	names := RegistryNamesForBizKeys(keys)
	if len(names) != 0 {
		t.Errorf("expected empty for unmapped keys, got %v", names)
	}
}

func TestSpiritDeferredRegistryIncludesShellComputerUseAndGraph(t *testing.T) {
	enabled := []string{
		"plan_and_execute", "cancel_orchestration", "synthesize_results",
		"get_team_deliverable", "build_orchestration_graph", "memory_search",
		"shell_exec", "datetime",
		"computer_use_observe", "computer_use_screenshot", "computer_use_act",
		"computer_use_launch", "computer_use_session",
		"subagents_spawn",
	}
	_, def := SplitCoreResidentTools(enabled, "spirit")
	names := RegistryNamesForBizKeys(def)
	assertContainsAll(t, names, []string{
		"hostexec",
		"computer_use_observe", "computer_use_screenshot", "computer_use_act",
		"computer_use_launch", "computer_use_session",
		"build_orchestration_graph",
		"synthesize_results", "get_team_deliverable", "cancel_orchestration",
		"subagents_spawn",
	})
	for _, n := range names {
		if n == "plan_and_execute" || n == "datetime" || n == "memory_search" {
			t.Errorf("core tool %q must not be in deferred registry names", n)
		}
	}
}

func TestRegistryNamesForBizKeys_IdentityMappedCustomTools(t *testing.T) {
	keys := []string{
		"computer_use_act", "computer_use_observe", "computer_use_screenshot",
		"computer_use_launch", "computer_use_session",
		"build_orchestration_graph",
		"synthesize_results", "get_team_deliverable", "cancel_orchestration",
		"shell_exec",
		"memory_add", "memory_update", "memory_delete", "memory_load",
		"read_upstream_deliverable",
		"search_messages", "list_agent_sessions", "read_session_history",
	}
	names := RegistryNamesForBizKeys(keys)
	assertContainsAll(t, names, []string{
		"computer_use_act", "computer_use_observe", "computer_use_screenshot",
		"computer_use_launch", "computer_use_session",
		"build_orchestration_graph",
		"synthesize_results", "get_team_deliverable", "cancel_orchestration",
		"hostexec",
		"memory_add", "memory_update", "memory_delete", "memory_load",
		"read_upstream_deliverable",
		"search_messages", "list_agent_sessions", "read_session_history",
	})
}

func TestSpiritDeferredRegistryIncludesMemoryWritesAndUpstream(t *testing.T) {
	enabled := []string{
		"plan_and_execute", "datetime", "memory_search", "shell_exec",
	}
	_, def := SplitCoreResidentTools(enabled, "spirit")
	names := RegistryNamesForBizKeys(MergeNonCoreMappedDeferred(def, "spirit"))
	assertContainsAll(t, names, []string{
		"hostexec",
		"memory_add", "memory_update", "memory_delete", "memory_load",
		"read_upstream_deliverable",
		"working_memory",
		"synthesize_results", "get_team_deliverable", "cancel_orchestration",
		"build_orchestration_graph",
		"search_messages", "list_agent_sessions", "read_session_history",
	})
	assertNotContainsAny(t, names, []string{
		"memory_search", "memory_remember", "plan_and_execute", "datetime",
	})
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
