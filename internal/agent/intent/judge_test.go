package intent

import "testing"

func TestLooksLikeTrailingQuestion(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  bool
	}{
		{"empty", "", false},
		{"plain statement", "已完成分析，结果如上。", false},
		{"trailing half-width question", "我需要您补充股票代码或交易所?", true},
		{"trailing full-width question", "我需要您补充：股票代码或交易所？", true},
		{"markdown bold suffix", "请问您指的是哪家公司？**", true},
		{"closing bracket after question", "您希望选哪个方案（A 或 B）?）", true},
		{"quote after question", "您指的是哪家公司？”", true},
		{"question in middle with period ending", "您指的是哪家公司？以下是三家候选的介绍。", true},    // 预筛放行，由 LLM 判定排除
		{"question in middle with ellipsis ending", "您指的是哪家公司？以下是三家候选的介绍……", true}, // 同上
		{"multiple questions with period ending", "请提供：1. 股票代码？2. 时间范围？谢谢。", true},
		{"polite offer still matches prefilter", "需要我继续吗？", true}, // 预筛放行，由 LLM 判定排除
		{"trailing whitespace/newlines", "请选择时间范围？\n\n  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeTrailingQuestion(tt.reply); got != tt.want {
				t.Errorf("LooksLikeTrailingQuestion(%q) = %v, want %v", tt.reply, got, tt.want)
			}
		})
	}
}

// TestLooksLikeTrailingQuestionImperative 覆盖祈使式阻断提问（无 ?/？）的预筛。
// 正例取自 round-11 生产复测（output/live-44-functional-test/04-部署复测.md
// 第十一轮）本地 qwen 模型的真实反问措辞；反例为不得误放的陈述/礼貌收尾。
func TestLooksLikeTrailingQuestionImperative(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  bool
	}{
		// ── 生产实句（round-11，必须命中）──
		{"prod tell-me-which trailing colon", "请告诉我你想重复哪一项：", true},
		{"prod please-supplement inline list", "请补充：1. 股票代码 2. 时间范围", true},
		{"prod reply-me-a-number", "回我一个编号或名字就行", true},
		{"prod provide-original-or-specify", "请提供需要重复的原文，或说明需要重复的是哪一类内容。", true},
		// ── 祈使式阻断提问的常见变体（必须命中）──
		{"provide info list lead-in", "请提供以下信息：\n1. 股票代码\n2. 时间范围", true},
		{"provide info list blank line before list", "请提供以下信息：\n\n1. 股票代码\n2. 时间范围", true},
		{"choose with dash option list", "请选择一项：\n- 重复第一句\n- 重复第二句", true},
		{"select entry with parenthetical", "请选择要重复的条目（回我编号即可）：", true},
		{"ni-xiang direct ask", "你想重复哪一项：", true},
		{"mafan confirm", "麻烦确认一下要补充的内容：", true},
		{"canonical need-supplement without question mark", "我需要您补充：股票代码或交易所", true},
		{"results paragraph then imperative tail line", "已找到三项历史记录。\n请告诉我你想重复哪一项：", true},
		{"results then imperative same line", "结果如上。请告诉我你想深入哪一项：", true},
		// ── 误放防线（不得命中）──
		{"informational colon list", "以下是结果：\n1. ALPHA\n2. BETA", false},
		{"informational inline colon", "结果如下：青岛是山东省地级市。", false},
		{"statement with colon", "已完成分析，结论如下：一切正常。", false},
		{"imperative mid-text but statement tail", "请确认方案。以下按方案 A 执行，结果：成功。", false},
		{"polite offer ru-xu", "如需其他格式，请告诉我。", false},
		{"polite offer ru-guo suishi", "如果你还想深入某个部分，随时告诉我。", false},
		{"polite offer xuyao-dehua", "已生成三种方案，默认采用 A。需要调整的话请告诉我。", false},
		{"delivered default choice no request", "默认已为您选择 A 方案，可直接使用。", false},
		{"notification not request", "特此告知：维护窗口已排期。", false},
		{"bare list only", "1. 已完成 ALPHA\n2. 已完成 BETA", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeTrailingQuestion(tt.reply); got != tt.want {
				t.Errorf("LooksLikeTrailingQuestion(%q) = %v, want %v", tt.reply, got, tt.want)
			}
		})
	}
}
