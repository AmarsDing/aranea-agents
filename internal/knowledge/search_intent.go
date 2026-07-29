package knowledge

import (
	"regexp"
	"strings"
)

// 搜索意图分流（P3 统一搜索框双区）。
//
// ⚠️ 本规则表与前端 `web/src/features/knowledge/searchIntent.ts` 维护**同一份定义**，
// 修改任一侧必须同步另一侧（两侧注释互指）。
//
// 规则（按优先级）：
//  1. 即时区（instant）——强定位信号：路径分隔符 / 扩展名模式 / 引号短语
//  2. 语义区（semantic）——自然语言问句：中/英疑问词开头或疑问语气结尾
//  3. auto —— 无强信号：双区并列（即时过滤 + 回车语义检索）
//
// 后端用途：knowledge_search 工具/检索路由可参考本分类选择 L0 精确层或 L2 语义层；
// 前端用途：双区搜索框决定即时区/语义区的主次展示与回车落点。

// SearchIntent 搜索意图分类。
type SearchIntent string

const (
	IntentInstant  SearchIntent = "instant"
	IntentSemantic SearchIntent = "semantic"
	IntentAuto     SearchIntent = "auto"
)

var (
	pathSeparatRe  = regexp.MustCompile(`[/\\]`)
	fileExtRe      = regexp.MustCompile(`(?i)\.(md|markdown|txt|log|pdf|docx?|xlsx?|pptx?|csv|json|ya?ml|toml|xml|html?|png|jpe?g|webp)\b`)
	quotedPhraseRe = regexp.MustCompile(`"[^"]+"|"[^"]+"`)
	zhQuestionHead = regexp.MustCompile(`^(什么|如何|怎么|为什么|为何|哪些|哪个|哪|是否|能不能|可以不可以)`)
	zhQuestionTail = regexp.MustCompile(`[吗呢吧？]\s*$`)
	enQuestionHead = regexp.MustCompile(`(?i)^(what|how|why|which|when|where|who|whom|whose|is|are|can|could|does|do)\b`)
	enQuestionTail = regexp.MustCompile(`\?\s*$`)
)

// ClassifySearchIntent 判定查询意图（规则表与前端 searchIntent.ts 一致）。
func ClassifySearchIntent(query string) SearchIntent {
	q := strings.TrimSpace(query)
	if q == "" {
		return IntentAuto
	}
	// 强即时信号优先：用户找的是具体文件/路径/短语，疑问语气不改变定位意图。
	if pathSeparatRe.MatchString(q) || fileExtRe.MatchString(q) || quotedPhraseRe.MatchString(q) {
		return IntentInstant
	}
	if zhQuestionHead.MatchString(q) || zhQuestionTail.MatchString(q) ||
		enQuestionHead.MatchString(q) || enQuestionTail.MatchString(q) {
		return IntentSemantic
	}
	return IntentAuto
}
