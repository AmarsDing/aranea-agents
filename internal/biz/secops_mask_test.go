package biz

// D6 L-2 脱敏引擎测试（验收 #3 的引擎半边：placeholder 会话一致、灰度、
// shadow、secondCheck 四检测器；语义锚 = workbench server/mask.go 逐字）。

import (
	"fmt"
	"strings"
	"testing"
)

// 测试常量（全部满足/违反真实校验位构造，见各用例注释）。
const (
	maskTestPhone   = "13812345678"        // 合法手机号
	maskTestIDCard  = "11010519491231002X" // 校验位有效的身份证（167%11=2→X）
	maskTestIDBadID = "110101199003070018" // 校验位无效的身份证（wb 种子同款，154%11=0→'1'）
	maskTestBank    = "4111111111111111"   // Luhn 有效的 16 位卡号
	maskTestEmail   = "ops@example.com"    // 普通邮箱
)

func maskTestRule(id, entity, pattern string, operator string) MaskRule {
	return MaskRule{
		ID: id, Name: id, Entity: entity, Pattern: pattern,
		Operator: operator, Enabled: true, RolloutPct: 100, Version: 1,
	}
}

func newMaskTestEngine(t *testing.T, rules ...MaskRule) *MaskEngine {
	t.Helper()
	eng, err := NewMaskEngine(rules)
	if err != nil {
		t.Fatalf("NewMaskEngine: %v", err)
	}
	return eng
}

func TestMaskEngine_PlaceholderConsistencyPerScope(t *testing.T) {
	rule := maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder)
	eng := newMaskTestEngine(t, rule)

	out1, hits1 := eng.Mask("sess-a", "联系 "+maskTestPhone+" 或再拨 "+maskTestPhone)
	if hits1 != 2 {
		t.Fatalf("hits = %d, want 2", hits1)
	}
	// 同一作用域同一原文 → 同一占位符 <手机号_1>，出现两次。
	if strings.Count(out1, "<手机号_1>") != 2 {
		t.Fatalf("placeholder consistency broken: %q", out1)
	}
	if strings.Contains(out1, maskTestPhone) {
		t.Fatalf("original leaked after masking: %q", out1)
	}
	// 不同原文 → per-scope per-entity 递增占位符。
	out2, _ := eng.Mask("sess-a", "备用 "+"13900001111")
	if !strings.Contains(out2, "<手机号_2>") {
		t.Fatalf("expected incremented placeholder <手机号_2>, got %q", out2)
	}
	// 不同作用域各自从 1 重新计数。
	out3, _ := eng.Mask("sess-b", "联系 "+maskTestPhone)
	if strings.Count(out3, "<手机号_1>") != 1 {
		t.Fatalf("per-scope partition broken: %q", out3)
	}
}

func TestMaskEngine_OperatorMaskReplacesWithStars(t *testing.T) {
	rule := maskTestRule("mr_x", "PASSWORD", `[A-Za-z]{4}`, MaskOperatorMask)
	eng := newMaskTestEngine(t, rule)
	out, hits := eng.Mask("s", "ab abcd efgh end")
	if hits != 2 || out != "ab *** *** end" {
		t.Fatalf("masked = %q hits=%d, want 'ab *** *** end' hits=2", out, hits)
	}
}

func TestMaskEngine_ShadowCountsOnly(t *testing.T) {
	rule := maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder)
	rule.Shadow = true
	eng := newMaskTestEngine(t, rule)
	out, hits := eng.Mask("s", "联系 "+maskTestPhone)
	if hits != 1 {
		t.Fatalf("shadow hits = %d, want 1", hits)
	}
	if out != "联系 "+maskTestPhone {
		t.Fatalf("shadow rule must not replace: %q", out)
	}
}

func TestMaskEngine_WordlistMask(t *testing.T) {
	rule := MaskRule{
		ID: "mr_w", Name: "mr_w", Entity: "内部项目代号",
		Wordlist: []string{"phoenix", "PHOENIX"}, Operator: MaskOperatorPlaceholder,
		Enabled: true, RolloutPct: 100,
	}
	eng := newMaskTestEngine(t, rule)
	out, hits := eng.Mask("s", "推进 phoenix-2026 与 PHOENIX 2077")
	if hits != 2 {
		t.Fatalf("wordlist hits = %d, want 2", hits)
	}
	if !strings.Contains(out, "<内部项目代号_1>") || !strings.Contains(out, "<内部项目代号_2>") {
		t.Fatalf("wordlist placeholders missing: %q", out)
	}
	if strings.Contains(out, "phoenix") || strings.Contains(out, "PHOENIX") {
		t.Fatalf("wordlist originals leaked: %q", out)
	}
}

func TestMaskRolloutHit_BoundariesAndStability(t *testing.T) {
	if !MaskRolloutHit("any", 100) || !MaskRolloutHit("any", 200) {
		t.Fatal("pct >= 100 must always hit")
	}
	if MaskRolloutHit("any", 0) || MaskRolloutHit("any", -5) {
		t.Fatal("pct <= 0 must never hit")
	}
	// 灰度桶对同 scope 稳定。
	first := MaskRolloutHit("sess-stable", 50)
	for i := 0; i < 8; i++ {
		if MaskRolloutHit("sess-stable", 50) != first {
			t.Fatal("rollout bucket must be stable for the same scope")
		}
	}
	// 灰度跳过禁用规则：pct=0 的规则对同一文本不命中也不替换。
	rule := maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder)
	rule.RolloutPct = 0
	eng := newMaskTestEngine(t, rule)
	out, hits := eng.Mask("s", "联系 "+maskTestPhone)
	if hits != 0 || out != "联系 "+maskTestPhone {
		t.Fatalf("rollout=0 rule must be skipped entirely: %q hits=%d", out, hits)
	}
}

func TestNewMaskEngine_SkipsDisabledAndInert(t *testing.T) {
	enabled := maskTestRule("a", "X", `x`, MaskOperatorPlaceholder)
	disabled := maskTestRule("b", "Y", `y`, MaskOperatorPlaceholder)
	disabled.Enabled = false
	inert := maskTestRule("c", "Z", "", MaskOperatorPlaceholder) // 无正则无词表
	eng := newMaskTestEngine(t, enabled, disabled, inert)
	if got := eng.RuleCount(); got != 1 {
		t.Fatalf("RuleCount = %d, want 1 (disabled/inert skipped)", got)
	}
}

func TestNewMaskEngine_BadPatternFailsGroup(t *testing.T) {
	_, err := NewMaskEngine([]MaskRule{maskTestRule("bad", "X", `[unclosed`, MaskOperatorPlaceholder)})
	if err == nil {
		t.Fatal("bad regex must fail engine construction (fail-closed, no partial rules)")
	}
}

// ─── secondCheck 四检测器（验收 #3 核心：独立代码路径、内置高精确度）────────

func TestSecondCheck_IDCardChecksum(t *testing.T) {
	eng := newMaskTestEngine(t)
	f := eng.SecondCheck("证件 " + maskTestIDCard + " 请录入")
	if len(f) != 1 || f[0] != "idcard:"+maskTestIDCard {
		t.Fatalf("valid idcard must be found, got %v", f)
	}
	if f := eng.SecondCheck(maskTestIDBadID); len(f) != 0 {
		t.Fatalf("invalid-checksum idcard must NOT be a finding, got %v", f)
	}
}

func TestSecondCheck_Phone(t *testing.T) {
	eng := newMaskTestEngine(t)
	f := eng.SecondCheck("电话 " + maskTestPhone)
	if len(f) != 1 || strings.HasPrefix(f[0], "phone:") == false {
		t.Fatalf("phone must be found, got %v", f)
	}
	// 11 位但非 1[3-9] 开头不误报；12 位含边界的数字串不误报。
	if f := eng.SecondCheck("订单 12345678901 与编号 138123456780"); len(f) != 0 {
		t.Fatalf("non-phone digit runs must not be findings, got %v", f)
	}
}

func TestSecondCheck_BankCardLuhn(t *testing.T) {
	eng := newMaskTestEngine(t)
	f := eng.SecondCheck("卡号 " + maskTestBank)
	if len(f) != 1 || f[0] != "bankcard:"+maskTestBank {
		t.Fatalf("luhn-valid card must be found, got %v", f)
	}
	// Luhn 无效的 16 位数字不报。
	if f := eng.SecondCheck("编号 4111111111111110"); len(f) != 0 {
		t.Fatalf("luhn-invalid card must not be a finding, got %v", f)
	}
	// 18 位且身份证校验有效 → bankcard 检测排除（避免双报）。
	if f2 := eng.SecondCheck(maskTestIDCard); len(f2) != 1 || !strings.HasPrefix(f2[0], "idcard:") {
		t.Fatalf("valid idcard must report exactly idcard (not bankcard), got %v", f2)
	}
}

func TestSecondCheck_Email(t *testing.T) {
	eng := newMaskTestEngine(t)
	f := eng.SecondCheck("邮箱 " + maskTestEmail)
	if len(f) != 1 || f[0] != "email:"+maskTestEmail {
		t.Fatalf("email must be found, got %v", f)
	}
}

func TestSecondCheckKinds_UniqueOrderPreserved(t *testing.T) {
	got := SecondCheckKinds([]string{
		"idcard:" + maskTestIDCard,
		"phone:" + maskTestPhone,
		"phone:" + "13900001111",
		"email:" + maskTestEmail,
	})
	want := []string{"idcard", "phone", "email"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("kinds = %v, want %v (unique, first-seen order)", got, want)
	}
}

// 规则覆盖后上云文本无 findings（一级脱敏与二级检测的配合语义）。
func TestMaskThenSecondCheck_NoFindingsWhenCovered(t *testing.T) {
	eng := newMaskTestEngine(t,
		maskTestRule("mr_01_idcard", "身份证号", `\b\d{17}[\dXx]\b`, MaskOperatorPlaceholder),
		maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder),
	)
	text := "证件 " + maskTestIDCard + "，电话 " + maskTestPhone
	masked, hits := eng.Mask("s", text)
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
	if got := eng.SecondCheck(masked); len(got) != 0 {
		t.Fatalf("masked text must be finding-free when rules cover, got %v", got)
	}
}

// 作用域上限：超容量淘汰一个作用域，引擎继续可用（常驻进程防无界增长）。
func TestMaskEngine_ScopeCap(t *testing.T) {
	eng := newMaskTestEngine(t, maskTestRule("mr_02_phone", "手机号", `\b1[3-9]\d{9}\b`, MaskOperatorPlaceholder))
	for i := 0; i < maxMaskEngineScopes+32; i++ {
		_, _ = eng.Mask(fmt.Sprintf("scope-%d", i), maskTestPhone)
	}
	eng.mu.Lock()
	n := len(eng.maps)
	eng.mu.Unlock()
	if n > maxMaskEngineScopes {
		t.Fatalf("scope maps = %d, cap = %d", n, maxMaskEngineScopes)
	}
}
