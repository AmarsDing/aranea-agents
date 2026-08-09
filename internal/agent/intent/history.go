package intent

import (
	"strings"

	"aranea-agents/pkg/strutil"
)

// HistoryMessage 是注入意图识别 pass 的近期对话消息（仅 user/assistant）。
type HistoryMessage struct {
	Role    string
	Content string
}

const (
	// MaxIntentHistoryMessages 是注入 intent pass 的近期对话消息数上限。
	// 超过时丢弃最旧消息——意图识别只需要够解析指代/省略的近端上下文，
	// 全量历史只会稀释注意力并抬高 prompt 成本。
	MaxIntentHistoryMessages = 6
	// intentHistoryMaxContentRunes 是单条历史消息的内容截断上限（rune），
	// 防止单条长消息撑爆意图识别 prompt。
	intentHistoryMaxContentRunes = 200
)

// buildUserMessageContent 拼装 intent pass 的 user 消息：有历史时前置
// "Recent conversation" 段（先旧后新），当前消息固定在末尾。历史中的
// 非 user/assistant 角色与空内容在此过滤（防御性，上游已过滤一轮）。
func buildUserMessageContent(current string, history []HistoryMessage) string {
	current = strings.TrimSpace(current)
	if len(history) > MaxIntentHistoryMessages {
		history = history[len(history)-MaxIntentHistoryMessages:]
	}
	var sb strings.Builder
	sb.WriteString("Recent conversation (oldest first, for reference resolution only):\n")
	for _, h := range history {
		role := strings.TrimSpace(h.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(h.Content)
		if content == "" {
			continue
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(strutil.TruncateRunes(content, intentHistoryMaxContentRunes))
		sb.WriteString("\n")
	}
	if sb.Len() == len("Recent conversation (oldest first, for reference resolution only):\n") {
		// 历史全部过滤后无有效条目，退化为无历史形态。
		return "User message:\n\n" + current
	}
	sb.WriteString("\nUser message:\n\n")
	sb.WriteString(current)
	return sb.String()
}
