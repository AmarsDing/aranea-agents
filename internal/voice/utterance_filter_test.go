package voice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// V11-T3（设计 §17.4）：无意义终稿过滤——语气词/单字碎片/极短含混音
// 不建 Chat Turn，从源头挡住背景人声/杂音识别出的垃圾语句。
func TestFilterNoiseFinal(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		durationMs int
		wantDrop   bool
		wantReason string
	}{
		// F1：语气词表整句命中（归一化大小写/标点/空白）
		{"filler 嗯", "嗯", 1200, true, "filler_word"},
		{"filler 带标点", "嗯。", 900, true, "filler_word"},
		{"filler 啊", "啊", 500, true, "filler_word"},
		{"filler 呃", "呃", 400, true, "filler_word"},
		{"filler 哦", "哦", 600, true, "filler_word"},
		{"filler 叠音", "嗯嗯", 800, true, "filler_word"},
		{"filler 那个", "那个", 700, true, "filler_word"},
		{"filler 英文 hmm", "Hmm", 600, true, "filler_word"},

		// F2：单 rune 碎片（背景音识别出的单字）
		{"single char 是", "是", 900, true, "single_char"},
		{"标点归一后为空", "？！", 300, true, "empty"},

		// F3：极短含混音（duration>0 且 <300ms 且 ≤2 rune）
		{"too short 两字 200ms", "你好", 200, true, "too_short"},
		{"too short 边界 299ms", "在呢", 299, true, "too_short"},

		// 保留：正常语句
		{"keep 正常指令", "帮我打开微信", 1200, false, ""},
		{"keep 两字但时长充足", "你好", 800, false, ""},
		{"keep 两字 300ms 边界", "你好", 300, false, ""},
		{"keep 时长未知（0）不按 F3 丢弃", "好的", 0, false, ""},
		{"keep 语气词嵌句中不整句命中", "那个帮我查一下天气", 1500, false, ""},
		{"keep 唤醒剥离后的净文本", "查一下天气", 1000, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drop, reason := FilterNoiseFinal(tc.text, tc.durationMs)
			require.Equal(t, tc.wantDrop, drop)
			require.Equal(t, tc.wantReason, reason)
		})
	}
}

// 与确认词表的顺序契约：「嗯」是 approveWord——有待决确认时先被 confirm
// 拦截（到不了过滤器）；无待决确认时被 F1 丢弃。两表职责不冲突。
func TestFilterNoiseFinalDoesNotBreakConfirmWords(t *testing.T) {
	// 确认词本身不过滤（有待决确认时在过滤器之前已被拦截，此处仅锁死顺序假设）
	require.Equal(t, VoiceConfirmApprove, MatchVoiceConfirm("嗯"))
	drop, _ := FilterNoiseFinal("嗯", 500)
	require.True(t, drop, "无待决确认的裸「嗯」按噪声丢弃")
	// deny/approve 长词非语气词表成员，时长未知时不被 F3 误伤
	drop, _ = FilterNoiseFinal("好的", 0)
	require.False(t, drop)
}
