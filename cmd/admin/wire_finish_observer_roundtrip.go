package main

// wire_finish_observer_roundtrip.go —— Q5 批 D3-f 对抗评审 C3 修复批：
// graph 链/builder 两处 rtTrip 字面量的共享构造（各在主本 wire.go 与手镜
// wire_gen.go 一处，共四调用点）。字面量内联态的失效面：删掉
// OnAbnormalFinish 键值（或双 file 手镜漂移漏一）编译与全部测试仍绿、
// 生产 llm_abnormal_finish 观测静默全灭——本会话 D2-f C4 同型教训
// （wireinject 主本漂移 = plain regen 静默丢接线）。提取为唯一穿透函数
// 并钉板断言（wire_finish_observer_roundtrip_test.go），漂移即红。
//
// 两个超时面是存量差异（graph 链用 provider.LLMLeakGuardTimeout、builder
// 用 120s），参数原样承接，不做顺手归一。

import (
	"net/http"
	"time"

	"aranea-agents/internal/provider"
)

// newObservedFinishRoundTrip 构造挂 finish_reason/EOF 观测钩子的共享
// RoundTrip。hooks 由 provideLLMFinishObserverHooks（wire 单例）供给；
// 零钩子时 OnAbnormalFinish 为 nil，观测层按设计 pass-through。
func newObservedFinishRoundTrip(timeout time.Duration, hooks provider.FinishObserverHooks) *provider.RoundTrip {
	return &provider.RoundTrip{
		HTTP:             &http.Client{Timeout: timeout},
		OnAbnormalFinish: hooks.OnAbnormalFinish,
	}
}
