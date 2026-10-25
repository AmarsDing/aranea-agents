package biz

import (
	"regexp"
	"strings"
)

var evalRuleCall = regexp.MustCompile(`(?i)\b([a-z][a-z0-9_]{0,31})\s*\(`)

// EvalRuleAnchors keeps a function identifier even when ordinary tokenization
// discards its one-letter name. Arguments never become an inferred answer.
func EvalRuleAnchors(query string) []string {
	var out []string
	seen := map[string]bool{}
	for _, match := range evalRuleCall.FindAllStringSubmatch(query, 8) {
		switch strings.ToLower(match[1]) {
		case "if", "when", "where", "what", "which", "who", "and", "or", "not", "on", "in", "at":
			continue
		}
		anchor := strings.ToLower(match[1]) + "("
		if !seen[anchor] {
			out = append(out, anchor)
			seen[anchor] = true
		}
	}
	return out
}

// EvalRetrievalQueries preserves an independent original-query recall leg.
// The optional second leg treats all choices equally as retrieval hints. It
// neither chooses an option nor adds option text to memory evidence.
func EvalRetrievalQueries(query string, options ...string) []string {
	queries := []string{query}
	seen := map[string]bool{strings.TrimSpace(query): true}
	var hints []string
	runes := 0
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" || seen[option] {
			continue
		}
		seen[option] = true
		part := []rune(option)
		if len(part) > 1024 {
			part = part[:1024]
		}
		if runes+len(part) > 8192 || len(hints) >= 16 {
			break
		}
		runes += len(part)
		hints = append(hints, string(part))
	}
	if len(hints) > 0 {
		queries = append(queries, query+"\n"+strings.Join(hints, "\n"))
	}
	return queries
}
