package biz

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D7-F2: RecipeKeysReusable fail-closed 契约测试。
//   - want == ""（当前任务不受剧本约束）→ 放行（旧条目继续服务）；
//   - want != "" 且 have == ""（旧条目/无 provenance 条目）→ 一次性降级：
//     keys 不复用（Topology/Specialties 仍可复用），不删数据；
//   - 仅指纹完全一致才复用 keys。
// ---------------------------------------------------------------------------

func TestRecipeKeysReusable(t *testing.T) {
	cases := []struct {
		name string
		have string
		want string
		ok   bool
	}{
		{"both empty → permissive", "", "", true},
		{"unconstrained task reuses constrained entry", "a", "", true},
		{"constrained task + legacy entry → one-time downgrade", "", "a", false},
		{"same fingerprint → reuse", "a", "a", true},
		{"different fingerprint → closed", "a", "b", false},
		{"whitespace tolerated", "  a  ", " a ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RecipeKeysReusable(tc.have, tc.want); got != tc.ok {
				t.Fatalf("RecipeKeysReusable(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
			}
		})
	}
}

func TestRecipeKeysReusable_FailClosedDocsConstant(t *testing.T) {
	// 守护语义边界：受剧本约束任务绝不复用无 provenance 配方 keys。
	if RecipeKeysReusable(strings.TrimSpace(""), "fp-playbook-x") {
		t.Fatal("legacy entry (empty fingerprint) must not serve playbook-bound tasks")
	}
	if !RecipeKeysReusable("fp-playbook-x", "") {
		t.Fatal("unconstrained tasks must keep reusing any entry keys")
	}
}
