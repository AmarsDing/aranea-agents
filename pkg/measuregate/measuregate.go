// Package measuregate 提供「有界测量 + 三态裁决」公共件（M19，orca
// worktree-base-divergence measureGate 同款）：把调用方上下文与单次端到端
// deadline 组合，把「测量型操作」的结果归为三态之一——
//
//	Within   在预算内完成（正向证据，测量结论可信）
//	Exceeded 确定超出预算（反向证据，例如分歧度超过阈值上限）
//	Unknown  无法判定（底层错误、父上下文先行取消等歧义情况）
//
// 使用约定（写进调用方的契约注释）：只有 Within 才被当作可信结论消费；
// Unknown 一律走保守分支（拒复用/不击杀/回冷路径），绝不可坍缩为 Exceeded
// 或 Within——「无法判定」是一等公民结果。
package measuregate

import (
	"context"
	"errors"
	"time"
)

// ErrBudgetExceeded 是测量预算耗尽的哨兵原因（context.Cause 可比判）。
var ErrBudgetExceeded = errors.New("measuregate: budget exceeded")

// Verdict 是一次有界测量的三态裁决。
type Verdict int

const (
	// Within：fn 在预算内正常完成。
	Within Verdict = iota
	// Exceeded：fn 未能在预算内完成（确定的预算耗尽）。
	Exceeded
	// Unknown：无法判定（fn 返回了非预算原因的错误，或父上下文先行取消）。
	Unknown
)

func (v Verdict) String() string {
	switch v {
	case Within:
		return "within"
	case Exceeded:
		return "exceeded"
	default:
		return "unknown"
	}
}

// Measure 在 ctx 之上叠加单次端到端预算 budget 执行 fn，并给出三态裁决：
//
//	fn 返回 nil                                   → Within
//	fn 因本预算耗尽而中断（context.Cause 命中哨兵） → Exceeded
//	其它一切（fn 自身报错、父 ctx 取消、budget<=0…）→ Unknown
//
// 「父 ctx 取消」与「本次预算耗尽」无法区分时一律 Unknown：调用方在保守分支里
// 重试或走冷路径，比在错的方向上下结论安全。
func Measure(ctx context.Context, budget time.Duration, fn func(ctx context.Context) error) Verdict {
	if fn == nil || budget <= 0 || ctx == nil {
		return Unknown
	}
	mctx, cancel := context.WithTimeoutCause(ctx, budget, ErrBudgetExceeded)
	defer cancel()
	if err := fn(mctx); err != nil {
		if errors.Is(err, ErrBudgetExceeded) {
			return Exceeded
		}
		// fn 可能把 ctxerr 原样透传，也可能自行包装；只要预算哨兵确为本调用
		// 链上的终止原因，仍判 Exceeded。
		if cause := context.Cause(mctx); errors.Is(cause, ErrBudgetExceeded) && mctx.Err() != nil {
			return Exceeded
		}
		return Unknown
	}
	return Within
}
