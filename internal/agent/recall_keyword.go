package agent

import (
	"encoding/json"
	"strings"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const intentContextHeader = "Derived intent (align your plan and tools to this JSON):"

// RecallKeywordFromMessages prefers Intent Pass search_hints, then last user text.
func RecallKeywordFromMessages(messages []trpcmodel.Message) string {
	for _, m := range messages {
		if m.Role != trpcmodel.RoleSystem {
			continue
		}
		if kw := searchHintsFromIntentContent(m.Content); kw != "" {
			return kw
		}
	}
	return lastUserMessageText(messages)
}

func searchHintsFromIntentContent(content string) string {
	content = strings.TrimSpace(content)
	if !strings.Contains(content, intentContextHeader) {
		return ""
	}
	jsonPart := strings.TrimSpace(strings.TrimPrefix(content, intentContextHeader))
	if jsonPart == "" {
		jsonPart = content[strings.Index(content, intentContextHeader)+len(intentContextHeader):]
		jsonPart = strings.TrimSpace(jsonPart)
	}
	var art struct {
		SearchHints []string `json:"search_hints"`
		RefinedGoal string   `json:"refined_goal"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &art); err != nil {
		return ""
	}
	var parts []string
	for _, h := range art.SearchHints {
		if s := strings.TrimSpace(h); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) > 0 {
		kw := strings.Join(parts, " ")
		return trimRecallKeyword(kw)
	}
	if g := strings.TrimSpace(art.RefinedGoal); g != "" {
		return trimRecallKeyword(g)
	}
	return ""
}

func trimRecallKeyword(s string) string {
	s = strings.TrimSpace(s)
	return safeTruncate(s, 120)
}
