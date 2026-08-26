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
		// ADR-79-V V2（2026-08-26）：含任务动作词的轮次不得判为事实查询——
		// 以下用例在 V2 前因子串命中「的天气」被误判（复合任务被免除规划义务）。
		"核对昨天的天气数据并生成巡检报告",
		"查一下明天的天气然后写成日报",
	}
	for _, s := range no {
		if LooksLikeFactQuery(s) {
			t.Errorf("%q should not be a fact query", s)
		}
	}
}
