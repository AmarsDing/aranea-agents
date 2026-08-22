package knowledge

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const summaryCardMaxRunes = 160

// DeriveSummaryCard builds a non-LLM summary card from document body.
// LLM-generated cards (when available) overwrite these fields later;
// empty-LLM ingest still gets a one-line preview so the workbench is not blank.
func DeriveSummaryCard(body, relPath, source string) (summary, docType, summaryHash string) {
	text := strings.TrimSpace(stripYAMLFrontmatter(body))
	docType = inferDocType(relPath, source)
	if text == "" {
		return "", docType, ""
	}
	return clipRunes(firstCardLine(text), summaryCardMaxRunes), docType, HashContent(text)
}

func inferDocType(relPath, source string) string {
	p := strings.ToLower(relPath + " " + source)
	switch {
	case strings.Contains(p, "faq"):
		return "faq"
	case strings.Contains(p, "report") || strings.Contains(p, "财报"):
		return "report"
	case strings.Contains(p, "manual") || strings.Contains(p, "手册"):
		return "manual"
	default:
		return "note"
	}
}

func firstCardLine(text string) string {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line
	}
	return text
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func stripYAMLFrontmatter(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := strings.TrimPrefix(s, "---")
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return s
	}
	after := rest[idx+4:]
	return strings.TrimLeft(after, "\r\n")
}
