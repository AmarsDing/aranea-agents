// judge.go — P2 后置澄清转换的阻断性提问判定器（2026-08-21）。
//
// 背景：assistant 回复以纯文本提问收尾（如「我需要您补充：股票代码或交易所…？」）
// 但 Intent Pass 未标 needs_clarification 时，澄清门不触发，用户面对纯文本
// 干等、无结构化面板。service 层对「回复以 ?/？ 结尾」的 turn 调用本判定器，
// 由轻量 LLM 区分阻断性提问（转 ClarifyBlock 挂起）与礼貌性收尾（不打扰）。
package intent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/agent/llmcompat"
	"aranea-agents/internal/biz"
)

// judgeMaxReplyRunes 是送入判定器的回复文本上限（取尾部——问题在结尾）。
const judgeMaxReplyRunes = 2000

// LooksLikeTrailingQuestion 报告回复文本是否含疑问标记（?/？）。
// 仅作预筛：通过后才走 LLM 判定，避免对每条回复都付出一次旁路调用。
// 含问号即放行（不要求结尾——问句可能出现在回复中段，后随句号收尾的补充
// 说明）；误放进来的礼貌性收尾/设问由 LLM 判定器排除。
func LooksLikeTrailingQuestion(reply string) bool {
	s := strings.TrimSpace(reply)
	if s == "" {
		return false
	}
	return strings.Contains(s, "?") || strings.Contains(s, "？")
}

const blockingQuestionJudgeSystem = `You judge whether an assistant's reply is a BLOCKING question directed at the user: the assistant cannot make meaningful progress on the user's request until the user answers. Reply with ONE JSON object only, no markdown fences, no commentary. Keys:
- blocking (boolean): true only when the reply's main purpose is to ask the user for information or a decision required to proceed (e.g. which entity/ticker/time range, which option to choose, a missing required parameter). false for rhetorical questions, polite offers ("需要我继续吗？", "要不要我顺便…"), status reports that happen to end with a question, or when the reply already delivers substantive results and the question is an optional follow-up.
- question (string): when blocking is true, the single core question to present back to the user, rewritten as one concise sentence in the reply's language (merge multiple sub-questions into one sentence listing the needed details). Empty when blocking is false.`

// BlockingQuestionVerdict 是阻断性提问判定的结果。
type BlockingQuestionVerdict struct {
	Blocking      bool
	Question      string
	PromptTok     int
	CompletionTok int
	Duration      time.Duration
	// Outcome: completed | skipped_preflight | skipped_catalog | skipped_llm | skipped_parse
	Outcome string
}

// JudgeBlockingQuestion 调用轻量 chat completion 判定回复是否为阻断性提问。
// 任何失败（catalog/LLM/解析）均返回 Blocking=false——后置转换是增强而非
// 正确性依赖，判定失败时保持纯文本回复原样，不阻断 turn。
func JudgeBlockingQuestion(ctx context.Context, catalog biz.TeamModelCatalog, httpClient *http.Client, provider, model, replyText string) (res BlockingQuestionVerdict) {
	start := time.Now()
	defer func() { res.Duration = time.Since(start) }()

	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	replyText = strings.TrimSpace(replyText)
	if catalog == nil || httpClient == nil || provider == "" || model == "" || replyText == "" {
		res.Outcome = "skipped_preflight"
		return
	}
	// 只送尾部：问题在结尾，截断控制旁路调用的 prompt 规模。
	if utf8.RuneCountInString(replyText) > judgeMaxReplyRunes {
		replyText = string([]rune(replyText)[utf8.RuneCountInString(replyText)-judgeMaxReplyRunes:])
	}
	row, err := catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		res.Outcome = "skipped_catalog"
		return
	}
	var cfg llmcompat.ProviderAPIConfig
	llmcompat.MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	llmcompat.ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)
	// 与 intent pass 同理：分类任务强制关闭思考段，压旁路延迟。
	cfg.ThinkingDisabled = true

	msgs := []llmcompat.OpenAICompatMessage{
		{Role: "system", Content: blockingQuestionJudgeSystem},
		{Role: "user", Content: replyText},
	}
	// 判定器在 turn 收尾关键路径上，超时从严（intent pass 45s 是因与 BUILD
	// 并行被掩盖；此处串行，20s 上限防 provider 挂死拖住收尾）。
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	text, _, promptTok, completionTok, err := llmcompat.CallOpenAICompatChat(callCtx, httpClient, cfg, model, msgs)
	if err != nil {
		res.Outcome = "skipped_llm"
		return
	}
	res.PromptTok = promptTok
	res.CompletionTok = completionTok

	text = stripFences(text)
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var payload struct {
		Blocking bool   `json:"blocking"`
		Question string `json:"question"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(text)), &payload) != nil {
		res.Outcome = "skipped_parse"
		return
	}
	res.Blocking = payload.Blocking
	res.Question = strings.TrimSpace(payload.Question)
	res.Outcome = "completed"
	return
}
