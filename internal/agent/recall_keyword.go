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
	return cleanRecallQuery(lastUserMessageText(messages))
}

// recallQueryMaxSegments caps how many sub-queries multi-query expansion
// packs into the single recall query string (P1-1). Fan-out stays inside
// one string so downstream recallers keep one embed + one search per turn.
const recallQueryMaxSegments = 4

// recallFillerPrefixes are high-precision politeness/filler openers stripped
// from each sub-query. The list is deliberately short: only unambiguous
// fillers (never bare "请"/"帮" — "请帖"/"帮派" are real nouns).
var recallFillerPrefixes = []string{
	"请直接回答一下", "请直接回答", "请回答一下", "请回答",
	"请问一下", "请问", "麻烦你一下", "麻烦你", "麻烦",
	"帮我一下", "帮我", "帮忙", "告诉我一下", "告诉我",
	"我想知道", "我想问", "你好", "您好", "在吗", "哈喽", "嗨",
}

// cleanRecallQuery turns a raw user sentence into a focused recall query
// (P1-1 召回关键词清洗 + 多查询扩展):
//  1. Split into sub-queries on sentence/clause punctuation — a multi-question
//     turn ("我叫什么？我喜欢喝什么？") becomes multiple focused sub-queries
//     instead of one punctuation-laden blob that keyword/vector matching
//     cannot score well.
//  2. Strip filler prefixes and dedupe; drop empty segments.
//  3. Pack tail-first (question-last bias): when over the 120-rune budget,
//     leading chit-chat is dropped before the actual question at the tail.
//
// Falls back to the legacy truncate-to-120 behavior when cleaning yields
// nothing (filler-only input) or the input is a single unsegmented blob.
func cleanRecallQuery(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	segs := splitRecallSegments(s)
	if len(segs) == 0 {
		return trimRecallKeyword(s)
	}
	if len(segs) > recallQueryMaxSegments {
		// Keep the tail segments — the most recent clauses carry the intent.
		segs = segs[len(segs)-recallQueryMaxSegments:]
	}
	// Tail-biased packing within the 120-rune budget.
	picked := make([]string, 0, len(segs))
	budget := 120
	for i := len(segs) - 1; i >= 0; i-- {
		cost := len([]rune(segs[i]))
		if len(picked) > 0 {
			cost++ // joining space
		}
		if cost > budget {
			if len(picked) == 0 {
				// Single oversized tail segment — legacy truncation path.
				return safeTruncate(segs[i], 120)
			}
			break
		}
		picked = append([]string{segs[i]}, picked...)
		budget -= cost
	}
	return strings.Join(picked, " ")
}

// splitRecallSegments splits s on sentence/clause punctuation, strips filler
// prefixes, and returns deduped non-empty segments in original order.
func splitRecallSegments(s string) []string {
	raw := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '。', '！', '？', '!', '?', '；', ';', '，', ',', '…', '\n', '\r', '\t':
			return true
		}
		return false
	})
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, seg := range raw {
		seg = strings.TrimSpace(seg)
		for _, f := range recallFillerPrefixes {
			if rest := strings.TrimPrefix(seg, f); rest != seg {
				seg = strings.TrimSpace(rest)
				break // strip at most one prefix per segment
			}
		}
		if seg == "" || seen[seg] {
			continue
		}
		seen[seg] = true
		out = append(out, seg)
	}
	return out
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
