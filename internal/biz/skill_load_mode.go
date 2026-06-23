package biz

import "strings"

const (
	// SkillLoadModeProgressive is an Aranea-specific composite marker, not a
	// framework load mode. The framework's normalizeSkillLoadMode does not
	// recognize "progressive" and falls back to SkillLoadModeTurn.
	//
	// When this mode is active, Aranea additionally enables:
	//   - Tool result mode (WithSkillsLoadedContentInToolResults)
	//   - Directory hints
	//   - BeforeModel hook that writes routed slugs to invocation state and
	//     injects a "## Routed Skills" system message (the [routed] marker
	//     equivalent, since the framework's injectOverview does not support
	//     [routed] markers natively).
	SkillLoadModeProgressive = "progressive"
)

func IsProgressiveSkillLoad(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), SkillLoadModeProgressive)
}
