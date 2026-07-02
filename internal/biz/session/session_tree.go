package session

import "fmt"

// SessionType classifies a session's role in the session tree hierarchy.
//
// The tree supports arbitrary depth:
//   - spirit (depth 0): root session, user-facing
//   - team (depth 1+): team session created by SpiritTeamAssembler
//   - agent (depth 1+): agent session (team member, spirit-direct, or sub-agent)
//   - standalone: non-spirit sessions (direct chat without spirit orchestration)
//
// The legacy "member" type is unified into "agent" to support arbitrary nesting
// (team → agent → agent → ...).
type SessionType string

const (
	// SessionTypeSpirit is the root session a user interacts with directly.
	SessionTypeSpirit SessionType = "spirit"
	// SessionTypeTeam is a team session created by SpiritTeamAssembler.
	SessionTypeTeam SessionType = "team"
	// SessionTypeAgent is an agent session (team member, spirit-direct, sub-agent).
	SessionTypeAgent SessionType = "agent"
	// SessionTypeStandalone is a non-spirit session (direct chat).
	SessionTypeStandalone SessionType = "standalone"
)

// SessionTree is the recursive tree structure returned by GetSessionTree.
// It contains the root spirit session and its direct children (team or agent
// sessions). Each child may have its own children, supporting arbitrary depth.
type SessionTree struct {
	Root     Session            // Spirit session (root of the tree)
	Children []*SessionTreeNode // Direct children (team or agent sessions)
}

// SessionTreeNode is a node in the session tree, supporting arbitrary depth.
// Each node contains a session and its child nodes (recursively).
//
// Activities are not embedded here to avoid an import cycle on internal/biz.
// Callers load activities separately via biz.ActivityReader.ListBySession
// when a node is expanded in the UI.
type SessionTreeNode struct {
	Session  Session            // Current session (team or agent)
	Children []*SessionTreeNode // Child nodes (recursive, supports arbitrary depth)
}

// DepthValidationConfig bundles the depth limits required by ValidateDepth.
//
// Two layers of depth limits are enforced:
//   - SpiritMaxDepth (absolute): bounds the total session tree depth
//     rooted at a spirit session. Configured via Spirit ParallelConfig.
//   - AgentMaxRelativeDepth (relative): bounds how many levels of sub-agents
//     an agent may spawn below itself. Configured per-agent via
//     AgentRuntimeSettings.SubagentsMaxGenerationDepth. Zero means
//     "no sub-agents allowed" (the agent cannot create child agent sessions).
//
// When AgentMaxRelativeDepth > 0, the agent at parentSession.MemberAgentKey
// is allowed to spawn children up to that relative depth below itself.
// A value of 0 disables agent-level sub-session creation entirely.
type DepthValidationConfig struct {
	SpiritMaxDepth        int // absolute max tree depth (spirit ParallelConfig)
	AgentMaxRelativeDepth int // relative max sub-agent depth (AgentRuntimeSettings)
}

// ValidateDepth checks whether a child session can be created at childDepth
// under parentSession, given the configured depth limits (P1-4).
//
// Checks performed:
//  1. Spirit-level absolute depth: childDepth must not exceed SpiritMaxDepth.
//  2. Agent-level relative depth: when parentSession.MemberAgentKey is set
//     (i.e., parent is an agent session), the relative depth
//     (childDepth - parentSession.AgentDepth) must not exceed
//     AgentMaxRelativeDepth.
//
// Returns nil if the depth is acceptable, or an error describing the
// violated limit. The error is suitable for wrapping with apierror.BadRequest.
func ValidateDepth(parentSession Session, childDepth int, cfg DepthValidationConfig) error {
	if childDepth > cfg.SpiritMaxDepth {
		return fmt.Errorf("session tree depth (%d) exceeds spirit max (%d)",
			childDepth, cfg.SpiritMaxDepth)
	}
	// Agent-level relative depth check (only when parent is an agent session).
	if parentSession.MemberAgentKey != "" && cfg.AgentMaxRelativeDepth > 0 {
		relativeDepth := childDepth - parentSession.AgentDepth
		if relativeDepth > cfg.AgentMaxRelativeDepth {
			return fmt.Errorf("subagent generation depth (%d) exceeds agent max (%d)",
				relativeDepth, cfg.AgentMaxRelativeDepth)
		}
	}
	return nil
}
