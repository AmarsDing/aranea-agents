package document

func truncateText(text string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return text, false
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, false
	}
	return string(runes[:maxChars]), true
}

func normalizedPositive(value *int) *int {
	if value == nil || *value <= 0 {
		return nil
	}
	out := *value
	return &out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
