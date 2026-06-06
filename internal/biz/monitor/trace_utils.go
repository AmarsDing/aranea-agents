package monitor

// TraceSpansRaw extracts the spans array from a trace config map.
func TraceSpansRaw(config map[string]any) []any {
	if spans, ok := config["spans"].([]any); ok {
		return spans
	}
	if trace, ok := config["trace"].(map[string]any); ok {
		if spans, ok := trace["spans"].([]any); ok {
			return spans
		}
	}
	return []any{}
}
