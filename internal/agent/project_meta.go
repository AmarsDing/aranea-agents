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

	// ParentActivityID is the activity ID that child activities (thinking/action/reply)
	// should use as their ParentActivityID instead of the turn's root task ID.
	// When set (non-empty), the ActivityProjector uses this value for all child
	// activities created during the turn. This enables team member activities to
	// be nested under the session activity (kind=session) in the spirit session's
	// activity tree, rather than under the child session's own root task.
	//
	// Set by the team runner to SessionActivityID(teamID, agentKey) so that
	// member thinking/action/reply are children of the session activity.
	ParentActivityID string

	// ParentTaskID is set by the system-push pattern (e.g. synthesis trigger
	// after all teams complete) to attach the new Turn to an existing Task
	// instead of creating a new one. Empty for normal user-input turns.
	// Design: docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.2.1
	ParentTaskID string

	// NodeIDToAgentKey maps graph node IDs (e.g. "member-1") to member agent
	// keys (e.g. "spirit-worker-a"). Used by V2Projector.ProcessEvent to
	// attribute Steps to the correct member agent when GraphAgent executes
	// multiple member agents through a single projector.
	//
	// 2026-07-04 问题 1 修复：Graph 模式下所有 member agent 事件被错误归到
	// anchor agent 名下，前端 MemberSessionPanel 匹配不到成员活动。
	NodeIDToAgentKey map[string]string
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
