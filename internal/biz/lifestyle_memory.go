package biz

import "strings"

// lifestyleMemoryMarkers are preference/chatter snippets that must not leak
// into a work-request recall (S05: 高危运维召回「日料/寿司」). Shared by
// prompt injection and the memory_search tool path.
var lifestyleMemoryMarkers = []string{
	"日料", "寿司", "聚餐", "火锅", "奶茶",
	"喜欢吃", "爱吃", "听什么歌", "什么音乐", "追剧",
	"周末去", "爱好是", "喜欢看",
}

// LifestyleMemoryText reports whether a memory line (or the user query) is
// lifestyle/preference chatter rather than task context.
func LifestyleMemoryText(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	for _, m := range lifestyleMemoryMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// ShouldDropLifestyleMemories reports whether lifestyle hits should be
// stripped for this user query: the turn is a work request, and the query
// itself does not mention those lifestyle terms (the user asked about them).
func ShouldDropLifestyleMemories(query string) bool {
	if !HasTaskActionSignal(query) {
		return false
	}
	return !LifestyleMemoryText(query)
}
