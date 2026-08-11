package service

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ── P3 M2: Agent Case LLM 提取器 ─────────────────────────────────────────
//
// 与 MemoryLLMExtractor（用户记忆/L3 facts）并列：复用其 callModel 通道
// （provider 路由/超时/错误语义），用独立 prompt 提取面向 Agent 复用的任务
// 经验。输出为纯 JSON（不用 function calling，schema 小且只需单对象）。

// agentCaseExtractSystemPrompt 要求 LLM 以 Agent 自我改进视角提取经验，
// 显式排除用户画像（那是用户记忆管线的职责）。
const agentCaseExtractSystemPrompt = `你是 Agent 任务经验提取器。从对话中提取该 Agent 完成任务的可复用经验——面向 Agent 自我改进，而非用户画像。

只输出一个 JSON 对象，不要输出任何其他内容：
{
  "goal": "用户要完成的任务目标（一句话）",
  "approach": "Agent 解决问题的有效做法与关键步骤（2-4 句，写可复用的方法而非流水账）",
  "outcome": "success | partial | failure",
  "outcome_summary": "结果一句话",
  "pitfalls": "失败或走弯路的教训（仅 outcome 非 success 时填写，否则为空字符串）",
  "tools_used": ["实际用到的工具名"],
  "quality": 0.0
}

规则：
- 对话无实质任务（闲聊、单轮常识问答）→ 只输出 {"skip": true}
- approach 必须是可迁移的做法，禁止逐句复述对话过程
- pitfalls 只在非 success 时非空
- quality 是你对本次提取可靠性的自评（0.0-1.0）
- 不要提取用户个人信息（姓名/偏好/习惯等），那是另一条记忆管线的职责`

// AgentCaseLLMExtractor implements biz.AgentCaseExtractor via the shared
// memory-worker LLM channel.
type AgentCaseLLMExtractor struct {
	llm *MemoryLLMExtractor
}

// NewAgentCaseLLMExtractor wraps the shared memory LLM extractor. Returns nil
// when llm is nil (Worker then falls back to heuristic extraction).
func NewAgentCaseLLMExtractor(llm *MemoryLLMExtractor) *AgentCaseLLMExtractor {
	if llm == nil {
		return nil
	}
	return &AgentCaseLLMExtractor{llm: llm}
}

var _ biz.AgentCaseExtractor = (*AgentCaseLLMExtractor)(nil)

// ExtractCase runs one LLM call and parses the structured case. Skip / parse
// semantics: ErrAgentCaseSkip = 会话无提取价值（调用方整条跳过）；
// ErrLLMExtractorUnavailable / ErrLLMExtractionFailed = 调用方降级启发式。
func (e *AgentCaseLLMExtractor) ExtractCase(ctx context.Context, in biz.ConsolidateInput) (*biz.AgentCase, error) {
	if e == nil || e.llm == nil {
		return nil, biz.ErrLLMExtractorUnavailable
	}
	transcript := buildCaseTranscript(in.Messages)
	if transcript == "" {
		return nil, biz.ErrAgentCaseSkip
	}
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(agentCaseExtractSystemPrompt),
			trpcmodel.NewUserMessage("Conversation excerpt:\n\n" + transcript),
		},
	}
	text, _, err := e.llm.callModel(ctx, in, req)
	if err != nil {
		return nil, err
	}
	return parseAgentCaseResponse(text)
}

// agentCaseLLMResponse 是提取 prompt 约定的 JSON 结构。
type agentCaseLLMResponse struct {
	Skip           bool     `json:"skip"`
	Goal           string   `json:"goal"`
	Approach       string   `json:"approach"`
	Outcome        string   `json:"outcome"`
	OutcomeSummary string   `json:"outcome_summary"`
	Pitfalls       string   `json:"pitfalls"`
	ToolsUsed      []string `json:"tools_used"`
	Quality        float64  `json:"quality"`
}

// parseAgentCaseResponse parses the LLM JSON output into an AgentCase.
// Markdown code fences are tolerated. Normalization: unknown outcome →
// partial；quality 截断到 (0,1]，未给/非法时按 JSON 模式默认 0.85。
func parseAgentCaseResponse(text string) (*biz.AgentCase, error) {
	s := stripCaseJSONFence(strings.TrimSpace(text))
	if s == "" {
		return nil, biz.ErrLLMExtractionFailed
	}
	var r agentCaseLLMResponse
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, biz.ErrLLMExtractionFailed
	}
	if r.Skip {
		return nil, biz.ErrAgentCaseSkip
	}
	goal := strings.TrimSpace(r.Goal)
	if goal == "" {
		// 没提取到目标 = 无实质任务，按 skip 语义处理。
		return nil, biz.ErrAgentCaseSkip
	}
	outcome := strings.TrimSpace(strings.ToLower(r.Outcome))
	switch outcome {
	case biz.AgentCaseOutcomeSuccess, biz.AgentCaseOutcomePartial, biz.AgentCaseOutcomeFailure:
	default:
		outcome = biz.AgentCaseOutcomePartial
	}
	pitfalls := strings.TrimSpace(r.Pitfalls)
	if outcome == biz.AgentCaseOutcomeSuccess {
		pitfalls = ""
	}
	quality := r.Quality
	if quality <= 0 {
		quality = biz.ExtractionQualityJSONMode
	}
	if quality > 1 {
		quality = 1
	}
	var tools []string
	seen := map[string]struct{}{}
	for _, name := range r.ToolsUsed {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tools = append(tools, name)
	}
	return &biz.AgentCase{
		Goal:           goal,
		Approach:       strings.TrimSpace(r.Approach),
		Outcome:        outcome,
		OutcomeSummary: strings.TrimSpace(r.OutcomeSummary),
		Pitfalls:       pitfalls,
		ToolsUsed:      tools,
		Quality:        quality,
	}, nil
}

// buildCaseTranscript renders messages for the case prompt. Tool messages keep
// their tool name — it is the only signal the LLM has for tools_used.
func buildCaseTranscript(msgs []biz.ConsolidateMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "unknown"
		}
		if name := strings.TrimSpace(m.ToolName); name != "" {
			role = role + "(" + name + ")"
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// stripCaseJSONFence removes an optional wrapping ```json ... ``` fence.
func stripCaseJSONFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
