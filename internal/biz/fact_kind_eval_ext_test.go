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
		{"如果明天下雨", false}, // 无结果标记 → 非假设句
		{"明天就出发", false},  // 无 如果 → 非假设句
		{"If I move to Berlin then I can bike", true},
		{"I would move there", false}, // 无定语从句 → 非假设句
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

// TestLooksLikeHealthIdentityStatement covers AMC P3 / E3：健康/家庭长期
// 状态谓词命中；一次性事件句、普通偏好句不命中。
func TestLooksLikeHealthIdentityStatement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"我患有糖尿病", true},
		{"我对青霉素过敏", true},
		{"确诊高血压已经一年了", true},
		{"我父亲去世那年我非常难过", true},
		{"她丧偶多年一直独居", true},
		{"我离异后独自居住在这套老房子里", true},
		{"她怀孕五个月了", true},
		{"我长期失眠，需要靠助眠药入睡", true},
		{"I'm allergic to peanuts", true},
		{"I take metformin every morning", true},
		{"she was widowed last year", true},
		{"he lives alone in the suburbs", true},
		{"my wife is pregnant", true},
		{"我昨天去公园跑了五公里", false},
		{"他很喜欢吃辣", false},
		{"那家医院的环境不错", false},
		{"今天天气真好", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := LooksLikeHealthIdentityStatement(tc.stmt); got != tc.want {
			t.Errorf("LooksLikeHealthIdentityStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestLooksLikeHealthIdentityStatement_C7Guards covers AMC P3 评审 C7：
// 动词短语必须带限定宾语——裸 "lost my keys"/"i take the bus" 是日常事件
// 不是长期状态；只有 kin/med 宾语才命中 evergreen。
func TestLooksLikeHealthIdentityStatement_C7Guards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		// 误伤面（此前裸 "lost my "/"i take " 命中）
		{"i lost my keys yesterday", false},
		{"i lost my wallet on the metro", false},
		{"i take the bus to work", false},
		{"i take two sugars in my coffee", false},
		{"i'm taking the day off", false},
		// 命中面：kin / med 宾语
		{"i lost my father last year", true},
		{"i lost my wife in march", true},
		{"lost my husband", true},
		{"i lost my grandmother last winter", true},
		{"i take insulin every morning", true},
		{"i'm taking vitamins", true},
		{"i take blood pressure medication", true},
		// AMC P3 评审 round-3 R2-C7-2：复数亲属同属 "lost my <kin>" 类
		// （"parents" 早收而 "children" 缺位是词表非对称）。
		{"i lost my children in the fire", true},
		{"i lost my kids last year", true},
		{"i lost my grandparents", true},
		{"i lost my siblings in the accident", true},
		// AMC P3 评审 round-3 R2-C7-1：Han↔latin 字类切换分词——混写词条
		// 命中 CN 词表（"维生素c" → 维生素/c）。
		{"i take 维生素c daily", true},
		{"i'm taking 维生素c", true},
		{"i take 胰岛素 every morning", true},
	}
	for _, tc := range cases {
		if got := LooksLikeHealthIdentityStatement(tc.stmt); got != tc.want {
			t.Errorf("LooksLikeHealthIdentityStatement(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestCanonicalizeFactKind_HealthRoutesToUserIdentity pins AMC P3 / E3:
// statement 启发式优先于 LLM 标签——健康长期状态即使被标成 event/
// preference 也必须升入 user_identity（evergreen），普通事件不受影响。
func TestCanonicalizeFactKind_HealthRoutesToUserIdentity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, stmt string
		want       string
	}{
		{"event", "我患有糖尿病", "user_identity"},
		{"", "我对青霉素过敏", "user_identity"},
		{"preference", "I'm allergic to peanuts", "user_identity"},
		{"general", "我离异后独自居住", "user_identity"},
		{"event", "我昨天去爬了山", "event"},
		{"fact", "今天天气真好", "fact"},
	}
	for _, tc := range cases {
		if got := CanonicalizeFactKind(tc.kind, tc.stmt); got != tc.want {
			t.Errorf("CanonicalizeFactKind(%q, %q) = %q, want %q", tc.kind, tc.stmt, got, tc.want)
		}
	}
}

// TestStripEvalHypotheticalClauses covers AMC P3 评审 R2-C6-1：混合句
// 「事实，如果…就…」只剔反事实从句、保留事实前缀；条件后的结果从句
// （含 就/would）同剔；纯假设整体剔除；非假设句恒等。
func TestStripEvalHypotheticalClauses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want string
	}{
		{"我今天走了8000步，如果状态好明天就走10000步", "我今天走了8000步"},
		{"如果我练出毛病就少跑2000步", ""},
		{"如果明天下雨，我就不跑10公里", ""},
		{"if I trained harder, I would run 8000 steps", ""},
		// 条件出现之前的从句保留事实面——「我七点就起床」的 就 是习惯副
		// 词，不是结果标记。
		{"我七点就起床了，如果明天下雨就取消晨跑", "我七点就起床了"},
		{"我住在杭州", "我住在杭州"},
		{"我喜欢跑步，今天跑了5公里", "我喜欢跑步，今天跑了5公里"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := StripEvalHypotheticalClauses(tc.stmt); got != tc.want {
			t.Errorf("StripEvalHypotheticalClauses(%q) = %q, want %q", tc.stmt, got, tc.want)
		}
	}
}

// TestMergeEvalHypotheticalAtoms covers AMC P3 评审 R2-C6-2：原子化按句终
// 标点切分会把条件标记剥到上一枚——结果句（就/则/便/then 起头）与前枚
// 条件句（含 如果/if）合回整原子，整原子假设守卫才能在三条派生链上统一
// 拦截。无条件的起头句、非结果句不合并。
func TestMergeEvalHypotheticalAtoms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		atoms []string
		want  []string
	}{
		{[]string{"如果明天下雨。", "我就不跑10公里了。"}, []string{"如果明天下雨。我就不跑10公里了。"}},
		{[]string{"If it rains.", "Then I will stay home."}, []string{"If it rains.Then I will stay home."}},
		// 第三枚非结果句不并入。
		{[]string{"如果明天下雨。", "我就取消晨跑。", "我继续上班。"}, []string{"如果明天下雨。我就取消晨跑。", "我继续上班。"}},
		// 前枚无条件标记 → 「就」起头也是事实句，不合并。
		{[]string{"他喜欢跑步。", "就要坚持。"}, []string{"他喜欢跑步。", "就要坚持。"}},
		{[]string{"我住在杭州。", "我喜欢蓝色。"}, []string{"我住在杭州。", "我喜欢蓝色。"}},
		// R2-C6-2 定向收紧：头窗外（>3 字符）的 就 是日常副词——条件无关的
		// 事实句不得被合并进假设原子而丢序列/档案（「我今天就走了8000步」
		// 是真实观测）。
		{[]string{"如果明天下雨。", "我今天就走了8000步。"}, []string{"如果明天下雨。", "我今天就走了8000步。"}},
		{nil, nil},
	}
	for i, tc := range cases {
		got := MergeEvalHypotheticalAtoms(tc.atoms)
		if len(got) != len(tc.want) {
			t.Fatalf("case %d: len=%d want %d (%v)", i, len(got), len(tc.want), got)
		}
		for j := range got {
			if got[j] != tc.want[j] {
				t.Errorf("case %d atom[%d] = %q, want %q", i, j, got[j], tc.want[j])
			}
		}
	}
}
