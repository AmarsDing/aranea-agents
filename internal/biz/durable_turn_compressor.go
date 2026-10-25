package biz

import "context"

// DurableTurnCompressor runs synchronous context compaction before durable resume (CH-BOR-14).
type DurableTurnCompressor interface {
	BeforeDurableTurn(ctx context.Context, sessionID string, agent Agent) error
}

// DurablePrecheckVerdict 是 durable 压缩预检的判级（D4-b，deferred 战役
// 2026-10-25；F1 登记 feature-flags-registry §1「D4 估算/预检就绪 flip」，
// 就绪不翻）。判读基于上一轮权威读数（sess.ContextUsedTokens），零查询。
type DurablePrecheckVerdict string

const (
	// DurablePrecheckBelowSoft 未越软带（used < soft）：无须压缩动作。
	DurablePrecheckBelowSoft DurablePrecheckVerdict = "below_soft"
	// DurablePrecheckSoftBand 软带（soft ≤ used < hard）：软/硬带判级与
	// runCompress 的触线口径逐点对齐（adaptive buffer 只读内存态）。
	DurablePrecheckSoftBand DurablePrecheckVerdict = "soft_band"
	// DurablePrecheckHardBand 硬带（used ≥ hard）。
	DurablePrecheckHardBand DurablePrecheckVerdict = "hard_band"
)

// DurablePrecheckReport 承载一次预检的全量观测字段（seat①
// session.compress.precheck 步骤日志原样透传）。
type DurablePrecheckReport struct {
	Verdict       DurablePrecheckVerdict
	UsedTokens    int
	SoftTokens    int
	HardTokens    int
	ContextWindow int
}

// AtOrAboveSoft reports whether the verdict sits in the soft or hard band —
// the mode=on trigger predicate（正常触发语义 AfterNativeTurn）。
func (r DurablePrecheckReport) AtOrAboveSoft() bool {
	return r.Verdict == DurablePrecheckSoftBand || r.Verdict == DurablePrecheckHardBand
}

// DurableTurnPrecheck 是 durable 压缩预检窄口（D4-b）：由 session.Compressor
// 实现；admitTurn 与 wire 断言此接口（TurnDeps.Compress 的静态口
// NativeTurnCompressor 保持不动，本口按可用性断言）。
type DurableTurnPrecheck interface {
	// PrecheckVerdict 纯函数式判级：复用 compressTriggerThresholds，不上
	// 库、无写副作用（adaptive buffer 仅读内存态）。
	PrecheckVerdict(sess Session, agent Agent) DurablePrecheckReport
	// AfterNativeTurn 正常触发语义（flight-guarded、非 forced、不绕
	// suppress），与框架原生轮后异步压缩触发完全一致。
	AfterNativeTurn(ctx context.Context, sessionID string, agent Agent)
}
