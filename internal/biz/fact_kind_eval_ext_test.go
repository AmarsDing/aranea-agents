package biz

import "testing"

// TestLooksLikeConstraintStatement_EvalExt covers the AMC C3-A Q7a cue
// extensions (AMC C3 修复 #2 收紧后为 如果/每当/只要/规定/规则/whenever/
// if…then/policy；裸字串 若/按照 因误伤面移出清单).
func TestLooksLikeConstraintStatement_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"如果明天下雨我就取消露营", true},
		// #2: 裸「若」字串误伤 若干/若隐若现，收紧后不再命中；
		// 「若…就」式规则改由 假设句守卫在检索后的 supersede 阶段拦截。
		{"若客户再来电话就转给我", false},
		{"若干问题待解决", false},
		{"每当有人进门要说欢迎", true},
		{"只要订单超过一百就免运费", true},
		// #2: 「按照计划执行」是程序性陈述而非规则，裸「按照」已移出。
		{"按照公司流程报销需要发票", false},
		{"公司有规定要打卡", true},
		{"他喜欢研究规则怪谈", true},
		{"Whenever it rains, take an umbrella", true},
		{"If it rains then stay home", true},
		{"Company policy forbids smoking here", true},
		{"I always drink coffee in the morning", false},
		{"我喜欢喝拿铁", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := LooksLikeConstraintStatement(tc.stmt); got != tc.want {
			t.Errorf("LooksLikeConstraintStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestIsHypotheticalStatement covers 修复 #2: conditional 句式仅入约束归类
// （G 维度可检索），但绝不允许 supersede 任何在册事实。
func TestIsHypotheticalStatement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"如果我搬到上海就退租", true},
		{"假如涨价则停止订阅", true},
		{"要是明天下雨便取消出行", true},
		{"我会取消露营", false},
		{"如果明天下雨", false},       // 无结果标记 → 非假设句
		{"明天就出发", false},           // 无 如果 → 非假设句
		{"If I move to Berlin then I can bike", true},
		{"I would move there", false},  // 无定语从句 → 非假设句
		{"", false},
	}
	for _, tc := range cases {
		if got := IsHypotheticalStatement(tc.stmt); got != tc.want {
			t.Errorf("IsHypotheticalStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestShouldSupersedeSameSlotFact_Hypothetical pins 修复 #2: 假设句命中 live
// 槽位也不失效在册居住事实；非假设的新值照常 supersede。
func TestShouldSupersedeSameSlotFact_Hypothetical(t *testing.T) {
	t.Parallel()
	old := "我住在杭州"
	kind := "preference"
	if !ShouldSupersedeSameSlotFact(kind, old, kind, "我搬到上海了") {
		t.Fatal("real move should supersede old residence")
	}
	if ShouldSupersedeSameSlotFact(kind, old, kind, "如果我搬到上海就退租") {
		t.Fatal("hypothetical must never supersede the live residence fact")
	}
}

// TestPreferenceSlotKey_EvalExt covers the AMC C3-A Q7c slot extensions
// (搬到/搬去/moved to→live, 换工作/入职/新工作→job, 手机号/电话号码→phone).
func TestPreferenceSlotKey_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want string
	}{
		{"我搬到了上海", "live"},
		{"她搬去了北京", "live"},
		{"I moved to Berlin last month", "live"},
		{"我换工作了", "job"},
		{"我入职了新的互联网公司", "job"},
		{"我找到了一份新工作", "job"},
		{"我的手机号是13800001111", "phone"},
		{"我的电话号码是13800001111", "phone"},
		{"我最喜欢的颜色是红色", "favorite:颜色"},
		{"今天天气不错", ""},
	}
	for _, tc := range cases {
		if got := PreferenceSlotKey(tc.stmt); got != tc.want {
			t.Errorf("PreferenceSlotKey(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

// TestHasPreferenceUpdateCue_EvalExt covers 不喜欢了/没感觉/改为 added by
// AMC C3-A Q7c (existing cues remain covered by older tests).
func TestHasPreferenceUpdateCue_EvalExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"我现在不喜欢蓝色了", true},
		{"我对咖啡没感觉", true},
		{"我的最爱改为红茶", true},
		{"最喜欢改成乌龙茶", true},
		{"换成冰美式了", true},
		{"I prefer tea now", true},
		{"我喜欢茉莉花茶", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := HasPreferenceUpdateCue(tc.stmt); got != tc.want {
			t.Errorf("HasPreferenceUpdateCue(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}
