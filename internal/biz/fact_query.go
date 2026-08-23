package biz

import "strings"

// LooksLikeFactQuery reports a light-gear fact lookup the Spirit should
// answer with datetime / web search — not plan_and_execute.
func LooksLikeFactQuery(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	for _, p := range factQueryPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

var factQueryPatterns = []string{
	"天气怎么样", "天气如何", "查天气", "明天天气", "今天天气", "的天气",
	"weather", "forecast", "气温",
	"现在几点", "几点了", "what time", "current time",
	"汇率", "exchange rate",
	"今日新闻", "今天新闻", "头条新闻",
}
