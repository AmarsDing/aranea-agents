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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/agent/llmcompat"
	"aranea-agents/internal/biz"
)

// judgeMaxReplyRunes 是送入判定器的回复文本上限（取尾部——问题在结尾）。
const judgeMaxReplyRunes = 2000

// LooksLikeTrailingQuestion 报告回复文本是否疑似阻断性提问收尾。
// 仅作预筛：通过后才走 LLM 判定，避免对每条回复都付出一次旁路调用。
// 两条放行路径：
//  1. 含问号（?/？）即放行（不要求结尾——问句可能出现在回复中段，后随句号
//     收尾的补充说明）；误放进来的礼貌性收尾/设问由 LLM 判定器排除。
//  2. 祈使式信息请求（2026-10-29 round-11：本地模型常用无问号祈使句反问，
//     如「请告诉我你想重复哪一项：」「请补充：1.…2.…」「回我一个编号或名字
//     就行」）。为控制误放（每次误判挂起都会终结 turn 等待用户），本路径
//     要求双重锚定：请求必须出现在收尾位置（最后一个非空行；若回复以列表
//     收尾则取列表引导行）且该行末句含信息请求短语；同句含礼貌软化语
//     （如需/如果/随时…）时不放行——那是「如需调整请告诉我」式客套收尾。
func LooksLikeTrailingQuestion(reply string) bool {
	s := strings.TrimSpace(reply)
	if s == "" {
		return false
	}
	if strings.Contains(s, "?") || strings.Contains(s, "？") {
		return true
	}
	return looksLikeImperativeInfoRequest(s)
}

// infoRequestMarkers 是用户导向的信息请求短语（索取信息/确认/选择/补充）。
// 仅在收尾句中生效；「告诉我/回我」等口语化表达由 LLM 判定器兜底误放。
var infoRequestMarkers = []string{
	"请告诉我", "请提供", "请问", "请确认", "请选择", "请补充", "请说明",
	"请告知", "请回复", "请指明", "请给出", "请填", "请输入",
	"麻烦", "告诉我", "回我",
	"你想", "你想要", "你需要", "你希望", "你打算", "你指的是",
	"我需要", "还需要",
}

// infoRequestSofteners 是礼貌性/假设性软化语：与请求短语同句出现时不视为
// 阻断请求（「如需调整请告诉我」是收尾客套，而非挂起等待用户作答）。
var infoRequestSofteners = []string{
	"如需", "如有", "如果", "若是", "随时", "欢迎", "的话",
}

// listItemLineRe 匹配 Markdown 列表项行（-/* • 或 1. 1、1) 编号）。
var listItemLineRe = regexp.MustCompile(`^\s*(?:[-*•·]|\d{1,2}[.、)）])`)

// sentenceEndRe 按中文/英文句末标点切句（不含 .，避免切断「1.」编号）。
var sentenceEndRe = regexp.MustCompile(`[。！？!?；;…]+`)

// looksLikeImperativeInfoRequest 报告回复是否以祈使式信息请求收尾。
func looksLikeImperativeInfoRequest(reply string) bool {
	line := tailRequestLine(reply)
	if line == "" {
		return false
	}
	segments := sentenceEndRe.Split(line, -1)
	for i := len(segments) - 1; i >= 0; i-- {
		sent := strings.TrimSpace(segments[i])
		if sent == "" {
			continue
		}
		// 只看末句：请求短语在收尾句才算 terminal position；
		// 中段祈使句后随陈述收尾的不放行（由问号路径或判定器覆盖）。
		if !containsAnySubstr(sent, infoRequestMarkers) {
			return false
		}
		return !containsAnySubstr(sent, infoRequestSofteners)
	}
	return false
}

// tailRequestLine 取用于祈使句检测的收尾候选行：最后一个非空行；若回复以
// 列表（选项/待补清单）收尾，则向上取列表的引导行——请求通常写在引导行上
// （「请提供以下信息：\n1. …\n2. …」）。
func tailRequestLine(reply string) string {
	lines := strings.Split(reply, "\n")
	idx := len(lines) - 1
	for idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	if idx < 0 {
		return ""
	}
	if !listItemLineRe.MatchString(lines[idx]) {
		return strings.TrimSpace(lines[idx])
	}
	// 列表收尾：跳过连续列表项与空行，取上方引导行。
	for idx >= 0 && (strings.TrimSpace(lines[idx]) == "" || listItemLineRe.MatchString(lines[idx])) {
		idx--
	}
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(lines[idx])
}

func containsAnySubstr(s string, subs []string) bool {
	for _, p := range subs {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
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
	// CachedTok 是 prompt-cache 命中部分（2026-10-20 补透出，此前解析层丢弃）。
	CachedTok int
	Duration  time.Duration
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

	text, _, promptTok, completionTok, cachedTok, err := llmcompat.CallOpenAICompatChat(callCtx, httpClient, cfg, model, msgs)
	if err != nil {
		res.Outcome = "skipped_llm"
		return
	}
	res.PromptTok = promptTok
	res.CompletionTok = completionTok
	res.CachedTok = cachedTok

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
