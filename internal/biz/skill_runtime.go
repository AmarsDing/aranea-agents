package biz

import (
	"encoding/json"
	"strings"
)

// SkillRuntimePolicy configures how many published Skills are exposed as ADK tools for an agent turn.
// Stored in agent_runtime_settings.skill_runtime_json.
type SkillRuntimePolicy struct {
	AllowedSlugs []string `json:"allowed_slugs"`
	DeniedSlugs  []string `json:"denied_slugs"`
	// AllowedTags: conjunctive filter — a Skill must carry every listed token (matched against SkillTag.Name, case-insensitive).
	AllowedTags []string `json:"allowed_tags"`

	IntentRoutingEnabled bool `json:"intent_routing_enabled"`
	IntentMaxPaths       int  `json:"intent_max_paths"`
	MaxSkillsInToolset   int  `json:"max_skills_in_toolset"`
}

// SkillRuntimeCandidate is a lightweight Skill row for routing (slug + tags + taxonomy paths from metadata).
type SkillRuntimeCandidate struct {
	Slug          string
	Name          string
	Description   string
	Tags          []SkillTag
	TaxonomyPaths []string
}

// ParseSkillRuntimePolicy unmarshals skill_runtime_json with safe defaults.
func ParseSkillRuntimePolicy(raw string) SkillRuntimePolicy {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var wire struct {
		AllowedSlugs         []string `json:"allowed_slugs"`
		DeniedSlugs          []string `json:"denied_slugs"`
		AllowedTags          []string `json:"allowed_tags"`
		IntentRoutingEnabled *bool    `json:"intent_routing_enabled"`
		IntentMaxPaths       int      `json:"intent_max_paths"`
		MaxSkillsInToolset   int      `json:"max_skills_in_toolset"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		wire = struct {
			AllowedSlugs         []string `json:"allowed_slugs"`
			DeniedSlugs          []string `json:"denied_slugs"`
			AllowedTags          []string `json:"allowed_tags"`
			IntentRoutingEnabled *bool    `json:"intent_routing_enabled"`
			IntentMaxPaths       int      `json:"intent_max_paths"`
			MaxSkillsInToolset   int      `json:"max_skills_in_toolset"`
		}{}
	}
	p := SkillRuntimePolicy{
		AllowedSlugs: wire.AllowedSlugs,
		DeniedSlugs:  wire.DeniedSlugs,
		AllowedTags:  wire.AllowedTags,
	}
	if wire.IntentRoutingEnabled != nil {
		p.IntentRoutingEnabled = *wire.IntentRoutingEnabled
	} else {
		p.IntentRoutingEnabled = true
	}
	p.IntentMaxPaths = wire.IntentMaxPaths
	p.MaxSkillsInToolset = wire.MaxSkillsInToolset

	if p.IntentMaxPaths <= 0 {
		p.IntentMaxPaths = 3
	}
	if p.MaxSkillsInToolset <= 0 {
		p.MaxSkillsInToolset = 32
	}
	if p.MaxSkillsInToolset > 256 {
		p.MaxSkillsInToolset = 256
	}
	normalizeLowerSlice(&p.AllowedSlugs)
	normalizeLowerSlice(&p.DeniedSlugs)
	normalizeTagTokens(&p.AllowedTags)
	return p
}

func normalizeLowerSlice(s *[]string) {
	out := make([]string, 0, len(*s))
	seen := map[string]bool{}
	for _, x := range *s {
		x = strings.TrimSpace(strings.ToLower(x))
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	*s = out
}

func normalizeTagTokens(s *[]string) {
	out := make([]string, 0, len(*s))
	seen := map[string]bool{}
	for _, x := range *s {
		x = strings.TrimSpace(strings.ToLower(x))
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	*s = out
}
