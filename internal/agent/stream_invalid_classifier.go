package agent

import "strings"

// stream_invalid_classifier.go — P0-4（A9/A10）：LLM 流式「无效输出」分类器。
//
// 背景：Gemini InvalidStream 同类缺口——流式 LLM 可能产出「技术上成功收官
// 但语义无效」的 turn：无 finish_reason 的空响应、中段 EOF、finish_reason=
// length 的截断、content_filter 安全拦截、tool_call 组装校验失败（框架
// llmflow.validateCompletedToolCallNames 硬终止）。历史代码把这些一律折叠
// 成「空回复/失败」，无分类、无重试区分。
//
// 本文件是纯函数分类器（无 I/O、无日志、无时钟，与 provider.ClassifyRetry
// 同风格）：stream_consumer 在 consume 期间收集原始观测（StreamInvalidObs），
// finalize 时经 ClassifyInvalidStream 打标到 EventStreamResult.InvalidKind；
// turn_invalid_retry.go 的重试助手据 InvalidActionFor 的判定表决定是否原样
// 重发。判定表是包级变量（设计 §12.2② 裁决：表驱动而非散落的 if 链）。
//
// 边界（设计 §5.5）：context-overflow 明确不在本分类器领地——它已由
// shrink-retry（P0-3 SHRINK）独占接管，本表必须返回 None，防止双重重试。

// StreamInvalidKind 枚举一次流式 turn 的无效类别。
type StreamInvalidKind int

const (
	// StreamInvalidNone — 流正常（或由 cancel/doom/首字节超时/上下文溢出等
	// 既有语义路径接管），无需 invalid-retry 介入。
	StreamInvalidNone StreamInvalidKind = iota
	// StreamInvalidNoFinishReason — 流收官时未观察到任何 finish_reason。
	StreamInvalidNoFinishReason
	// StreamInvalidMalformedToolCall — tool_call 组装校验失败（如函数名为空，
	// 框架 llmflow 硬终止）。
	StreamInvalidMalformedToolCall
	// StreamInvalidMaxTokens — finish_reason=length（输出超限截断）。
	StreamInvalidMaxTokens
	// StreamInvalidSafety — 安全系统拦截（finish_reason=content_filter 或
	// 错误载荷命中安全标记）。恒 TERMINATE，绝不重试（重试同一内容违反
	// 内容政策语义，且会反复烧 token）。
	StreamInvalidSafety
	// StreamInvalidHTTPEOF — HTTP 流中段 EOF（连接被供应商掐断）。
	StreamInvalidHTTPEOF
	// StreamInvalidStreamError — 其余通用流错误。
	StreamInvalidStreamError
)

func (k StreamInvalidKind) String() string {
	switch k {
	case StreamInvalidNone:
		return "none"
	case StreamInvalidNoFinishReason:
		return "no_finish_reason"
	case StreamInvalidMalformedToolCall:
		return "malformed_tool_call"
	case StreamInvalidMaxTokens:
		return "max_tokens"
	case StreamInvalidSafety:
		return "safety"
	case StreamInvalidHTTPEOF:
		return "http_eof"
	case StreamInvalidStreamError:
		return "stream_error"
	default:
		return "unknown"
	}
}

// StreamInvalidObs 是消费端对一次流式 turn 的纯观测快照。所有字段都是
// consume 循环内逐事件收集的事实，无派生语义。
type StreamInvalidObs struct {
	// SawFinishReason / FinishReason：流期间是否观察到 finish_reason 及其
	// 最后取值（OpenAI 惯例小写：stop / length / content_filter / tool_calls…）。
	SawFinishReason bool
	FinishReason    string
	// HasContent：是否累积到非空正文/推理文本。
	HasContent bool
	// ToolCallDispatched：本轮流内是否观察到任何 tool_call 信封（= 工具已
	// 派发/执行过）。一旦为 true，任何重新调用 Run 的重试都可能重复执行
	// 已产生副作用的工具——处置表据此把 Retry/续写降级为 Terminate。
	ToolCallDispatched bool
	// DoomLoopDetected：消费端 doom-loop 检测中止（重复输出）。
	DoomLoopDetected bool
	// FirstByteTimedOut：消费端首字节守卫判死（属 stall 守卫语义，不在
	// invalid-retry 领地——stall 预算已在 guard 内耗尽）。
	FirstByteTimedOut bool
	// Canceled：turn ctx 取消/用户中止/消费端 panic 兜底归一。
	Canceled bool
	// HasError / LastError / LastErrorType：流内观察到的最后一个错误事件。
	HasError      bool
	LastError     string
	LastErrorType string
}

// InvalidRetryAction 是分类后的处置动作。
type InvalidRetryAction int

const (
	// InvalidActionContinue — 接受现状：不再重试（MaxAttempts=0）或允许有限
	// 次续写重发（MaxAttempts>0，语义 = 无 finish_reason 但有内容 / length
	// 截断时的「继续生成」尝试，沿用同一 prompt 原样重发）。
	InvalidActionContinue InvalidRetryAction = iota
	// InvalidActionRetry — 原样重发整个 turn（仅当本轮未派发任何 tool_call
	// 时合法）。
	InvalidActionRetry
	// InvalidActionTerminate — 终态：不做任何额外动作，结果原样交给既有
	// 失败/空回复/部分内容路径。
	InvalidActionTerminate
)

func (a InvalidRetryAction) String() string {
	switch a {
	case InvalidActionContinue:
		return "continue"
	case InvalidActionRetry:
		return "retry"
	case InvalidActionTerminate:
		return "terminate"
	default:
		return "unknown"
	}
}

// StreamInvalidAction 是 (kind × 有无内容) 的处置判定：动作 + 允许的额外
// 尝试上限（不含首次；0 = 原样接受/终止）。
type StreamInvalidAction struct {
	Action      InvalidRetryAction
	MaxAttempts int
}

// invalidStreamStrategy 是包级策略表（设计 §12.2②：表驱动，pin 测试覆盖
// 全枚举×边界）。行 = kind；列 = 有内容 / 无内容。
var invalidStreamStrategy = map[StreamInvalidKind]struct {
	withContent StreamInvalidAction
	noContent   StreamInvalidAction
}{
	StreamInvalidNone: {
		withContent: StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 0},
		noContent:   StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 0},
	},
	StreamInvalidNoFinishReason: {
		// 有内容 = 半截正文后流静默收官：允许 1 次续写重发。
		withContent: StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 1},
		// 空响应 = 什么都没产出：允许 2 次原样重发。
		noContent: StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
	},
	StreamInvalidMalformedToolCall: {
		// tool_call 组装失败：未派发时可安全重发；有正文与否都可能伴随半截
		// tool_call delta——只要 ToolCallDispatched=false 就从未执行过工具。
		withContent: StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
		noContent:   StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
	},
	StreamInvalidMaxTokens: {
		// length 截断且有内容：允许 1 次续写尝试；空内容 = length 出现在
		// 零输出上（异常），直接终态。
		withContent: StreamInvalidAction{Action: InvalidActionContinue, MaxAttempts: 1},
		noContent:   StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0},
	},
	StreamInvalidSafety: {
		// doom flag 同类：安全拦截恒 TERMINATE，绝不重试。
		withContent: StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0},
		noContent:   StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0},
	},
	StreamInvalidHTTPEOF: {
		// 中段 EOF：未派发工具时重发安全。
		withContent: StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
		noContent:   StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
	},
	StreamInvalidStreamError: {
		withContent: StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
		noContent:   StreamInvalidAction{Action: InvalidActionRetry, MaxAttempts: 2},
	},
}

// 错误消息标记集（strings.Contains，小写化后匹配）。与 provider 包
// retry_classifier.go 的标记哲学一致：集中定义 + 测试 pin。
var (
	// invalidMalformedToolCallMarkers：框架 llmflow.validateCompletedToolCallNames
	// 的硬终止消息（pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go:1018）。
	invalidMalformedToolCallMarkers = []string{
		"tool call function name is empty",
	}
	// invalidSafetyMarkers：供应商安全系统拒绝的文本标记（镜像 provider
	// contentFilterMarkers，去掉过宽的 "blocked by"——它同时命中普通
	// 网络拦截错误，安全类误判恒 TERMINATE 的代价是错杀可重试故障）。
	invalidSafetyMarkers = []string{
		"content_filter",
		"content filter",
		"safety system",
		"moderation",
	}
	// invalidMaxTokensMarkers：输出超限错误文本（与 finish_reason=length
	// 互补——部分供应商以错误事件而非 finish_reason 表达）。刻意避开
	// "maximum context length"（属溢出领地）。
	invalidMaxTokensMarkers = []string{
		"max_tokens",
		"max output tokens",
		"output length",
	}
	// invalidOverflowMarkers：上下文溢出——P0-3 SHRINK 独占领地，本分类器
	// 恒返回 None（防双重重试）。镜像 provider.contextOverflowMarkers。
	invalidOverflowMarkers = []string{
		"context length exceeded",
		"context_length_exceeded",
		"context window",
		"maximum context length",
		"token limit exceeded",
		"request too large",
	}
	// invalidHTTPEOFMarkers：HTTP 流中段 EOF。
	invalidHTTPEOFMarkers = []string{
		"unexpected eof",
		"eof",
	}
	// invalidStallExhaustedMarkers：活性守卫重连穷尽的终态错误（first_byte_retry.go
	// emitTerminalStallError）。stall 预算已在守卫内耗尽，不属于可重试 invalid。
	invalidStallExhaustedMarkers = []string{
		"liveness exhausted",
	}
)

func matchInvalidMarker(msg string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// ClassifyInvalidStream 是纯分类函数：观测快照 → 无效类别。
//
// 判定优先级（高→低）：
//  1. doom / canceled / 首字节超时 → None（各有专属语义路径，且 doom 恒终态）。
//  2. finish_reason 正常收官族（stop/tool_calls/end_turn/function_call）→ None。
//  3. finish_reason = content_filter → Safety；= length → MaxTokens
//     （finish_reason 是供应商的权威终态，优先于错误文本猜测）。
//  4. 错误事件标记分析：安全 > 溢出（None，P0-3 领地）> 输出超限 > 活性耗尽
//     （None，预算已尽）> malformed tool_call > EOF > 通用 StreamError。
//  5. 无 finish_reason → NoFinishReason；其余 → None。
func ClassifyInvalidStream(obs StreamInvalidObs) StreamInvalidKind {
	// doom flag：恒终态（设计 §5.5#1 doom abort 一律不重试）。
	if obs.DoomLoopDetected {
		return StreamInvalidNone
	}
	if obs.Canceled {
		return StreamInvalidNone
	}
	if obs.FirstByteTimedOut {
		return StreamInvalidNone
	}

	finish := strings.ToLower(strings.TrimSpace(obs.FinishReason))
	if obs.SawFinishReason {
		switch finish {
		case "stop", "tool_calls", "function_call", "end_turn":
			return StreamInvalidNone
		case "content_filter", "safety":
			return StreamInvalidSafety
		case "length", "max_tokens":
			return StreamInvalidMaxTokens
		}
	}

	if obs.HasError {
		errText := strings.ToLower(obs.LastErrorType + " " + obs.LastError)
		// 安全拦截的错误形态也恒 Safety（错误文本可能先于 finish_reason 到达，
		// 或供应商只发错误不发 finish_reason）。
		if matchInvalidMarker(errText, invalidSafetyMarkers) {
			return StreamInvalidSafety
		}
		// 溢出是 P0-3 SHRINK 独占领地：恒 None，防双重重试。
		if matchInvalidMarker(errText, invalidOverflowMarkers) {
			return StreamInvalidNone
		}
		if matchInvalidMarker(errText, invalidMaxTokensMarkers) {
			return StreamInvalidMaxTokens
		}
		// 活性守卫重连穷尽的终态错误：stall 预算已在 guard 内耗尽，不归
		// invalid-retry（否则等于绕过 stall 预算共账）。
		if matchInvalidMarker(errText, invalidStallExhaustedMarkers) {
			return StreamInvalidNone
		}
		if matchInvalidMarker(errText, invalidMalformedToolCallMarkers) {
			return StreamInvalidMalformedToolCall
		}
		if matchInvalidMarker(errText, invalidHTTPEOFMarkers) {
			return StreamInvalidHTTPEOF
		}
		return StreamInvalidStreamError
	}

	if !obs.SawFinishReason {
		return StreamInvalidNoFinishReason
	}
	return StreamInvalidNone
}

// InvalidActionFor 查策略表并应用两条全表 override：
//
//  1. doom flag（obs.DoomLoopDetected=true）：无论分类为何，恒 TERMINATE
//     （防御纵深——正常链路 classify 已把 doom 归为 None，此处钉死语义）。
//  2. ToolCallDispatched=true：任何带重试/续写次数的处置降级为 TERMINATE——
//     本轮工具已派发/执行，重新调用整个 Run 会重复产生副作用。
func InvalidActionFor(kind StreamInvalidKind, obs StreamInvalidObs) StreamInvalidAction {
	if obs.DoomLoopDetected {
		return StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0}
	}
	row, ok := invalidStreamStrategy[kind]
	if !ok {
		return StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0}
	}
	act := row.noContent
	if obs.HasContent {
		act = row.withContent
	}
	if obs.ToolCallDispatched && act.MaxAttempts > 0 {
		return StreamInvalidAction{Action: InvalidActionTerminate, MaxAttempts: 0}
	}
	return act
}
