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
		{"event", "I like blue", "preference"},
		{"fact", "My favorite color is red", "preference"},
		{"", "You must never share the password", "constraint"},
		{"event", "I always drink coffee", "event"},
		{"", "I live in Seattle", "preference"},
	}
	for _, tc := range cases {
		if got := CanonicalizeFactKind(tc.kind, tc.stmt); got != tc.want {
			t.Errorf("CanonicalizeFactKind(%q, %q) = %q, want %q", tc.kind, tc.stmt, got, tc.want)
		}
	}
}

func TestLooksLikeAbsenceMetaStatement(t *testing.T) {
	poison := []string{
		// 2026-08-26 domain-B 污染循环实录
		"用户询问公司当前的变更窗口安排，但系统中尚无相关记录，需要用户提供后再记忆",
		"用户询问当前值班电话号码，但系统中暂无此信息",
		"用户询问的是公司团队当前的值班电话，该信息目前不在记忆/知识库/工作区中",
		"The user asked about the maintenance window, but there is no record of it",
		"用户问出口带宽，未找到相关信息",
	}
	for _, s := range poison {
		if !LooksLikeAbsenceMetaStatement(s) {
			t.Errorf("absence meta-statement must be detected: %q", s)
		}
	}
	genuine := []string{
		// 真实否定事实：无询问元标记
		"值班电话原号码为0571-6655-0001，现已更换为0571-8899-1234",
		"原巡检周期每月一次已作废",
		"出口带宽不再是1Gbps",
		"值班电话是0571-8899-1234",
		// 询问标记但无缺失标记：真实事实
		"用户询问过值班电话，新号码为0571-8899-1234",
		"",
	}
	for _, s := range genuine {
		if LooksLikeAbsenceMetaStatement(s) {
			t.Errorf("genuine fact must not be filtered: %q", s)
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

func TestPreferenceSlotKey(t *testing.T) {
	cases := []struct {
		stmt, want string
	}{
		{"My favorite color is red", "favorite:color"},
		{"I live in Seattle", "live"},
		{"我最喜欢的颜色是红色", "favorite:颜色"},
		{"我住在杭州", "live"},
		{"I like coffee", "like"},
		{"杂项陈述", ""},
	}
	for _, tc := range cases {
		if got := PreferenceSlotKey(tc.stmt); got != tc.want {
			t.Errorf("PreferenceSlotKey(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

func TestHasPreferenceUpdateCue_Chinese(t *testing.T) {
	if !HasPreferenceUpdateCue("现在喜欢茶") {
		t.Fatal("现在 must count as an update cue")
	}
	if HasPreferenceUpdateCue("喜欢茶") {
		t.Fatal("bare 喜欢 must not count as an update cue")
	}
}
