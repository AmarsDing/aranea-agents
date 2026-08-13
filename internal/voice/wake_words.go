package voice

import "strings"

// wake_words.go — V10 唤醒词剥离与退出词匹配（设计 §16.4①）。
//
// 拦截顺序（评审确认）：唤醒词剥离 → 退出词匹配 → 确认词拦截 → Chat 管线。

// wakeWords 唤醒词同音词表：云端 ASR 可能把「小媛」识别为同音字。
var wakeWords = []string{"小媛", "小圆", "小袁", "小源", "小园", "小员"}

// exitWords 退出词表（需求 §2.12）。注意「不用了」与确认 denyWords 重叠，
// 退出词优先（设计 §16.4①：pending confirm 期间说「不用了」判为休眠）。
var exitWords = []string{"休息吧", "再见", "退出", "退下吧", "不用了"}

// wakeWordTrimSet 剥离唤醒词后紧随的分隔符（逗号/顿号/句号/空白等）。
const wakeWordTrimSet = "，。！？!?、,.;；~～… \t"

// StripWakeWord 剥离句首唤醒词（含叠词连说形态），返回净文本与是否命中。
// 非句首出现（如「你觉得小媛怎么样」）不剥离；未命中时原样返回 text。
func StripWakeWord(text string) (stripped string, matched bool) {
	s := strings.TrimSpace(text)
	for {
		rest, ok := stripOneWakeWord(s)
		if !ok {
			break
		}
		matched = true
		s = rest
	}
	if !matched {
		return text, false
	}
	return s, true
}

// stripOneWakeWord 尝试剥离一次句首唤醒词 + 紧随分隔符。
func stripOneWakeWord(s string) (string, bool) {
	for _, w := range wakeWords {
		if strings.HasPrefix(s, w) {
			rest := strings.TrimLeft(s[len(w):], wakeWordTrimSet)
			return rest, true
		}
	}
	return s, false
}

// MatchExitWord 将 ASR 终稿（已剥离唤醒词）整句匹配退出词；
// 归一化规则复用 normalizeConfirmWord（大小写/空白/首尾标点）。
func MatchExitWord(text string) bool {
	w := normalizeConfirmWord(text)
	if w == "" {
		return false
	}
	for _, e := range exitWords {
		if w == e {
			return true
		}
	}
	return false
}
