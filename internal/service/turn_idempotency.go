package service

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrTurnDuplicate 表示同一 (session_id, request_id) 的消息已受理过
// （in-flight 或 TTL 窗口内已完成）。重复提交不产生第二条用户消息/第二轮
// turn；各传输边界将其映射为「静默成功」（WS 忽略 / HTTP 空 ACK），客户端
// 经 WS 事件或历史快照拿到首次提交的结果。
var ErrTurnDuplicate = errors.New("turn duplicate submission")

func isTurnDuplicate(err error) bool {
	return errors.Is(err, ErrTurnDuplicate)
}

// turnIdemTTL 是幂等键的保留窗口。前端重试（retryFailedMessage 复用
// pending-user-<uuid>）发生在用户可感知的分钟级窗口内；30min 覆盖
// 「刷新页面后再点重试」的长尾，又不至于让内存地图无界增长。
const turnIdemTTL = 30 * time.Minute

// turnIdemSweepEvery 每积累多少次 claim 触发一次惰性过期清扫（摊销成本，
// 避免独立 janitor goroutine）。
const turnIdemSweepEvery = 128

// turnIdemRegistry 是提交幂等键的内存登记表（P3，2026-08-20）。
//
// 语义：
//   - claim：首次提交返回 true 并占位；同键在 TTL 窗口内重复提交返回 false。
//   - release：执行失败时撤销占位，允许客户端重试（成功的 turn 不撤销——
//     重试必须被去重直到 TTL 过期）。
//
// 进程内内存实现：重启后窗口清空，极端场景（重启瞬间客户端自动重发）可能
// 重复执行一次。WS 路径的 request_id 已持久化到 error envelope 做关联，但
// messages 表无 request_id 列，DB 级去重留作后续增强（成本/收益不匹配：
// 重启+自动重发同时命中的概率极低，且前端重试均为用户手动触发）。
type turnIdemRegistry struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	claims int
}

func newTurnIdemRegistry() *turnIdemRegistry {
	return &turnIdemRegistry{seen: make(map[string]time.Time)}
}

func turnIdemKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

// claim 登记 (sessionID, requestID)；首次返回 true，窗口内重复返回 false。
// 空 requestID 不参与去重（无客户端键的入口），直接返回 true。
func (r *turnIdemRegistry) claim(sessionID, requestID string) bool {
	if r == nil {
		return true
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claims++
	if r.claims%turnIdemSweepEvery == 0 {
		r.sweepLocked(now)
	}
	key := turnIdemKey(sessionID, requestID)
	if ts, ok := r.seen[key]; ok && now.Sub(ts) < turnIdemTTL {
		return false
	}
	r.seen[key] = now
	return true
}

// release 撤销占位（仅用于执行失败路径，使重试可重新受理）。
func (r *turnIdemRegistry) release(sessionID, requestID string) {
	if r == nil || strings.TrimSpace(requestID) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.seen, turnIdemKey(sessionID, requestID))
}

func (r *turnIdemRegistry) sweepLocked(now time.Time) {
	for k, ts := range r.seen {
		if now.Sub(ts) >= turnIdemTTL {
			delete(r.seen, k)
		}
	}
}

// claimTurnIdem 在 turn 执行入口登记幂等键；重复提交返回 false。
// requestID 为空（channel/cron/A2A 等无客户端键入口）时不参与去重。
func (o *ChatOrchestrator) claimTurnIdem(sessionID, requestID string) bool {
	if o == nil {
		return true
	}
	return o.turnIdem.claim(sessionID, requestID)
}

// releaseTurnIdem 撤销占位，仅用于执行失败路径（重试须可重新受理）；
// 成功/排队的 turn 不撤销，窗口内重试一律去重。
func (o *ChatOrchestrator) releaseTurnIdem(sessionID, requestID string) {
	if o == nil {
		return
	}
	o.turnIdem.release(sessionID, requestID)
}
