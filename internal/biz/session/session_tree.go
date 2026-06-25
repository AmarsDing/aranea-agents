package session

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
