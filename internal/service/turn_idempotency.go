package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ErrTurnDuplicate 表示同一 (session_id, request_id) 的消息已受理过
// （in-flight 或 TTL 窗口内已完成）。重复提交不产生第二条用户消息/第二轮
// turn；各传输边界将其映射为「静默成功」（WS 忽略 / HTTP 空 ACK），客户端
// 经 WS 事件或历史快照拿到首次提交的结果。
var ErrTurnDuplicate = errors.New("turn duplicate submission")

func isTurnDuplicate(err error) bool {
	return errors.Is(err, ErrTurnDuplicate)
}

// turnIdemTTL 是内存幂等键的保留窗口。前端重试（retryFailedMessage 复用
// pending-user-<uuid>）发生在用户可感知的分钟级窗口内；30min 覆盖
// 「刷新页面后再点重试」的长尾，又不至于让内存地图无界增长。
// 持久化认领（TurnSubmitClaimer）不受此 TTL 限制，进程重启后仍去重。
const turnIdemTTL = 30 * time.Minute

// turnIdemSweepEvery 每积累多少次 claim 触发一次惰性过期清扫（摊销成本，
// 避免独立 janitor goroutine）。
const turnIdemSweepEvery = 128

// TurnSubmitClaimer is the durable claim/release port for chat submit and
// enqueue idempotency. Implemented by data.ChatTurnSubmitClaimer.
type TurnSubmitClaimer interface {
	TryClaim(ctx context.Context, sessionID, requestID string) (bool, error)
	Release(ctx context.Context, sessionID, requestID string) error
}

// turnIdemRegistry 是提交幂等键的登记表（P3，2026-08-20；2026-10-16 持久化）。
//
// 语义：
//   - claim：首次提交返回 true 并占位；同键在内存 TTL 窗口内或 DB 中已存在则返回 false。
//   - release：执行失败时撤销占位，允许客户端重试（成功的 turn 不撤销——
//     重试必须被去重直到内存 TTL 过期；持久化认领则直到显式 Release）。
//
// 双层：内存快路径 + 可选 durable claimer（chat_turn_submit_claim）。
// durable 不可用时降级为内存-only（与历史行为一致），并打 Warn。
type turnIdemRegistry struct {
	mu       sync.Mutex
	seen     map[string]time.Time
	claims   int
	durable  TurnSubmitClaimer
	lg       loggateway.Logger
}

func newTurnIdemRegistry() *turnIdemRegistry {
	return &turnIdemRegistry{seen: make(map[string]time.Time)}
}

func (r *turnIdemRegistry) setDurable(c TurnSubmitClaimer, lg loggateway.Logger) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durable = c
	r.lg = lg
}

func turnIdemKey(sessionID, requestID string) string {
	return sessionID + "\x00" + requestID
}

// claim 登记 (sessionID, requestID)；首次返回 true，窗口内/DB 重复返回 false。
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
	// Durable claim (best-effort). On conflict → duplicate even if memory was cold
	// (process restart). On error → degrade to memory-only and keep the claim.
	if r.durable != nil {
		ok, err := r.durable.TryClaim(context.Background(), sessionID, requestID)
		if err != nil {
			if r.lg != nil {
				r.lg.Warn("durable turn idem claim failed; using memory-only",
					loggateway.StepID("chat.turn_idem_durable"),
					loggateway.SessionID(sessionID),
					loggateway.Str("request_id", requestID),
					loggateway.Err(err))
			}
		} else if !ok {
			// Mirror durable hit into memory so subsequent checks are cheap.
			r.seen[key] = now
			return false
		}
	}
	r.seen[key] = now
	return true
}

// release 撤销占位（仅用于执行失败路径，使重试可重新受理）。
func (r *turnIdemRegistry) release(sessionID, requestID string) {
	if r == nil || strings.TrimSpace(requestID) == "" {
		return
	}
	requestID = strings.TrimSpace(requestID)
	r.mu.Lock()
	durable := r.durable
	lg := r.lg
	delete(r.seen, turnIdemKey(sessionID, requestID))
	r.mu.Unlock()
	if durable != nil {
		if err := durable.Release(context.Background(), sessionID, requestID); err != nil && lg != nil {
			lg.Warn("durable turn idem release failed",
				loggateway.StepID("chat.turn_idem_durable"),
				loggateway.SessionID(sessionID),
				loggateway.Str("request_id", requestID),
				loggateway.Err(err))
		}
	}
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
