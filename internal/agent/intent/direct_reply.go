package intent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// directReplyPatterns are user turns that should answer immediately without an
// extra intent-classification LLM. Underspecified task phrases are excluded by
// LooksLikeUnderspecifiedTask so the clarification gate still sees "帮我做个应用".
var directReplyPatterns = []string{
	"不要调用工具",
	"不要调用任何工具",
	"do not use tools",
	"don't use tools",
	"don't call tools",
	"do not call tools",
	"介绍你自己",
	"介绍一下你自己",
	"介绍自己",
	"你是谁",
	"who are you",
	"introduce yourself",
	"现在几点",
	"几点了",
	"what time",
	"current time",
	"请记住",
	"记住：",
	"记住:",
	"remember that",
	"remember:",
	"刚才我说",
	"刚才让你",
	"我刚才",
}

// underspecifiedTaskPatterns keep the intent pass on short ambiguous tasks
// (2026-07-23 clarification gate). "能帮我做什么" must not match.
var underspecifiedTaskPatterns = []string{
	"帮我做个",
	"帮我做一个",
	"帮我写",
	"写一个",
	"做一个应用",
	"做个应用",
	"实现一个",
	"开发一个",
	"创建一个",
	"build me an app",
	"create an application",
	"make an app",
}

// SkipForDirectReply reports whether this user turn is chit-chat / identity /
// remember / clock-query that must not pay an extra intent LLM.
//
// ADR-79-V V2（2026-08-26）：分类只可增加义务、不可免除义务。含任务动作
// 词的轮次不得命中直接回复快路径——「请记住这个拓扑，然后排查核心交换机」
// 子串命中「请记住」但它是复合任务，skip 会导致组织路由失效（P-INTENT-SKIP）。
func SkipForDirectReply(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	if LooksLikeUnderspecifiedTask(t) {
		return false
	}
	if biz.HasTaskActionSignal(t) {
		return false
	}
	if looksLikeBareGreeting(t) {
		return true
	}
	if biz.LooksLikeFactQuery(t) {
		return true
	}
	for _, p := range directReplyPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// looksLikeBareGreeting reports a turn that is only a hello — not "你好，帮我写周报".
// Contains("你好") would skip almost every polite task opener.
func looksLikeBareGreeting(t string) bool {
	t = strings.Trim(t, "。.!！?？~～、,， ")
	switch t {
	case "你好", "您好", "嗨", "哈喽", "早上好", "晚上好", "下午好", "在吗", "在么", "在不在",
		"hi", "hello", "hey", "yo", "good morning", "good evening", "good afternoon":
		return true
	}
	return false
}

// rememberRequestPatterns are explicit memory writes (IDENTITY.md).
var rememberRequestPatterns = []string{
	"请记住",
	"记住：",
	"记住:",
	"以后都",
	"我的习惯是",
	"不要再",
	"remember that",
	"remember:",
}

// LooksLikeRememberRequest reports a user turn that must persist a preference
// even if the LLM never called memory_remember.
func LooksLikeRememberRequest(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	for _, p := range rememberRequestPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// LooksLikeUnderspecifiedTask reports short ambiguous task asks that still
// need the intent pass so the clarification gate can run.
func LooksLikeUnderspecifiedTask(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	for _, p := range underspecifiedTaskPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}
