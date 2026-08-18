package biz

import "testing"

func TestCanonicalizeFactKind(t *testing.T) {
	cases := []struct {
		kind, stmt, want string
	}{
		{"preference", "我的工号是 DIAG-20260818-A7", "user_identity"},
		{"domain_knowledge", "工号 DIAG-1，负责杭州滨江机房", "user_identity"},
		{"preference", "喜欢简洁的回答", "preference"},
		{"user_preference", "部署必须走灰度", "preference"},
		{"identity", "用户叫张三", "user_identity"},
		{"profile", "我叫李四", "user_identity"},
		{"profile", "喜欢咖啡", "preference"},
		{"constraint", "不要再使用 emoji", "constraint"},
		{"", "杂项", "general"},
	}
	for _, tc := range cases {
		if got := CanonicalizeFactKind(tc.kind, tc.stmt); got != tc.want {
			t.Errorf("CanonicalizeFactKind(%q, %q) = %q, want %q", tc.kind, tc.stmt, got, tc.want)
		}
	}
}

func TestUserScopedFactKind(t *testing.T) {
	if !UserScopedFactKind("preference") || !UserScopedFactKind("user_identity") || !UserScopedFactKind("constraint") {
		t.Fatal("identity/preference/constraint must be user-scoped")
	}
	if UserScopedFactKind("domain_knowledge") || UserScopedFactKind("general") {
		t.Fatal("domain/general must stay agent-scoped")
	}
}
