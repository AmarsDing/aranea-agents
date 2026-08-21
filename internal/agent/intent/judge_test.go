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
		{"question in middle with period ending", "您指的是哪家公司？以下是三家候选的介绍。", true},   // 预筛放行，由 LLM 判定排除
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
