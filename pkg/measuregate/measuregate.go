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

// ErrBudgetExceeded 是测量预算耗尽的哨兵原因：每次 Measure 调用生成一个实现
// Is(ErrBudgetExceeded) 的独立实例作为 context 终止 cause，指针身份=「本调用的
// 预算耗尽」，以此与父 ctx 传播的取消/嵌套外层的预算哨兵区分（对抗评审 C10/C11）。
var ErrBudgetExceeded = errors.New("measuregate: budget exceeded")

// budgetCause 单次调用的预算终止原因实例（不可导出，防止外部伪造身份）。
type budgetCause struct{ budget time.Duration }

func (c *budgetCause) Error() string { return ErrBudgetExceeded.Error() }

// Is 让 errors.Is(err, ErrBudgetExceeded) 对对外的错误语义保持成立；
// Measure 内部裁决用的是指针身份（errors.Is(err, cause)），不受此方法影响。
func (c *budgetCause) Is(target error) bool { return target == ErrBudgetExceeded }

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
//	fn 返回 nil                                              → Within
//	fn 的错误链携带【本调用】的预算哨兵实例（透传 Cause/包装）  → Exceeded
//	fn 透传 ctx.Err()（DeadlineExceeded）且确为本预算终止       → Exceeded
//	其它一切（fn 自身报错、父 ctx 取消/超时、嵌套外层预算、budget<=0…）→ Unknown
//
// 「父 ctx 取消」或「嵌套 Measure 的外层预算」与「本次预算耗尽」无法区分时一律
// Unknown：哨兵按调用实例判身份，外层预算绝不会被误计入内层（C10）；fn 在预算
// 边界返回自身业务错误时同样归 Unknown（歧义不坍缩，C11）。调用方在保守分支里
// 重试或走冷路径，比在错的方向上下结论安全。
func Measure(ctx context.Context, budget time.Duration, fn func(ctx context.Context) error) Verdict {
	if fn == nil || budget <= 0 || ctx == nil {
		return Unknown
	}
	cause := &budgetCause{budget: budget}
	mctx, cancel := context.WithTimeoutCause(ctx, budget, cause)
	defer cancel()
	err := fn(mctx)
	if err == nil {
		return Within
	}
	if errors.Is(err, cause) {
		return Exceeded
	}
	if mctx.Err() != nil && context.Cause(mctx) == cause && errors.Is(err, context.DeadlineExceeded) {
		// fn 只看 ctx.Err()（非 Cause）而终止确属本预算 → 仍判 Exceeded；
		// 父取消/父超时（Cause 非本实例）与 fn 自身业务错误都不入口。
		return Exceeded
	}
	return Unknown
}
