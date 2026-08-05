package biz

import (
	"regexp"
	"strings"
)

// FactMark represents a fact extracted from agent response via <fact> XML tags.
type FactMark struct {
	Type       string
	Confidence string
	Content    string
}

var factRegex = regexp.MustCompile(`<fact\s+type="([^"]+)"\s+confidence="([^"]+)">\s*([\s\S]*?)\s*</fact>`)

// ParseFactMarks extracts <fact> tags from agent response text.
// Returns the clean response (without tags) and extracted facts.
func ParseFactMarks(response string) (clean string, facts []FactMark) {
	matches := factRegex.FindAllStringSubmatch(response, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			facts = append(facts, FactMark{
				Type:       strings.TrimSpace(m[1]),
				Confidence: strings.TrimSpace(m[2]),
				Content:    strings.TrimSpace(m[3]),
			})
		}
	}
	// Remove tags from response for user display
	clean = factRegex.ReplaceAllString(response, "")
	clean = strings.TrimSpace(clean)
	return
}

// HasFactMarks checks if the response contains any <fact> tags.
func HasFactMarks(response string) bool {
	return strings.Contains(response, "<fact")
}

// StripFactMarks removes complete <fact>...</fact> tag pairs from text
// without extracting them. Use at display/persist boundaries where facts
// have already been extracted upstream and only the user-visible text is
// needed. Unclosed tags (e.g. truncated streams) are left untouched.
func StripFactMarks(text string) string {
	if !HasFactMarks(text) {
		return text
	}
	return strings.TrimSpace(factRegex.ReplaceAllString(text, ""))
}

// ExtractFactMarksOnly extracts facts without modifying the original response.
// Useful when you need to check facts but preserve the original text.
func ExtractFactMarksOnly(response string) []FactMark {
	var facts []FactMark
	matches := factRegex.FindAllStringSubmatch(response, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			facts = append(facts, FactMark{
				Type:       strings.TrimSpace(m[1]),
				Confidence: strings.TrimSpace(m[2]),
				Content:    strings.TrimSpace(m[3]),
			})
		}
	}
	return facts
}
