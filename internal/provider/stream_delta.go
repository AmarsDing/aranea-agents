package provider

import "strings"

// VisibleStreamingDelta merges one streaming text chunk into acc and returns the suffix that should be sent as delta.
// Some backends (including certain Gemini / GenAI streaming paths) emit cumulative assistant text on each partial chunk;
// others emit token-sized deltas only. Treating every chunk as additive duplicates content when the client concatenates deltas.
// When chunk has prefix acc, only the new suffix is appended and returned; otherwise the whole chunk is treated as a delta.
func VisibleStreamingDelta(acc *strings.Builder, chunk string) string {
	if chunk == "" {
		return ""
	}
	cur := acc.String()
	if strings.HasPrefix(chunk, cur) {
		suffix := chunk[len(cur):]
		if suffix == "" {
			return ""
		}
		acc.WriteString(suffix)
		return suffix
	}
	acc.WriteString(chunk)
	return chunk
}
