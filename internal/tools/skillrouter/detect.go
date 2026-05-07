package skillrouter

import (
	"regexp"
	"sort"
	"strings"
)

var (
	reFileTypeHint = regexp.MustCompile(`(?i)file_type\s*[:：]\s*([a-z0-9._-]+)`)
	reDomainHint   = regexp.MustCompile(`(?i)domain\s*[:：]\s*([a-z0-9._-]+)`)
)

// DetectIntentPaths scores taxonomy leaves by keyword overlap with the user query and returns top paths.
func DetectIntentPaths(query string, maxLeaves int) []string {
	query = strings.TrimSpace(query)
	if query == "" || maxLeaves <= 0 {
		return nil
	}
	qLower := strings.ToLower(query)
	var hits []struct {
		path  string
		score int
	}
	for _, leaf := range TaxonomyLeaves() {
		path := strings.TrimSpace(leaf.Path)
		if path == "" {
			continue
		}
		sc := keywordHits(qLower, leaf.Keywords)
		if strings.Contains(qLower, strings.ToLower(path)) {
			sc += 10
		}
		if sc > 0 {
			hits = append(hits, struct {
				path  string
				score int
			}{path: path, score: sc})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].path < hits[j].path
	})
	out := make([]string, 0, maxLeaves)
	seen := map[string]bool{}
	for _, h := range hits {
		if len(out) >= maxLeaves {
			break
		}
		if seen[h.path] {
			continue
		}
		seen[h.path] = true
		out = append(out, h.path)
	}
	return out
}

func keywordHits(queryLower string, kws []string) int {
	n := 0
	for _, kw := range kws {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw == "" {
			continue
		}
		if strings.Contains(queryLower, kw) {
			n++
		}
	}
	return n
}

// ExtractTagHints parses lightweight `file_type:*` / `domain:*` hints from natural language.
func ExtractTagHints(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(token string) {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" || seen[token] {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	for _, sm := range reFileTypeHint.FindAllStringSubmatch(query, -1) {
		if len(sm) > 1 {
			add("file_type:" + strings.ToLower(strings.TrimSpace(sm[1])))
		}
	}
	for _, sm := range reDomainHint.FindAllStringSubmatch(query, -1) {
		if len(sm) > 1 {
			add("domain:" + strings.ToLower(strings.TrimSpace(sm[1])))
		}
	}
	q := strings.ToLower(query)
	if strings.Contains(q, "xlsx") {
		add("file_type:xlsx")
	}
	return out
}
