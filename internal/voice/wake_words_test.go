package voice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 设计 §16.4①：同音词表句首剥离（含叠词连说 + 紧随标点/空白容错）。
func TestStripWakeWord(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantText string
		wantHit  bool
	}{
		{"单唤醒词", "小媛", "", true},
		{"唤醒词+指令（逗号）", "小媛，查天气", "查天气", true},
		{"唤醒词+指令（无标点）", "小媛查天气", "查天气", true},
		{"唤醒词+指令（英文逗号+空格）", "小媛, 查天气", "查天气", true},
		{"同音词 小圆", "小圆，明天天气怎么样", "明天天气怎么样", true},
		{"同音词 小袁", "小袁查一下邮件", "查一下邮件", true},
		{"同音词 小源", "小源，放首歌", "放首歌", true},
		{"叠词连说", "小媛小媛，查天气", "查天气", true},
		{"叠词连说无标点", "小媛小媛查天气", "查天气", true},
		{"叠词带中间停顿逗号", "小媛，小媛，查天气", "查天气", true},
		{"单唤醒带句号", "小媛。", "", true},
		{"句首空白容错", "  小媛，查天气", "查天气", true},
		{"非句首不剥离", "你觉得小媛怎么样", "你觉得小媛怎么样", false},
		{"句尾出现不剥离", "你好，小媛", "你好，小媛", false},
		{"无命中原样返回", "今天天气不错", "今天天气不错", false},
		{"空文本", "", "", false},
		{"仅标点", "，。", "，。", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotText, gotHit := StripWakeWord(c.in)
			require.Equal(t, c.wantHit, gotHit, "hit")
			require.Equal(t, c.wantText, gotText, "text")
		})
	}
}

// 设计 §16.4①：退出词整句归一化精确匹配（复用 normalizeConfirmWord 规则）。
func TestMatchExitWord(t *testing.T) {
	hit := []string{
		"休息吧", "再见", "退出", "退下吧", "不用了",
		"休息吧。", "再见！", " 退出 ", "退下吧，",
	}
	for _, s := range hit {
		require.True(t, MatchExitWord(s), "should hit: %q", s)
	}
	miss := []string{
		"", "不用了谢谢", "你休息吧", "退出模式", "好的", "查天气",
		"小媛，休息吧", // 含唤醒词前缀的整句不由本函数受理（先经 StripWakeWord）
	}
	for _, s := range miss {
		require.False(t, MatchExitWord(s), "should miss: %q", s)
	}
}
