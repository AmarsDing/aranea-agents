package vector

import "strings"

// FactVectorContentPrefix returns the stable content prefix for one fact id.
func FactVectorContentPrefix(factID string) string {
	return "fact_id:" + strings.TrimSpace(factID) + "\n"
}

// ParseFactVectorContent splits vector content into fact id and statement text.
func ParseFactVectorContent(content string) (factID, statement string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "fact_id:") {
		return "", content
	}
	rest := content[len("fact_id:"):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		return strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+1:])
	}
	return strings.TrimSpace(rest), ""
}

func factVectorContent(factID, statement string) string {
	return FactVectorContentPrefix(factID) + strings.TrimSpace(statement)
}
