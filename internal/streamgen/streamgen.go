// Package streamgen 提供应用层 LLM 流代际计数（2026-11-03 trpc 出库后
// fork model.Response.GenerationID 的应用层替代）。
//
// 背景：fork 在活性重连（liveness guard 重发同一 Request）时给每个
// model.Response 打 GenerationID="reconnect-N"，v2 projector 据此在代际
// 切换时截断 step 已累积的半截文本（否则同一 step 上两个 attempt 的片段
// 纯 append 成整段重复）。上游 v1.11.2 的 model.Response 无扩展载体，
// 且 llmflow 的 normalizeResponseIDs 在流内强制归一 Response.ID（重连
// 后的新 attempt 会被克隆回旧 ID），不能借用 ID 通道传递代际——故改为
// 「每框架 invocation 一个代际计数器」的带外通道：
//
//   - 生产侧：provider 活性守卫在每次重连决策时 Bump（invocationID 取自
//     模型调用 ctx 中的 agent.Invocation，与事件流 ev.InvocationID 同源）。
//   - 消费侧：v2 projector 处理流式 delta 时读 Label(ev.InvocationID)，
//     语义与 fork 一致：首发 attempt 恒 ""，第 N 次重连后 "reconnect-N"。
//
// 与 fork 的差异（已知且可观测，不影响正确性收敛）：
//
//  1. fork 的代际随响应帧同行，零竞态；计数器是带外通道，若重连决策与
//     projector 处理「决策前最后一帧」发生重排，该滞留帧会被误当新代际
//     触发一次截断（随后新 attempt 正常续写覆盖）。窗口极小：stall 触发
//     的重连决策前必然经历整段静默（滞留帧早已处理完毕）；stream_error
//     触发时窗口 = 投影处理滞后 vs 末帧与错误帧的到达间隔（同一管道内
//     通常为亚毫秒级）。截断动作有 projector.generation_reset Warn 日志
//     可观测。
//  2. 上游 graphagent 不为图节点创建子 invocation，并发图分支共享同一
//     invocation 时计数器合流——A 分支重连可能误截 B 分支的并发流。
//     当前编排不存在并发 LLM 图分支（team member 各有独立 turn/
//     invocation）；未来引入并发图分支时需按分支细分计数键。
package streamgen

import (
	"strconv"
	"sync"
	"time"
)

// entryTTL 是计数器条目的惰性过期时间。条目只在 Bump 时创建，且每次 Bump
// 顺带清扫过期条目——无后台 goroutine、无泄漏：一轮 turn 的生命周期以
// 分钟计（首字节/stall 守卫封顶静默时长），远小于 TTL。条目只覆盖「真实
// 发生过重连」的 invocation，规模极小。
const entryTTL = 2 * time.Hour

type entry struct {
	gen     int64
	touched time.Time
}

var (
	mu      sync.Mutex
	entries = map[string]*entry{}
)

// Bump 将 invocationID 的代际计数 +1 并返回新代际标签（"reconnect-N"）。
// invocationID 为空时返回 ""——无 invocation 的辅助调用链不参与代际机制。
func Bump(invocationID string) string {
	if invocationID == "" {
		return ""
	}
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	for id, e := range entries {
		if now.Sub(e.touched) > entryTTL {
			delete(entries, id)
		}
	}
	e := entries[invocationID]
	if e == nil {
		e = &entry{}
		entries[invocationID] = e
	}
	e.gen++
	e.touched = now
	return labelOf(e.gen)
}

// Label 返回 invocationID 的当前代际标签：从未重连（无条目/计数 0）时恒
// ""，第 N 次重连后为 "reconnect-N"。只读路径，不创建条目。
func Label(invocationID string) string {
	if invocationID == "" {
		return ""
	}
	mu.Lock()
	e := entries[invocationID]
	mu.Unlock()
	if e == nil {
		return ""
	}
	return labelOf(e.gen)
}

func labelOf(gen int64) string {
	if gen <= 0 {
		return ""
	}
	return "reconnect-" + strconv.FormatInt(gen, 10)
}
