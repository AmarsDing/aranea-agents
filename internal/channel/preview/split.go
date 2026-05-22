package preview

import "strings"

// SplitPages splits text into pages each within maxRunes, preferring paragraph boundaries.
func SplitPages(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	var pages []string
	for len(runes) > 0 {
		if len(runes) <= maxRunes {
			pages = append(pages, strings.TrimSpace(string(runes)))
			break
		}
		chunk := string(runes[:maxRunes])
		cutAt := maxRunes
		if idx := strings.LastIndex(chunk, "\n\n"); idx > len(chunk)/4 {
			// idx is byte offset; recompute rune cut from safe prefix.
			cutAt = len([]rune(chunk[:idx]))
			if cutAt <= 0 {
				cutAt = maxRunes
			}
		}
		pages = append(pages, strings.TrimSpace(string(runes[:cutAt])))
		runes = runes[cutAt:]
	}
	return pages
}
