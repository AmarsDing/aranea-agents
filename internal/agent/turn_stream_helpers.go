package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// ErrFirstByteTimeout indicates the model did not produce a meaningful event in time.
var ErrFirstByteTimeout = errors.New("first byte timeout")

// DisplayMarkdownFromStream returns assistant-visible text from a consumed event stream.
// Reasoning-only replies are included when main content is empty (aligned with Team turns).
// The second return value indicates whether the display text is a reasoning fallback
// (i.e. the LLM produced only reasoning with no separate reply content).
func DisplayMarkdownFromStream(result EventStreamResult) (string, bool) {
	reply := strings.TrimSpace(result.Reply.String())
	if reply != "" {
		return reply, false
	}
	reasoning := strings.TrimSpace(result.Reasoning.String())
	return reasoning, reasoning != ""
}

// IsTerminalStreamFailure reports whether the consumed stream carries a
// terminal provider/runtime error that must fail the turn even when reply or
// reasoning text exists. 背景（2026-10-29 44 实测缺口）：流中段
// "Context size has been exceeded" 这类 response_error 只被记录进
// HasError/LastError、并不中止消费；reasoning 回退（DisplayMarkdownFromStream）
// 会把仅有推理文本的回合显示为有内容，持久化层于是把它当成功收官——真实
// 失败被吞。本谓词是 chat persistTurn 与 team runner 共用的收口守卫。
//
// doom loop 除外：doom 的既有语义是「持久化截断内容 + 标记 doom」，由
// 调用方专属路径处理，不走通用失败收敛。
func IsTerminalStreamFailure(result EventStreamResult) bool {
	if !result.HasError || result.DoomLoopDetected {
		return false
	}
	return strings.TrimSpace(result.LastError) != ""
}

// 输出预算护栏默认值（字符数）。env 可调：
//   - chat 路径：ARANEA_CHAT_MAX_REASONING_CHARS / ARANEA_CHAT_MAX_OUTPUT_CHARS
//   - team 路径：ARANEA_TEAM_MAX_REASONING_CHARS / ARANEA_TEAM_MAX_OUTPUT_CHARS
//
// 取值依据：推理型模型空转实测 ~30-60 tok/s，3 分钟 reasoning ≈ 20-40k 字符；
// team 成员上下文窗口常见 64k tokens，护栏须远早于 provider 溢出触发。
const (
	DefaultChatMaxReasoningChars = 32768
	DefaultChatMaxOutputChars    = 131072
	DefaultTeamMaxReasoningChars = 16384
	DefaultTeamMaxOutputChars    = 65536
)

// StreamOutputGuardLimitsFromEnv 读取输出预算配置：envPrefix_MAX_REASONING_CHARS /
// envPrefix_MAX_OUTPUT_CHARS，未设置或非法时回落默认值；返回 0 表示不启用
// （env 显式设为 0/负数时禁用该侧护栏）。
func StreamOutputGuardLimitsFromEnv(envPrefix string, defaultReasoning, defaultOutput int) (maxReasoning, maxOutput int) {
	return envIntOrDefault(envPrefix+"_MAX_REASONING_CHARS", defaultReasoning),
		envIntOrDefault(envPrefix+"_MAX_OUTPUT_CHARS", defaultOutput)
}

func envIntOrDefault(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	if v, err := strconv.Atoi(raw); err == nil {
		if v < 0 {
			return 0
		}
		return v
	}
	return def
}

// EstimateTokensIfMissing fills missing token counts from text using a symmetric
// "fill missing" semantic: each side is estimated independently from its own text.
// Caller must pass non-empty inputPreview for prompt estimation to work (chat path
// historically passed "" which suppressed prompt estimation — fixed at call sites).
func EstimateTokensIfMissing(promptTok, completionTok int, inputPreview, displayMarkdown string) (int, int) {
	if promptTok <= 0 {
		promptTok = RoughTokenEstimate(inputPreview)
	}
	if completionTok <= 0 {
		completionTok = RoughTokenEstimate(displayMarkdown)
	}
	return promptTok, completionTok
}

// ConsumeWithFirstByteGuard runs the turn stream consumer with a first-byte deadline.
func ConsumeWithFirstByteGuard(
	parentCtx context.Context,
	firstByteTimeout time.Duration,
	events <-chan *trpcevent.Event,
	meta ProjectMeta,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) (EventStreamResult, error) {
	if firstByteTimeout <= 0 {
		firstByteTimeout = DefaultFirstByteTimeout
	}
	firstByteCtx, cancel := context.WithTimeout(parentCtx, firstByteTimeout)
	defer cancel()
	received := false
	result := ConsumeEventStreamWithFirstByte(firstByteCtx, parentCtx, events, meta, &received, opts, lg)
	if result.FirstByteTimedOut || (!received && parentCtx.Err() == nil) {
		return result, fmt.Errorf("%w after %s", ErrFirstByteTimeout, firstByteTimeout)
	}
	return result, nil
}
