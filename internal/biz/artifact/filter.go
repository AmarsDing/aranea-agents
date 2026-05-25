package artifact

import "strings"

// FilterArtifacts returns items matching query (name / mime / session) and mime prefix.
func FilterArtifacts(items []Artifact, query, mimePrefix string) []Artifact {
	query = strings.ToLower(strings.TrimSpace(query))
	mimePrefix = strings.ToLower(strings.TrimSpace(mimePrefix))
	if query == "" && mimePrefix == "" {
		return items
	}
	out := make([]Artifact, 0, len(items))
	for _, it := range items {
		mime := strings.ToLower(strings.TrimSpace(it.MimeType))
		if mimePrefix != "" && !strings.HasPrefix(mime, mimePrefix) {
			continue
		}
		if query != "" {
			hay := strings.ToLower(it.Name + " " + it.MimeType + " " + it.SessionID)
			if !strings.Contains(hay, query) {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}
