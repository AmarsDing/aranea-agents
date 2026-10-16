package review

import (
	"strings"
	"unicode/utf8"
)

// DefaultSoftRules are MVP soft ad-law / absolute wording hints (zh).
// Default behaviour: flag only — never auto-reject.
var DefaultSoftRules = []string{
	"第一", "绝对", "必赚", "稳赚", "100%", "国家级", "最好", "最强", "包治", "根治",
}

// SoftScan returns machine_flags for draft body (non-blocking).
func SoftScan(body string, rules []string) []string {
	if len(rules) == 0 {
		rules = DefaultSoftRules
	}
	lower := body
	flags := make([]string, 0)
	seen := map[string]bool{}
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		if strings.Contains(lower, r) {
			seen[r] = true
			flags = append(flags, "soft:"+r)
		}
	}
	if utf8.RuneCountInString(body) > 5000 {
		flags = append(flags, "soft:too_long")
	}
	return flags
}
