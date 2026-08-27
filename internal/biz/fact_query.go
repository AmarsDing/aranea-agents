package biz

import "strings"

// LooksLikeFactQuery reports a light-gear fact lookup the Spirit should
// answer with datetime / web search — not plan_and_execute.
//
// ADR-79-V V2（2026-08-26）：分类只可增加义务、不可免除义务。含任务动作
// 词的轮次（HasTaskActionSignal）一律返回 false——「核对昨天的天气数据并
// 生成巡检报告」子串命中「的天气」但它是复合任务，不得走事实查询快路径。
func LooksLikeFactQuery(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	if HasTaskActionSignal(t) {
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

// LooksLikeDirectAnswer reports a request the Spirit can answer directly in
// one pass — recommendations / explanations / opinions with no deliverable
// production （包C Q2-C1, session-eval-20260827 S11-t5 根修）。
//
// 背景：plan_and_execute 工具边界原只拦事实查询（LooksLikeFactQuery），
// 「推荐三本书」类明显直答请求被 Spirit LLM 自主送入计划工具——工具自判
// simple/direct 仍完成计划装配（计划板 + 事件写入时间线），单轮 input
// 9.6K→21.1K（+120%）。本判定供工具边界扩展拦截：命中即拒绝组队/计划
// 装配并引导直答。
//
// V2 豁免顺序与 LooksLikeFactQuery 一致：含任务动作信号（HasTaskActionSignal）
// 的轮次一律返回 false——「推荐三本微服务的书并整理成对比表格」含「整理/
// 对比」，是复合任务，不得走直答拦截。
func LooksLikeDirectAnswer(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	if HasTaskActionSignal(t) {
		return false
	}
	for _, p := range directAnswerPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// directAnswerPatterns 明显直答语义词。维护纪律：
//   - 只收"单轮口答即可满足"的语义（推荐/解释/评价/科普）；凡可能伴随
//     交付物生产的词不进表（「写」「做」「出」类已由任务信号拦截在先，
//     但「策划」「方案」类名词本身不是直答语义，也不进表）。
//   - 与 directReplyPatterns（chit-chat）互补：那边是身份/寒暄快路径，
//     本表是知识型口答请求，消费点是工具边界而非 skip 门。
var directAnswerPatterns = []string{
	"推荐", "荐书", "书单", "安利",
	"什么是", "是什么", "为什么", "啥是", "是啥",
	"怎么理解", "如何理解", "怎么看", "你觉得", "你认为", "你咋看",
	"解释一下", "说明一下", "讲讲", "科普", "举例说明", "介绍一下",
	"recommend", "explain", "what is", "why is", "why do",
}
