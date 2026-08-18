package intent

import "strings"

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
func SkipForDirectReply(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	if LooksLikeUnderspecifiedTask(t) {
		return false
	}
	for _, p := range directReplyPatterns {
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
