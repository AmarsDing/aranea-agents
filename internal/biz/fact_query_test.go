package biz

import "testing"

func TestLooksLikeFactQuery(t *testing.T) {
	yes := []string{
		"明天天气怎么样",
		"今天北京的天气",
		"what's the weather tomorrow",
		"现在几点了",
		"美元汇率",
		"今日新闻有什么",
	}
	for _, s := range yes {
		if !LooksLikeFactQuery(s) {
			t.Errorf("%q should be a fact query", s)
		}
	}
	no := []string{
		"帮我做个天气应用",
		"写一个天气预报页面",
		"请排查杭州机房告警",
		"用 Go 写 REST 接口",
		"",
	}
	for _, s := range no {
		if LooksLikeFactQuery(s) {
			t.Errorf("%q should not be a fact query", s)
		}
	}
}
