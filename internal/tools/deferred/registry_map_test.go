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

// 包C C6（session-eval-20260827 P11v）：twinops 查询/配置类 15 件恒等映射
// （可 defer），常驻五件（fixB 定式 + S05 高频/HITL 高危）必须保持无映射——
// 无映射即代码层保证任何配置路径（auto-split / 手动 JSON）都无法将其 defer。
func TestRegistryMap_TwinopsDeferrableAndResidentInvariant(t *testing.T) {
	deferrable := []string{
		"twin_alarm_query", "twin_alarm_get", "twin_alarm_ack", "twin_alarm_rule_get",
		"twin_line_status", "twin_line_events", "twin_line_probe",
		"twin_device_get", "twin_device_search", "twin_device_metrics",
		"twin_collector_status", "twin_inspection_query",
		"twin_config_diff", "twin_config_push", "twin_config_rollback",
	}
	names := RegistryNamesForBizKeys(deferrable)
	if len(names) != len(deferrable) {
		t.Errorf("expected %d identity mappings, got %d: %v", len(deferrable), len(names), names)
	}
	assertContainsAll(t, names, deferrable)

	resident := []string{
		"gns3_health_check", "gns3_exec", "gns3_fault_inject", "gns3_fault_clear",
		"twin_remediation_status",
	}
	if got := RegistryNamesForBizKeys(resident); len(got) != 0 {
		t.Errorf("resident five must stay unmapped (never deferrable), got %v", got)
	}
}

// C6 全量 ops 岗（11 岗 read_only/minimal profile、无手动 deferred JSON）走
// auto-split：twin_* 查询/配置件必须全部进 deferred 清单，常驻五件即使不在
// profile 核心集也不得进（无映射 → RegistryNamesForBizKeys 跳过）。
func TestAutoSplit_OpsRoleDefersTwinopsKeepsResident(t *testing.T) {
	enabled := []string{
		"twin_alarm_query", "twin_alarm_get", "twin_alarm_rule_get",
		"twin_line_status", "twin_line_events", "twin_line_probe",
		"twin_device_get", "twin_device_search", "twin_device_metrics",
		"twin_collector_status", "twin_remediation_status", "twin_inspection_query",
		"gns3_health_check", "gns3_exec", "gns3_fault_inject", "gns3_fault_clear",
		"twin_config_diff", "twin_config_push", "twin_config_rollback",
	}
	for _, profile := range []string{"minimal", "read_only"} {
		_, def := SplitCoreResidentTools(enabled, profile)
		names := RegistryNamesForBizKeys(MergeNonCoreMappedDeferred(def, profile))
		assertContainsAll(t, names, []string{
			"twin_alarm_query", "twin_line_status", "twin_device_get", "twin_config_diff",
		})
		assertNotContainsAny(t, names, []string{
			"gns3_health_check", "gns3_exec", "gns3_fault_inject", "gns3_fault_clear",
			"twin_remediation_status",
		})
	}
}
