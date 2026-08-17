package argnorm

import (
	"encoding/json"
	"strings"
)

func needsNorm(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "web_fetch", "httpfetch",
		"gemini_web_fetch", "gemini_fetch",
		"duckduckgo_search", "duckduckgo",
		"wikipedia_search", "wikipedia",
		"web_research", "web_search",
		"knowledge_search",
		"google_search", "arxiv_search", "search":
		return true
	default:
		return false
	}
}

// NormalizeArgs maps common LLM aliases onto standalone tool schemas.
// Unknown tools pass through unchanged (no JSON parse).
func NormalizeArgs(toolName string, jsonArgs []byte) []byte {
	name := strings.TrimSpace(toolName)
	if name == "" || len(jsonArgs) == 0 || !needsNorm(name) {
		return jsonArgs
	}
	switch name {
	case "web_fetch", "httpfetch":
		return rewrite(jsonArgs, coerceWebFetchURLs)
	case "gemini_web_fetch", "gemini_fetch":
		return rewrite(jsonArgs, coerceGeminiPrompt)
	default:
		return rewrite(jsonArgs, coerceSearchQuery)
	}
}

func rewrite(jsonArgs []byte, fn func(map[string]any) bool) []byte {
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil || len(m) == 0 {
		return jsonArgs
	}
	if !fn(m) {
		return jsonArgs
	}
	out, err := json.Marshal(m)
	if err != nil {
		return jsonArgs
	}
	return out
}

func coerceWebFetchURLs(m map[string]any) bool {
	if coerceToStringSlice(m, "urls") {
		return true
	}
	if urlsPresent(m["urls"]) {
		return false
	}
	for _, key := range []string{"url", "uri", "href", "link"} {
		if s := stringValue(m[key]); s != "" {
			m["urls"] = []any{s}
			delete(m, key)
			return true
		}
		if arr := anySlice(m[key]); len(arr) > 0 {
			m["urls"] = arr
			delete(m, key)
			return true
		}
	}
	return false
}

func coerceGeminiPrompt(m map[string]any) bool {
	if copyStringIfEmpty(m, "prompt", "url", "uri", "query", "text") {
		return true
	}
	if !missingString(m, "prompt") {
		return false
	}
	if arr := anySlice(m["urls"]); len(arr) > 0 {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			s := stringValue(item)
			if s == "" {
				continue
			}
			parts = append(parts, s)
		}
		if len(parts) == 0 {
			return false
		}
		m["prompt"] = strings.Join(parts, " ")
		delete(m, "urls")
		return true
	}
	return false
}

func coerceSearchQuery(m map[string]any) bool {
	return copyStringIfEmpty(m, "query", "q", "search", "keyword", "text", "prompt")
}

func coerceToStringSlice(m map[string]any, key string) bool {
	s := stringValue(m[key])
	if s == "" {
		return false
	}
	m[key] = []any{s}
	return true
}

func urlsPresent(v any) bool {
	if v == nil {
		return false
	}
	if s := stringValue(v); s != "" {
		return true
	}
	return len(anySlice(v)) > 0
}

func anySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, 0, len(x))
		for _, s := range x {
			if strings.TrimSpace(s) == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func stringValue(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func copyStringIfEmpty(m map[string]any, dest string, srcs ...string) bool {
	if !missingString(m, dest) {
		return false
	}
	for _, src := range srcs {
		s := stringValue(m[src])
		if s == "" {
			continue
		}
		m[dest] = s
		if src != dest {
			delete(m, src)
		}
		return true
	}
	return false
}

func missingString(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return true
	}
	s, isStr := v.(string)
	if !isStr {
		return false
	}
	return strings.TrimSpace(s) == ""
}
