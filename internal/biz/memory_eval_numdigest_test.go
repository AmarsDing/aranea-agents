package biz

import (
	"testing"
)

// TestParseEvalNumericAtoms covers AMC P3-c：同键主题数值三元组抽取。
// 日期形态（2025年5月6日 / 2025-05-06）必须先被遮蔽，绝不进序列；
// 单位用字符类捕获（任意语言单位都能存活），连接词不做意图式枚举。
func TestParseEvalNumericAtoms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		stmt string
		want []EvalNumericAtom
	}{
		{"中文步数", "user: 本周步数是3200步", []EvalNumericAtom{{Topic: "步数", Value: 3200, Unit: "步"}}},
		{"中文血压双值", "user: 早上血压128/86mmHg", []EvalNumericAtom{{Topic: "血压", Value: 128, Unit: ""}}},
		{"中文体重", "user: 今天体重75.3公斤", []EvalNumericAtom{{Topic: "体重", Value: 75.3, Unit: "公斤"}}},
		{"英文 steps", "user: my steps were 4500 steps", []EvalNumericAtom{{Topic: "steps", Value: 4500, Unit: ""}}},
		{"英文 weight", "user: weight dropped to 180 lb", []EvalNumericAtom{{Topic: "weight", Value: 180, Unit: ""}}},
		{"同主题多值继承", "user: 本周步数分别是3200、4500、2800步", []EvalNumericAtom{
			{Topic: "步数", Value: 3200, Unit: ""},
			{Topic: "步数", Value: 4500, Unit: ""},
			{Topic: "步数", Value: 2800, Unit: "步"},
		}},
		{"日期不进序列", "user: 我在2025年5月6日走了3200步", []EvalNumericAtom{{Topic: "走", Value: 3200, Unit: "步"}}},
		{"ISO 日期不进序列", "user: on 2025-05-06 I walked 3200 steps", []EvalNumericAtom{{Topic: "walked", Value: 3200, Unit: ""}}},
		{"无数值", "user: 我喜欢蓝色", nil},
		{"纯日期", "user: 今天是2025年5月7日", nil},
		{"手机号不进序列", "user: 我的手机号是13812345678", nil},
		{"身份证不进序列", "user: 身份证号是110101199001011234", nil},
		{"银行卡不进序列", "user: 卡号是6222021001116245", nil},
		// 拉丁月/裸年份同样是日历语法——"May 1"/"in 2025" 绝不成序列。
		{"拉丁月日期不进序列", "user: my birthday is May 1", nil},
		{"拉丁月倒装不进序列", "user: the trip was on 3rd of June", nil},
		{"裸年份不进序列", "user: I moved here in 2025", nil},
		{"中文裸年份不进序列", "user: 我1990年出生", nil},
		// 的 是领属助词不是主题：「我的体重」主题须归一到 体重。
		{"领属助词剥除", "user: 我的体重是75公斤", []EvalNumericAtom{{Topic: "体重", Value: 75, Unit: "公斤"}}},
		// AMC P3 评审 C4：标签词+短数字是标识符语法（覆盖 是/为/冒号 连接），
		// 绝不成度量序列——「工号3688」这类正是 ≥8 位守卫的盲区。
		{"工号冒号遮蔽", "user: 我的工号:3688 记住了", nil},
		{"工号是字遮蔽", "user: 我的工号是9527", nil},
		{"尾号遮蔽", "user: 银行卡尾号3688 别泄露", nil},
		{"英文 id 遮蔽", "user: my employee id: 3688 works", nil},
		{"英文 id-is 遮蔽", "user: my id is 3688 now", nil},
		// AMC P3 评审 C4：单位捕获吞下语气词（了/的/是…）时双侧剥除——
		// 「3200步了」单位是 步 不是 步了。
		{"单位语气词剥除", "user: 今天走了3200步了", []EvalNumericAtom{{Topic: "走", Value: 3200, Unit: "步"}}},
		{"单位语气词剥除2", "user: 他体重涨到75公斤了", []EvalNumericAtom{{Topic: "体重", Value: 75, Unit: "公斤"}}},
		// AMC P3 评审 C6：假设句（如果…就/if…would）是反事实，绝不成序列。
		{"假设句跳过", "user: 如果我练出毛病就少跑2000步", nil},
		{"if-would 跳过", "user: if I trained harder I would run 8000 steps", nil},
		// AMC P3 评审 round-3 F2/F3/F4：遮罩跨语统一——CN 标签连字符、EN
		// 过去时（was/were）、拉丁标签+CN 连接词（及镜像）都是标识符语法。
		{"工号连字符遮蔽", "user: 我的工号-9527 已经用了三年", nil},
		{"订单号连字符遮蔽", "user: 订单号-1234 快到了", nil},
		{"英文 id-was 遮蔽", "user: my employee id was 3688 last year", nil},
		{"英文 code-was 遮蔽", "user: the code was 5000", nil},
		{"拉丁标签中文连接遮蔽", "user: 我的id是9527", nil},
		{"拉丁标签为字遮蔽", "user: 我的uid为3688", nil},
		{"中文标签is连接遮蔽", "user: 工号is9527", nil},
		// 遮罩不得误伤度量："were" 不误掩非标签词的步数。
		{"步数were不误掩", "user: my steps were 4500 steps", []EvalNumericAtom{{Topic: "steps", Value: 4500, Unit: ""}}},
		// AMC P3 评审 round-3 F6：混合句只收事实从句——真实观测进序列，
		// 反事实尾句剔除；条件后的结果从句（含 就/would）同样剔除。
		{"混合句收事实弃假设", "user: 我今天走了8000步，如果状态好明天就走10000步", []EvalNumericAtom{{Topic: "走", Value: 8000, Unit: "步"}}},
		{"逗号式假设整句剔除", "user: 如果明天下雨，我就不跑10公里", nil},
		{"逗号式if-would整句剔除", "user: if I trained harder, I would run 8000 steps", nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ParseEvalNumericAtoms(tc.stmt)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseEvalNumericAtoms(%q) = %+v, want %+v", tc.stmt, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("atom[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestEvalNumSeriesRollup pins the rolling counters + canonical digest line
// round-trip：渲染出的摘要行必须能被 ParseEvalSeriesDigest 原样解析回去
// （写侧下次滚动要读旧行）。趋势按 (last-first)/天数/|mean| 线性外推。
func TestEvalNumSeriesRollup(t *testing.T) {
	t.Parallel()
	var s EvalNumSeries
	s.Topic = "步数"
	s.Add(4100, "步", "2025-05-01")
	s.Add(3200, "步", "2025-05-03")
	s.Add(5600, "步", "2025-05-05")
	s.Add(1800, "步", "2025-05-07")
	if s.Count != 4 || s.Min != 1800 || s.Max != 5600 || s.Last != 1800 || s.LastUnit != "步" {
		t.Fatalf("counters wrong: %+v", s)
	}
	if mean := s.Mean(); mean < 3674.9 || mean > 3675.1 {
		t.Fatalf("Mean() = %v, want ~3675", mean)
	}
	digest := s.RenderEvalSeriesDigest()
	back, ok := ParseEvalSeriesDigest(digest)
	if !ok {
		t.Fatalf("digest does not round-trip: %q", digest)
	}
	if back.Count != s.Count || back.Last != s.Last || back.FirstDay != s.FirstDay || back.LastDay != s.LastDay || back.FirstValue != s.FirstValue {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, s)
	}
	// AMC P3 评审 C3：sum 字段精确回合——再滚动零漂移（mean 只 %.1f 时可
	// 不行，逐轮回滚 → sum≈round(mean)×count 累计偏差）。
	if back.Sum != s.Sum {
		t.Fatalf("Sum must round-trip exactly: got %v, want %v (digest %q)", back.Sum, s.Sum, digest)
	}
	// AMC P3 评审 C5：canonical 识别是整行匹配，不是裸前缀——
	// 「[series-digest: 步数]」这种残缺/注入口吻不得当作派生行。
	if !IsEvalSeriesDigestStatement(digest) {
		t.Fatalf("rendered digest must be canonical: %q", digest)
	}
	if IsEvalSeriesDigestStatement("[series-digest: 步数]") || IsEvalSeriesDigestStatement("[series-digest:") {
		t.Fatal("bare prefix without canonical field layout must not be a digest")
	}
	// AMC P3 评审 C4：count≥2 才配置顶；单观测 digest 复述原子本身，是噪声。
	if !IsEvalSeriesDigestPinworthy(digest) {
		t.Fatal("multi-observation digest must be pinworthy")
	}
	// C3 回退路径：老行无 sum 字段 → Sum=mean×count（3675.0×4=14700 精确）。
	legacy := "[series-digest: 步数] count=4 last=1800步 mean=3675.0步 min=1800步 max=5600步 firstValue=4100步 firstDay=2025-05-01 lastDay=2025-05-07"
	lb, ok := ParseEvalSeriesDigest(legacy)
	if !ok || lb.Sum != 14700 {
		t.Fatalf("legacy no-sum line must fall back to mean*count: ok=%v sum=%v", ok, lb.Sum)
	}
	if !IsEvalSeriesDigestStatement(legacy) {
		t.Fatal("legacy no-sum line must still be canonical")
	}
	// 趋势文本必须出现（上升/下降/平稳 与 /day 后缀 或 CN 后缀），供 judge 直接引用。
	if got := s.TrendText(); got == "" {
		t.Fatal("TrendText must not be empty for a multi-point series")
	}
	// 下降序列
	var d EvalNumSeries
	d.Add(90, "kg", "2025-04-01")
	d.Add(85, "kg", "2025-05-01")
	if ttxt := d.TrendText(); ttxt == "" {
		t.Fatal("descending series must render a trend")
	}
	// 单点无趋势；count=1 digest 不配置顶（评审 C4）。
	var one EvalNumSeries
	one.Add(3200, "步", "2025-05-07")
	one.Topic = "步数"
	if got := one.TrendText(); got != "" {
		t.Fatalf("single-point series must have no trend, got %q", got)
	}
	if IsEvalSeriesDigestPinworthy(one.RenderEvalSeriesDigest()) {
		t.Fatal("single-observation digest must not be pinworthy")
	}
	// 非摘要行解析失败
	if _, ok := ParseEvalSeriesDigest("[2025-05-07 · sess-x] user: 我喜欢红色"); ok {
		t.Fatal("non-digest statement must not parse")
	}
	// AMC P3 评审 round-3 F1：exponent 记数值的完整回合——formatEvalNumValue
	// ('g') 对极端量级吐 %e，regex 所有值字段必须同行放行；否则 last/min/
	// max/firstValue 静默解析成 1（单位吞 e-05 尾巴），腐坏随轮转永驻。
	var es EvalNumSeries
	es.Topic = "浓度"
	es.Add(1e-05, "g", "2026-01-01")
	es.Add(5, "g", "2026-01-02")
	eLine := es.RenderEvalSeriesDigest()
	eb, ok := ParseEvalSeriesDigest(eLine)
	if !ok {
		t.Fatalf("exponent digest must parse: %q", eLine)
	}
	if eb.Min != 1e-05 || eb.FirstValue != 1e-05 || eb.Last != 5 || eb.Sum != es.Sum || eb.LastUnit != "g" {
		t.Fatalf("exponent round-trip corrupted: %+v (line %q)", eb, eLine)
	}
	if !IsEvalSeriesDigestStatement(eLine) {
		t.Fatalf("exponent digest must be canonical: %q", eLine)
	}
	var big EvalNumSeries
	big.Topic = "tokens"
	big.Add(2e6, "", "2026-01-01")
	big.Add(2e20, "", "2026-01-03")
	bLine := big.RenderEvalSeriesDigest()
	bb, ok := ParseEvalSeriesDigest(bLine)
	if !ok || bb.Min != 2e6 || bb.Max != 2e20 || bb.Sum != big.Sum {
		t.Fatalf("large-magnitude round-trip corrupted: ok=%v %+v (line %q)", ok, bb, bLine)
	}
	// round-3b：e/E 起头单位不得被宽松值类吞食——「5eV」必须解析为 5+eV
	// （宽松类贪吃成 "5e"→ParseFloat 归零，腐坏渲染单位）。
	var ev EvalNumSeries
	ev.Topic = "能量"
	ev.Add(5, "eV", "2026-01-01")
	ev.Add(7, "eV", "2026-01-02")
	evLine := ev.RenderEvalSeriesDigest()
	evb, ok := ParseEvalSeriesDigest(evLine)
	if !ok || evb.Last != 7 || evb.Min != 5 || evb.LastUnit != "eV" || evb.Sum != 12 {
		t.Fatalf("e-leading unit round-trip corrupted: ok=%v %+v (line %q)", ok, evb, evLine)
	}
	if !IsEvalSeriesDigestStatement(evLine) {
		t.Fatalf("eV digest must be canonical: %q", evLine)
	}
	// plain 小数值不受值类变更影响（decimal 依然精确）。
	var plain EvalNumSeries
	plain.Topic = "步数"
	plain.Add(3200, "步", "2026-01-01")
	plain.Add(4500, "步", "2026-01-02")
	pLine := plain.RenderEvalSeriesDigest()
	pb, ok := ParseEvalSeriesDigest(pLine)
	if !ok || pb.Last != 4500 || pb.Min != 3200 || pb.LastUnit != "步" || pb.Sum != 7700 {
		t.Fatalf("plain round-trip regressed: %+v (line %q)", pb, pLine)
	}
}
