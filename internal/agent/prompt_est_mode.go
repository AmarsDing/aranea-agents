package agent

import (
	"sync/atomic"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// D4-a：估算档位 flip（deferred 战役 2026-10-25；F1 登记
// feature-flags-registry §1「D4 估算/预检就绪 flip」，成果设计 87 三件套）。
//
// 档位真源在 internal/conf.PromptEstMode；agent 包禁依赖 internal/conf
// （红线），档位经 wire 侧 SetPromptEstModeProvider 注入闭包实时读取
// （改 env 无须重建 builder；与工具装配期快照 ag.Settings 不同，档位是
// 进程级全局证据通道，实时读属刻意）。未注入时缺省 shadow：消费值=
// legacy（零行为变化）+ 每 prompt 级调用一条 prompt.est_shadow 对照日志
// （P0-3① 契约冻结：step_id 固定、字段 legacy_est/segmented_est/delta、
// message 级不落影子）。
//
// 翻牌成对：prompt 级与 message 级同开关一起切。segmented 档偏差记档
// （87 design）：Σ per-message ceil 恒 ≥ 整片 ceil，差 ≤N-1——保守方向
// 偏差（宁可多估不漏估），翻转前已对人类/对比脚本声明。

type estMode string

const (
	estModeLegacy    estMode = "legacy"
	estModeShadow    estMode = "shadow"
	estModeSegmented estMode = "segmented"
)

// promptEstModeProvider 是档位读取闭包（wire 注入 conf 访问器翻译后的
// 值函数）。atomic.Value 即时生效；Store 的 concrete type 恒为 func() string。
var promptEstModeProvider atomic.Value // stores func() string

// estModeWarnOnce 保证坏档位只 Warn 一次（防逐调用刷日志）。仅在有
// Logger 的调用点消费 once 标记——message 级路径无 Logger，静默回落，
// 不提前吞掉告警机会。
var estModeWarnOnce atomic.Bool

// SetPromptEstModeProvider 注入档位读取闭包（wire 侧把 conf.PromptEstMode
// 翻译为 provider）。nil 回落 shadow 缺省；可重复调用（后值覆盖前值），
// 供测试与重载。
func SetPromptEstModeProvider(fn func() string) {
	if fn == nil {
		promptEstModeProvider.Store(func() string { return string(estModeShadow) })
		return
	}
	promptEstModeProvider.Store(fn)
}

// currentEstMode 读取并归一档位：空值/未知值回落 shadow；未知值且有
// Logger 时 once-Warn。真值集合 {legacy, shadow, segmented} 与
// conf.PromptEstMode 严格对齐（conf 侧已归一坏值回落 shadow，本层归一是
// 双保险+测试直注面）。
func currentEstMode(lg loggateway.Logger) estMode {
	raw := string(estModeShadow)
	if fn, ok := promptEstModeProvider.Load().(func() string); ok && fn != nil {
		raw = fn()
	}
	switch estMode(raw) {
	case estModeLegacy, estModeShadow, estModeSegmented:
		return estMode(raw)
	default:
		if lg != nil && estModeWarnOnce.CompareAndSwap(false, true) {
			lg.Warn("invalid prompt est mode, falling back to shadow",
				loggateway.StepID("agent.est_mode"),
				loggateway.Any("est_mode", raw))
		}
		return estModeShadow
	}
}

// sharedEstCal 是 segmented 档共享校准器（rune 类价格 grounded 缺省）。
// RecordAuthoritative 锚点校准沿用 A6 既有口；本批不新增锚点接线。
var sharedEstCal = NewSegmentedCalibrator(SegmentedEstConfig{})

// promptEstTokensGated 按当前档位估算整片 messages：
//   - legacy：promptEstTokens（旧实现，行为同 A6 之前）。
//   - shadow（缺省）：promptEstTokensShadow——消费 legacy 值，单条对照
//     日志（每条 prompt 级调用恰一条）。
//   - segmented：sharedEstCal.Estimate。
func promptEstTokensGated(msgs []trpcmodel.Message, lg loggateway.Logger) int {
	switch currentEstMode(lg) {
	case estModeLegacy:
		return promptEstTokens(msgs)
	case estModeSegmented:
		return sharedEstCal.Estimate(msgs)
	default:
		return promptEstTokensShadow(msgs, sharedEstCal, lg)
	}
}

// messageEstTokensGated 按当前档位估算单条消息：
//   - legacy / shadow：messageEstTokens（shadow 档 message 级刻意不落影子
//     ——防逐消息循环伪造整片调用毒化对比脚本，P0-3① 冻结契约）。
//   - segmented：逐消息 rune 类分段 ceil。Σ per-message ceil ≥ 整片 ceil，
//     差 ≤N-1（保守方向偏差，记档 87 design）。
func messageEstTokensGated(m trpcmodel.Message) int {
	if currentEstMode(nil) == estModeSegmented {
		return estimateTokensSegmentedCounts(runeClassCountsMessage(m), sharedEstCal.Config())
	}
	return messageEstTokens(m)
}

// ---------------------------------------------------------------------------
// D4-b：durable 压缩预检档位（conf.DurablePrecheckMode 的 wire 注入面）
// ---------------------------------------------------------------------------
//
// 档位真源在 internal/conf.DurablePrecheckMode（off/observe/on，缺省
// observe）。agent 侧消费座席=p8 装配对照落步（assembly_budget.go
// emitAssemblyCompressPrecheck）；service 侧座席①（admitTurn）直读 conf
// 不走本 provider。

type precheckMode string

const (
	precheckModeOff     precheckMode = "off"
	precheckModeObserve precheckMode = "observe"
	precheckModeOn      precheckMode = "on"
)

// durablePrecheckModeProvider 同 promptEstModeProvider 形状。conf 访问器
// 已归一坏值回落 observe 且在 wire 侧起机 Warn，本层归一纯双保险。
var durablePrecheckModeProvider atomic.Value // stores func() string

// SetDurablePrecheckModeProvider 注入 durable 预检档位读取闭包（wire 侧
// 翻译 conf.DurablePrecheckMode）。nil 回落 observe 缺省；可重复调用。
func SetDurablePrecheckModeProvider(fn func() string) {
	if fn == nil {
		durablePrecheckModeProvider.Store(func() string { return string(precheckModeObserve) })
		return
	}
	durablePrecheckModeProvider.Store(fn)
}

func currentPrecheckMode() precheckMode {
	raw := string(precheckModeObserve)
	if fn, ok := durablePrecheckModeProvider.Load().(func() string); ok && fn != nil {
		raw = fn()
	}
	switch precheckMode(raw) {
	case precheckModeOff, precheckModeObserve, precheckModeOn:
		return precheckMode(raw)
	default:
		return precheckModeObserve
	}
}
