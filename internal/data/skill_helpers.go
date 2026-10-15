package data

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

func invocationTimeGTE(threshold string) predicate.SkillInvocation {
	return predicate.SkillInvocation(func(s *entsql.Selector) {
		s.Where(entsql.ExprP(
			"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") >= ?",
			threshold,
		))
	})
}

func invocationTimeRange(from, to string) []predicate.SkillInvocation {
	out := []predicate.SkillInvocation{}
	if strings.TrimSpace(from) != "" {
		out = append(out, predicate.SkillInvocation(func(s *entsql.Selector) {
			s.Where(entsql.ExprP(
				"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") >= ?",
				from,
			))
		}))
	}
	if strings.TrimSpace(to) != "" {
		out = append(out, predicate.SkillInvocation(func(s *entsql.Selector) {
			s.Where(entsql.ExprP(
				"COALESCE(NULLIF("+s.C(skillinvocation.FieldStartedAt)+", ''), "+s.C(skillinvocation.FieldCreatedAt)+") <= ?",
				to,
			))
		}))
	}
	return out
}

func escapeJSONStringValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func parseSkillTags(raw string) []biz.SkillTag {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []biz.SkillTag{}
	}
	var envelope struct {
		Tags []biz.SkillTag `json:"tags"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Tags) > 0 {
		return normalizeSkillTags(envelope.Tags)
	}
	var tags []biz.SkillTag
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		return normalizeSkillTags(tags)
	}
	return []biz.SkillTag{}
}

func normalizeSkillTags(tags []biz.SkillTag) []biz.SkillTag {
	result := []biz.SkillTag{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag.Name = strings.TrimSpace(tag.Name)
		if tag.Name == "" || seen[strings.ToLower(tag.Name)] {
			continue
		}
		if tag.Source == "" {
			tag.Source = "user"
		}
		seen[strings.ToLower(tag.Name)] = true
		result = append(result, tag)
	}
	return result
}

func parseTaxonomyPathsFromJSON(lg loggateway.Logger, blob string) []string {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil
	}
	var wrap struct {
		TaxonomyPaths []string `json:"taxonomy_paths"`
	}
	if err := json.Unmarshal([]byte(blob), &wrap); err != nil {
		lg.Warn("unmarshal taxonomy_paths failed", loggateway.StepID("data.skill"), loggateway.Err(err))
		return nil
	}
	if len(wrap.TaxonomyPaths) == 0 {
		return nil
	}
	out := make([]string, 0, len(wrap.TaxonomyPaths))
	seen := map[string]bool{}
	for _, p := range wrap.TaxonomyPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func mergeTaxonomyPaths(lg loggateway.Logger, meta, cfg string) []string {
	a := parseTaxonomyPathsFromJSON(lg, meta)
	b := parseTaxonomyPathsFromJSON(lg, cfg)
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, p := range append(append([]string{}, a...), b...) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func previewSkillBody(body string, limit int) string {
	body = strings.TrimSpace(body)
	if limit <= 0 {
		return body
	}
	runes := []rune(body)
	if len(runes) <= limit {
		return body
	}
	return string(runes[:limit]) + "..."
}

func isPathWithinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) || absPath == absRoot
}
