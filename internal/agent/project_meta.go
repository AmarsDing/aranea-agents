package agent

import "strings"

// ProjectMeta carries the per-turn metadata required by ActivityProjector
// (and historically by the deprecated EventProjector) to project runtime
// events into Activity records and WS envelopes.
//
// All fields are set once at turn start via ActivityProjector.Configure and
// treated as read-only for the duration of the turn.
type ProjectMeta struct {
	SessionID          string
	RequestID          string
	InvocationID       string
	ParentInvocationID string
	TeamID             string
	Branch             string
	FilterKey          string
	RunID              string
	TraceID            string
	AgentID            string
	AgentDisplayName   string
	ContextWindow      int
	TurnPromptTokens   int
	TurnCompletionTok  int
	MemberAgentKeys    map[string]struct{} // agent_key set for team member_* envelopes
	Source             string
	TaskContent        string // User input text for the root task Activity

	// === Session tree hierarchy (Phase 1a) ===
	// SpiritSessionID is the root spirit session ID for cross-session aggregation.
	// When this Activity belongs to a team/agent sub-session, SpiritSessionID
	// points to the spirit session that initiated the tree. When this Activity
	// belongs to the spirit session itself, SpiritSessionID == SessionID.
	SpiritSessionID string
	// ParentSessionID is the immediate parent session ID (empty for spirit root).
	ParentSessionID string
	// RootSessionID is the root session ID of the tree (== SpiritSessionID for spirit-initiated trees).
	RootSessionID string
}

// isTeamMemberAuthor reports whether the given author string identifies a
// team member agent (as opposed to the team orchestrator itself or a
// non-team author). It is used by both the stream consumer (for legacy
// member tool call tracking) and ActivityProjector (for member text
// projection).
func isTeamMemberAuthor(author string, meta ProjectMeta) bool {
	if meta.TeamID == "" || strings.TrimSpace(author) == "" {
		return false
	}
	if strings.HasPrefix(author, "team") {
		return false
	}
	if len(meta.MemberAgentKeys) == 0 {
		return true
	}
	_, ok := meta.MemberAgentKeys[author]
	return ok
}

// coalesceStr returns the first non-empty string. Used by activity_persist.go
// and other projection logic to pick a display value from multiple candidates.
func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
