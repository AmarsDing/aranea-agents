package tools

import "strings"

// ToolSafety classifies a tool's concurrency safety.
// ConcurrentSafe tools are read-only and safe to parallelize and cache.
// Exclusive tools modify state and must be serialized; they are never cached.
type ToolSafety int

const (
	// SafetyConcurrentSafe indicates a read-only tool safe for parallel
	// execution and deterministic result caching.
	SafetyConcurrentSafe ToolSafety = iota
	// SafetyExclusive indicates a state-modifying tool that must be
	// serialized and never cached.
	SafetyExclusive
)

// ClassifyTool returns the safety classification for a tool by name.
// The classification is derived from the registry's SupportsConcurrency
// field. Unknown tools default to SafetyExclusive (safe default).
func ClassifyTool(name string) ToolSafety {
	name = strings.TrimSpace(name)
	if name == "" {
		return SafetyExclusive
	}
	for _, reg := range Registry() {
		if strings.EqualFold(reg.Name, name) {
			if reg.SupportsConcurrency {
				return SafetyConcurrentSafe
			}
			return SafetyExclusive
		}
	}
	// Unknown tools default to Exclusive (safe default).
	return SafetyExclusive
}

// IsCacheable returns true if the tool's result can be safely cached.
// Only ConcurrentSafe tools are cacheable.
func IsCacheable(name string) bool {
	return ClassifyTool(name) == SafetyConcurrentSafe
}
