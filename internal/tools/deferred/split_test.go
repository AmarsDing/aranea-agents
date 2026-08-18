package deferred

import (
	"sort"
	"testing"
)

func TestSplitCoreResidentTools_SpiritProfile(t *testing.T) {
	enabled := []string{
		"plan_and_execute", "cancel_orchestration", "synthesize_results",
		"get_team_deliverable", "build_orchestration_graph", "memory_search",
		"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel",
		"shell_exec", "datetime",
		"computer_use_observe", "computer_use_screenshot", "computer_use_act",
		"computer_use_launch", "computer_use_session",
		"web_fetch", "read_file", "save_file",
	}
	core, def := SplitCoreResidentTools(enabled, "spirit")

	// Spirit 核心集：编排入口 + 收口 + 基础工具（构图走 deferred）
	assertContainsAll(t, core, []string{
		"plan_and_execute", "synthesize_results", "get_team_deliverable",
		"cancel_orchestration",
		"datetime", "memory_search",
	})
	// 长尾应进 deferred
	assertContainsAll(t, def, []string{
		"web_fetch", "read_file", "save_file",
		"build_orchestration_graph",
		"computer_use_observe", "computer_use_screenshot", "computer_use_act",
		"computer_use_launch", "computer_use_session",
		"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel",
		"shell_exec",
	})
	assertNoOverlap(t, core, def)
	assertUnionEquals(t, core, def, enabled)
}

func TestSplitCoreResidentTools_CodingProfile(t *testing.T) {
	enabled := []string{
		"read_file", "read_multiple_files", "save_file", "list_file",
		"search_file", "search_content", "replace_content", "diff_edit", "patch_file",
		"read_lints", "delete_file",
		"web_fetch", "duckduckgo_search", "gemini_web_fetch", "google_search",
		"arxiv_search", "wikipedia_search",
		"skill_search", "use_skill",
		"await_user_reply", "todo_write", "datetime",
		"shell_exec", "claude_code",
	}
	core, def := SplitCoreResidentTools(enabled, "coding")

	assertContainsAll(t, core, []string{
		"read_file", "save_file", "list_file", "search_file", "search_content",
		"todo_write", "datetime",
		"replace_content", "diff_edit", "patch_file", "read_lints", "delete_file",
		"shell_exec",
	})
	assertContainsAll(t, def, []string{
		"web_fetch", "duckduckgo_search", "gemini_web_fetch", "google_search",
		"arxiv_search", "wikipedia_search",
		"skill_search", "use_skill",
		"await_user_reply",
		"claude_code",
		"read_multiple_files",
	})
	assertNoOverlap(t, core, def)
	assertUnionEquals(t, core, def, enabled)
}

func TestSplitCoreResidentTools_ChatOnlyProfile(t *testing.T) {
	enabled := []string{"datetime", "todo_write", "memory_search"}
	core, def := SplitCoreResidentTools(enabled, "chat_only")

	// chat_only 核心集极小，几乎所有都 deferred
	assertContainsAll(t, core, []string{"datetime"})
	assertContainsAll(t, def, []string{"todo_write", "memory_search"})
	assertNoOverlap(t, core, def)
	assertUnionEquals(t, core, def, enabled)
}

func TestSplitCoreResidentTools_EmptyEnabled(t *testing.T) {
	core, def := SplitCoreResidentTools(nil, "spirit")
	if len(core) != 0 {
		t.Errorf("expected empty core, got %v", core)
	}
	if len(def) != 0 {
		t.Errorf("expected empty deferred, got %v", def)
	}
}

func TestSplitCoreResidentTools_UnknownProfile(t *testing.T) {
	enabled := []string{"read_file", "save_file", "datetime"}
	core, def := SplitCoreResidentTools(enabled, "nonexistent_profile")

	// 未知 profile 回退到默认核心集
	assertContainsAll(t, core, []string{"datetime"})
	assertContainsAll(t, def, []string{"read_file", "save_file"})
	assertNoOverlap(t, core, def)
	assertUnionEquals(t, core, def, enabled)
}

func TestSplitCoreResidentTools_Deterministic(t *testing.T) {
	enabled := []string{"b_tool", "a_tool", "c_tool", "datetime"}
	core1, def1 := SplitCoreResidentTools(enabled, "spirit")
	core2, def2 := SplitCoreResidentTools(enabled, "spirit")

	// 多次调用结果一致（排序后比较）
	sort.Strings(core1)
	sort.Strings(core2)
	sort.Strings(def1)
	sort.Strings(def2)

	if len(core1) != len(core2) {
		t.Fatalf("core length mismatch: %d vs %d", len(core1), len(core2))
	}
	for i := range core1 {
		if core1[i] != core2[i] {
			t.Errorf("core[%d] mismatch: %s vs %s", i, core1[i], core2[i])
		}
	}
	if len(def1) != len(def2) {
		t.Fatalf("deferred length mismatch: %d vs %d", len(def1), len(def2))
	}
	for i := range def1 {
		if def1[i] != def2[i] {
			t.Errorf("deferred[%d] mismatch: %s vs %s", i, def1[i], def2[i])
		}
	}
}

// --- helpers ---

func assertContainsAll(t *testing.T, set []string, expected []string) {
	t.Helper()
	m := make(map[string]bool, len(set))
	for _, s := range set {
		m[s] = true
	}
	for _, e := range expected {
		if !m[e] {
			t.Errorf("expected %q in set %v", e, set)
		}
	}
}

func assertNoOverlap(t *testing.T, a, b []string) {
	t.Helper()
	m := make(map[string]bool, len(a))
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if m[s] {
			t.Errorf("overlap found: %q in both core and deferred", s)
		}
	}
}

func assertUnionEquals(t *testing.T, a, b, original []string) {
	t.Helper()
	union := make(map[string]bool, len(original))
	for _, s := range a {
		union[s] = true
	}
	for _, s := range b {
		union[s] = true
	}
	if len(union) != len(original) {
		t.Errorf("union size %d != original size %d", len(union), len(original))
	}
	for _, s := range original {
		if !union[s] {
			t.Errorf("missing %q from union", s)
		}
	}
}
