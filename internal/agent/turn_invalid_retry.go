package agent

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// turn_invalid_retry.go — P0-4（A10）：LLM 流式无效输出的「整轮原样重发」助手。
//
// 与 task_planner_decompose_llm.go 的 decomposeTaskStream 同范式：maxAttempts
// + 分类判定（此处由表驱动的 InvalidActionFor 给出）+ 指数退避 + ctx 穿透。
//
// 安全闸门（设计 §5.5#5/#6）：
//   - 仅当本轮未派发任何 tool_call（result.ToolCallDispatched=false）时才允许
//     重发——重新调用 runner.Run 会重放整个 agent loop，已执行的工具会重复
//     产生副作用。已派发的处置表已降级为 Terminate，此处只消费判定结果。
//   - SAFETY / doom 恒 Terminate，绝不重试。
//   - 硬错误（attemptFunc 返回 err，如首字节超时）一律透传不重试——首字节
//     守卫与 stall 重连预算已在模型层耗尽，重复重试等于绕过预算共账。
//
// stall 预算共账（设计 §12.2③）：重试尝试按 (attemptNo-1) 递减 stall 重连
// 池，经 provider.WithStallBudget 既有 ctx key 注入（见各接线点的 attempt
// 闭包）；消费端守卫总时限同步按 InvalidStallReconnects 重算。本文件只提供
// 纯函数 InvalidStallReconnects 与 attemptNo 契约，不新造 ctx key。
//
// 已知残余（Batch B 候选项）：框架 runner.Run 每次调用都会把 user message
// 追加进 framework session（runner.persistCurrentTurnMessages），重试会在
// 框架会话内留下重复 user 消息；落库消息由 ActivityProjector/AppendChatMessage
// 负责（幂等），仅框架内存事件流受影响。本批次不接线 rollback（chat 边界
// 不在允许文件内、team 的 rollbackRunnerSession 是一次性失败清理语义），
// 框架侧去重留待 Batch B。

const (
	// DefaultInvalidMaxRetries 是 invalid-retry 的默认重试上限（不含首次尝试）。
	DefaultInvalidMaxRetries = 2

	defaultInvalidBackoffBase = 1 * time.Second
	defaultInvalidBackoffCap  = 10 * time.Second
)

// invalid-retry 的 flow_log 步骤 ID。
const (
	StepInvalidClassified = "stream.invalid.classified"
	StepInvalidRetry      = "stream.invalid.retry"
	StepInvalidTerminated = "stream.invalid.terminated"
)

// InvalidRetryOptions 控制整轮无效重试。零值 = 全关（Enabled=false）——
// 默认行为与接线前逐字节一致（设计 §12.1：新开关默认 OFF）。
type InvalidRetryOptions struct {
	// Enabled 总开关；false 时 RunTurnWithInvalidRetry 直通单次尝试。
	Enabled bool
	// MaxRetries 重试上限（不含首次）；<=0 时落回 DefaultInvalidMaxRetries。
	// 与分类表逐 kind 上限取 min（硬帽）。
	MaxRetries int
	// BackoffBase / BackoffCap 指数退避基准/封顶；<=0 落回包默认值。
	BackoffBase time.Duration
	BackoffCap  time.Duration
	// Emitter 用于 flow_log 事件（可 nil——所有事件为 AS-EVT-01
	// Informational 级，丢失仅降可观测性，不影响语义）。
	Emitter *event.TraceEmitter
	// BeforeRetry 在每次重发前回调（如未来的会话回滚接线）；返回非 nil
	// 错误则放弃重试并返回最近一次结果。可 nil。
	BeforeRetry func(ctx context.Context, nextAttempt int) error
	// JitterFn 计算第 n 次重试前的退避（测试注入点）；nil 时用默认
	// 指数退避 ±25% 抖动。
	JitterFn func(attempt int, base time.Duration) time.Duration
}

// InvalidRetryOptionsFromEnv 经环境变量构造开关（conf.proto 不在本批次允许
// 改动清单内——沿用 env overlay 的既可达配置路径，与 ARANEA_INTENT_PASS_*
// 同例）：
//
//	ARANEA_INVALID_RETRY_ENABLED         "1"/"true" 开启（默认关）
//	ARANEA_INVALID_RETRY_MAX_RETRIES     重试上限（默认 2，<=0 视为 0=仅直通标记）
//	ARANEA_INVALID_RETRY_BACKOFF_BASE_MS 退避基准毫秒（默认 1000）
//	ARANEA_INVALID_RETRY_BACKOFF_CAP_MS  退避封顶毫秒（默认 10000）
func InvalidRetryOptionsFromEnv() InvalidRetryOptions {
	opts := InvalidRetryOptions{
		Enabled:     false,
		MaxRetries:  DefaultInvalidMaxRetries,
		BackoffBase: defaultInvalidBackoffBase,
		BackoffCap:  defaultInvalidBackoffCap,
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_INVALID_RETRY_ENABLED"))); v == "1" || v == "true" {
		opts.Enabled = true
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_INVALID_RETRY_MAX_RETRIES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MaxRetries = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_INVALID_RETRY_BACKOFF_BASE_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.BackoffBase = time.Duration(n) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_INVALID_RETRY_BACKOFF_CAP_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.BackoffCap = time.Duration(n) * time.Millisecond
		}
	}
	return opts
}

// InvalidStallReconnects 返回第 attemptNo 次尝试（1 基）可用的 stall 重连
// 预算：首次保持全量（与历史行为逐字节一致），此后每次重试递减 1（与
// invalid-retry 共账——一次 turn 内的模型层重连 + 编排层重发共享同一
// 恢复预算池，防止「每层各重试 N 次」的乘数放大）。
func InvalidStallReconnects(base, attemptNo int) int {
	if base <= 0 {
		return 0
	}
	if attemptNo <= 1 {
		return base
	}
	d := base - (attemptNo - 1)
	if d < 0 {
		return 0
	}
	return d
}

// TurnInvalidAttempt 是「一次完整 turn 尝试」：接线方在闭包内完成
// 预算注入 → LLM 调用 → 流消费，返回消费结果（err 仅限硬错误，如首字节
// 超时/调用发起失败）。attemptNo 从 1 开始。
type TurnInvalidAttempt func(ctx context.Context, attemptNo int) (EventStreamResult, error)

// RunTurnWithInvalidRetry 运行一次 turn，并按分类表对「无效收官」做整轮原样
// 重发。返回最后一次尝试的结果（InvalidRetryAttempts 记录已执行的重试次数）。
//
// 直通条件（不触发任何重试）：opts.Enabled=false；attemptFunc 返回硬错误；
// 分类为 None；处置为 Terminate；已达重试上限；父 ctx 取消。
func RunTurnWithInvalidRetry(
	ctx context.Context,
	attemptFn TurnInvalidAttempt,
	opts InvalidRetryOptions,
	lg loggateway.Logger,
) (EventStreamResult, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if !opts.Enabled {
		return attemptFn(ctx, 1)
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	attemptsUsed := 0
	for attemptNo := 1; ; attemptNo++ {
		result, err := attemptFn(ctx, attemptNo)
		if err != nil {
			// 硬错误一律透传：模型层 stall/首字节预算已耗尽，编排层不再叠加。
			result.InvalidRetryAttempts = attemptsUsed
			return result, err
		}
		kind := result.InvalidKind
		if kind == StreamInvalidNone {
			result.InvalidRetryAttempts = attemptsUsed
			return result, nil
		}
		obs := StreamInvalidObs{
			HasContent:         result.HasContent,
			ToolCallDispatched: result.ToolCallDispatched,
			DoomLoopDetected:   result.DoomLoopDetected,
		}
		act := InvalidActionFor(kind, obs)
		// AS-EVT-01: Informational（flow_log 尽力而为，丢失仅降可观测性）。
		emitInvalidFlow(opts.Emitter, StepInvalidClassified, "流式输出无效分类",
			event.P("kind", kind.String()),
			event.P("action", act.Action.String()),
			event.P("attempt", attemptNo),
			event.P("has_content", result.HasContent),
			event.P("tool_call_dispatched", result.ToolCallDispatched))

		budget := act.MaxAttempts
		if budget > maxRetries {
			budget = maxRetries
		}
		if act.Action == InvalidActionTerminate || budget <= 0 || attemptsUsed >= budget {
			// AS-EVT-01: Informational（同上）。
			emitInvalidFlow(opts.Emitter, StepInvalidTerminated, "无效流终态：不再重试",
				event.P("kind", kind.String()),
				event.P("action", act.Action.String()),
				event.P("attempt", attemptNo),
				event.P("attempts_used", attemptsUsed),
				event.P("retry_budget", budget))
			result.InvalidRetryAttempts = attemptsUsed
			return result, nil
		}
		if ctx.Err() != nil {
			result.InvalidRetryAttempts = attemptsUsed
			return result, nil
		}
		if opts.BeforeRetry != nil {
			if herr := opts.BeforeRetry(ctx, attemptNo+1); herr != nil {
				lg.Warn("invalid-retry 前置回调失败，放弃重试",
					loggateway.StepID(StepInvalidTerminated),
					loggateway.Err(herr))
				result.InvalidRetryAttempts = attemptsUsed
				return result, nil
			}
		}
		attemptsUsed++
		delay := invalidRetryBackoff(attemptsUsed, opts)
		// AS-EVT-01: Informational（同上；retry 计数即本事件流，不再单独接
		// Prometheus——指标项见 Batch B）。
		emitInvalidFlow(opts.Emitter, StepInvalidRetry, "无效流重试",
			event.P("kind", kind.String()),
			event.P("action", act.Action.String()),
			event.P("next_attempt", attemptNo+1),
			event.P("attempts_used", attemptsUsed),
			event.P("backoff_ms", delay.Milliseconds()))
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				result.InvalidRetryAttempts = attemptsUsed
				return result, nil
			case <-timer.C:
			}
		}
	}
}

// emitInvalidFlow 发 flow_log（TraceEmitter 可 nil——Informational 级事件
// 丢失仅降可观测性）。
func emitInvalidFlow(emitter *event.TraceEmitter, stepID, message string, extra ...event.Pair) {
	if emitter == nil {
		return
	}
	emitter.Log(stepID, event.FlowPhaseSkip, message, extra...)
}

func invalidRetryBackoff(attempt int, opts InvalidRetryOptions) time.Duration {
	base := opts.BackoffBase
	if base <= 0 {
		base = defaultInvalidBackoffBase
	}
	cap := opts.BackoffCap
	if cap <= 0 {
		cap = defaultInvalidBackoffCap
	}
	d := base << (attempt - 1)
	if d > cap {
		d = cap
	}
	if opts.JitterFn != nil {
		return opts.JitterFn(attempt, d)
	}
	// ±25% 抖动，防多会话同刻重试雷群（与 task_planner 范式一致）。
	jitter := d / 4
	if jitter > 0 {
		d += time.Duration(rand.Int63n(int64(2*jitter))) - jitter
	}
	if d < 0 {
		d = 0
	}
	return d
}
