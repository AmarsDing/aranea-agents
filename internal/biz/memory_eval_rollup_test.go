package biz

import (
	"strings"
	"testing"
)

// TestAssembleEvalProfileDigest covers AMC P3-a 拼装：分区模板渲染、角色前
// 缀剥除、rollup 行自我排除、每分区上限与空输入。
func TestAssembleEvalProfileDigest(t *testing.T) {
	t.Parallel()
	facts := []EvalRollupFact{
		{Statement: "user: 我叫张三", Kind: "user_identity", ValidFrom: "2025-01-01T08:00:00Z"},
		{Statement: "user: 我最喜欢的颜色是蓝色", Kind: "preference", ValidFrom: "2025-02-01T08:00:00Z"},
		{Statement: "user: 不要把我的信息透露给别人", Kind: "constraint", ValidFrom: "2025-03-01T08:00:00Z"},
		{Statement: "user: 昨天我去医院体检了", Kind: "event", ValidFrom: "2025-04-01T08:00:00Z"},
		// rollup 行必须被自我排除，否则 digest 套 digest。（fixture 需为
		// canonical 形态——评审 C5 后裸前缀不再算 rollup 行。）
		{Statement: "[profile-digest] 身份: 旧 digest", Kind: "domain_knowledge", ValidFrom: "2025-04-02T08:00:00Z"},
		{Statement: "[series-digest: 步数] count=3 last=1步 mean=1.0步 min=1步 max=1步 firstValue=1步 firstDay=2025-04-01 lastDay=2025-04-03", Kind: "domain_knowledge", ValidFrom: "2025-04-03T08:00:00Z"},
		{Statement: "[slot-history: favorite:颜色] x(2025-04-01, 已失效) → y(2025-04-04)", Kind: "domain_knowledge", ValidFrom: "2025-04-04T08:00:00Z"},
	}
	got := AssembleEvalProfileDigest(facts)
	if !IsEvalProfileDigestStatement(got) {
		t.Fatalf("digest must carry the identity prefix: %q", got)
	}
	for _, want := range []string{"身份: 我叫张三", "偏好: 我最喜欢的颜色是蓝色", "约束: 不要把我的信息透露给别人", "其他: 昨天我去医院体检了"} {
		if !strings.Contains(got, want) {
			t.Fatalf("digest missing %q: %q", want, got)
		}
	}
	for _, banned := range []string{"旧 digest", "series-digest", "slot-history", "user: "} {
		if strings.Contains(got, banned) {
			t.Fatalf("digest leaked %q: %q", banned, got)
		}
	}
	if AssembleEvalProfileDigest(nil) != "" {
		t.Fatal("empty input must render empty digest")
	}
	// 每分区上限：10 条偏好只保留最新 8 条。
	many := make([]EvalRollupFact, 0, 10)
	for i := 0; i < 10; i++ {
		many = append(many, EvalRollupFact{
			Statement: "user: 偏好项" + string(rune('A'+i)), Kind: "preference",
			ValidFrom: "2025-01-0" + string(rune('1'+i)) + "T00:00:00Z",
		})
	}
	got = AssembleEvalProfileDigest(many)
	if strings.Contains(got, "偏好项A") || strings.Contains(got, "偏好项B") {
		t.Fatalf("section cap must drop the oldest facts: %q", got)
	}
	if !strings.Contains(got, "偏好项I") || !strings.Contains(got, "偏好项J") {
		t.Fatalf("section cap must keep the newest facts: %q", got)
	}
}

// TestAssembleEvalSlotHistory covers AMC P3-a 取值链：按 valid_from 升序成
// 链、失效值带标注、单值不成链、未知日期兜底。
func TestAssembleEvalSlotHistory(t *testing.T) {
	t.Parallel()
	entries := []EvalRollupFact{
		{Statement: "user: 我现在把最喜欢的颜色改成红色了", Kind: "preference", ValidFrom: "2025-03-05T08:00:00Z"},
		{Statement: "user: 我最喜欢的颜色是蓝色", Kind: "preference", ValidFrom: "2025-03-01T08:00:00Z", ValidUntil: "2025-03-05T08:01:00Z"},
	}
	got := AssembleEvalSlotHistory("favorite:颜色", entries)
	if !IsEvalSlotHistoryStatement(got) {
		t.Fatalf("history must carry the identity prefix: %q", got)
	}
	iBlue := strings.Index(got, "蓝色")
	iRed := strings.Index(got, "红色")
	if iBlue < 0 || iRed < 0 || iBlue > iRed {
		t.Fatalf("chain must order values by valid_from asc: %q", got)
	}
	if !strings.Contains(got, "(2025-03-01, 已失效)") || !strings.Contains(got, "(2025-03-05)") {
		t.Fatalf("date/invalidation markers wrong: %q", got)
	}
	if !strings.Contains(got, " → ") {
		t.Fatalf("chain arrow missing: %q", got)
	}
	// 单值不成链。
	if AssembleEvalSlotHistory("favorite:颜色", entries[:1]) != "" {
		t.Fatal("single-entry slot must not render a history row")
	}
	// 未知日期兜底不 panic、行仍可读。
	got = AssembleEvalSlotHistory("live", []EvalRollupFact{
		{Statement: "user: 我住在杭州", Kind: "preference"},
		{Statement: "user: 我搬到上海了", Kind: "preference"},
	})
	if !strings.Contains(got, "未知日期") {
		t.Fatalf("missing-day fallback wrong: %q", got)
	}
}

// TestEvalRollupCanonicalGuards covers AMC P3 评审 C5：身份识别是整行
// canonical 匹配而非裸前缀——用户原句恰好以 marker 开头（数据注入）不得被
// 当作派生行采纳/轮换/置顶；真实渲染行（含 未知日期 兜底）必须识别。
func TestEvalRollupCanonicalGuards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt              string
		profile, slotHist bool
	}{
		{"[profile-digest] 身份: 我叫张三 | 偏好: 我喜欢蓝色", true, false},
		{"[profile-digest] 偏好: 偏好项X", true, false},
		// 裸前缀/残缺形态不得识别。
		{"[profile-digest]", false, false},
		{"[profile-digest] 胡说八道", false, false},
		{"[slot-history: favorite:颜色] 我最喜欢的颜色是蓝色(2025-03-01, 已失效) → 我最喜欢的颜色是红色(2025-03-05)", false, true},
		{"[slot-history: live] 我住在杭州(未知日期) → 我搬到上海了(未知日期)", false, true},
		{"[slot-history: favorite:颜色] 没有日期括号", false, false},
		{"[slot-history: ] 空槽名(2025-03-01)", false, false},
		// 普通事实与角色前缀行不受影响。
		{"user: 我喜欢蓝色", false, false},
	}
	for _, tc := range cases {
		if got := IsEvalProfileDigestStatement(tc.stmt); got != tc.profile {
			t.Errorf("IsEvalProfileDigestStatement(%q) = %v, want %v", tc.stmt, got, tc.profile)
		}
		if got := IsEvalSlotHistoryStatement(tc.stmt); got != tc.slotHist {
			t.Errorf("IsEvalSlotHistoryStatement(%q) = %v, want %v", tc.stmt, got, tc.slotHist)
		}
	}
}

// TestEvalSlotHistorySlotSanitize_Round3 covers AMC P3 评审 R2-C5-1：槽名
// 来自用户输入捕获（chineseFavoriteRe 不排除 ']'），canonical 头必须对
// 渲染器自闭合——剥离括号 + 定长后，渲染行过自己的 canonical 正则、轮转
// 前缀扫描与识别一致（此前 rotated 永不失效旧行、profile digest 把链行
// 当源事实重吸收，自重复累积）。
func TestEvalSlotHistorySlotSanitize_Round3(t *testing.T) {
	t.Parallel()
	slot := "favorite:颜色]"
	entries := []EvalRollupFact{
		{Statement: "user: 我最喜欢的颜色]是蓝色", Kind: "preference", ValidFrom: "2026-10-01T08:00:00Z", ValidUntil: "2026-10-02T08:00:00Z"},
		{Statement: "user: 我最喜欢的颜色]改成红色", Kind: "preference", ValidFrom: "2026-10-02T08:00:00Z"},
	}
	line := AssembleEvalSlotHistory(slot, entries)
	if line == "" {
		t.Fatal("bracketed slot must still render a chain")
	}
	if !IsEvalSlotHistoryStatement(line) || !IsEvalRollupStatement(line) {
		t.Fatalf("rendered line must pass its own canonical guards: %q", line)
	}
	if !strings.HasPrefix(line, EvalSlotHistoryStatementPrefix(slot)) {
		t.Fatalf("rotate prefix scan must match the rendered head: %q", line)
	}
	// 超长槽名：canonical 头定长截断，尾部 2000-rune 上限影响不到头。
	longSlot := "favorite:" + strings.Repeat("a", 500)
	l2 := AssembleEvalSlotHistory(longSlot, []EvalRollupFact{
		{Statement: "user: 颜色甲", Kind: "preference", ValidFrom: "2026-10-01T08:00:00Z", ValidUntil: "2026-10-02T08:00:00Z"},
		{Statement: "user: 颜色乙", Kind: "preference", ValidFrom: "2026-10-02T08:00:00Z"},
	})
	if !IsEvalSlotHistoryStatement(l2) || !strings.HasPrefix(l2, EvalSlotHistoryStatementPrefix(longSlot)) {
		t.Fatalf("overlong slot must stay canonical: %q", l2)
	}
}
