package biz

import "unicode/utf8"

// Member first-turn prefix budgets (runes). See design R14.
const (
	MemberPrefixBudgetRunes = 6000
	BriefBudgetRunes        = 2000
	KnowledgeBudgetRunes    = 2000
	MemoryBudgetRunes       = 1000
	ProtocolBudgetRunes     = 1000
)

// PrefixBudget is the four-segment member first-turn prefix.
type PrefixBudget struct {
	Brief     string
	Knowledge string
	Memory    string
	Protocol  string
}

func clipRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func (p PrefixBudget) totalRunes() int {
	return utf8.RuneCountInString(p.Brief) + utf8.RuneCountInString(p.Knowledge) +
		utf8.RuneCountInString(p.Memory) + utf8.RuneCountInString(p.Protocol)
}

// TrimPrefixBudget enforces per-segment caps then the 6KB combined hard cap.
// Overflow shrinks knowledge first, then memory; Brief is never cut for overflow.
func TrimPrefixBudget(in PrefixBudget) PrefixBudget {
	return trimPrefixBudgetWithLimit(in, MemberPrefixBudgetRunes)
}

func trimPrefixBudgetWithLimit(in PrefixBudget, limit int) PrefixBudget {
	out := PrefixBudget{
		Brief:     clipRunes(in.Brief, BriefBudgetRunes),
		Knowledge: clipRunes(in.Knowledge, KnowledgeBudgetRunes),
		Memory:    clipRunes(in.Memory, MemoryBudgetRunes),
		Protocol:  clipRunes(in.Protocol, ProtocolBudgetRunes),
	}
	for out.totalRunes() > limit {
		if n := utf8.RuneCountInString(out.Knowledge); n > 0 {
			need := out.totalRunes() - limit
			if need < 1 {
				need = 1
			}
			out.Knowledge = clipRunes(out.Knowledge, n-need)
			continue
		}
		if n := utf8.RuneCountInString(out.Memory); n > 0 {
			need := out.totalRunes() - limit
			if need < 1 {
				need = 1
			}
			out.Memory = clipRunes(out.Memory, n-need)
			continue
		}
		break
	}
	return out
}
