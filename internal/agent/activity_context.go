package agent

import (
	"context"

	"github.com/google/uuid"
)

// Phase 5 (typed ID consistency): distinct string types for each Activity ID
// flavor. Constructor functions return typed values; callers must explicitly
// convert to `string` when assigning to polymorphic string fields (e.g.
// `Activity.ID`, `Activity.ParentActivityID`). This provides compile-time
// safety against mixing up ID-generating functions — passing a
// TeamStageActivityID where GraphStageActivityID is expected is now a
// compile error instead of a silent runtime bug.
//
// The underlying type is `string` so the conversion is zero-cost; the
// safety is purely at the API boundary.

// RootTaskActivityID is the activity ID of the root task (user message).
// Sourced from ctx via RootTaskActivityIDFromCtx.
type RootTaskActivityID string

// GraphStageActivityID is the deterministic graph_stage activity ID derived
// from a spirit session ID. Shared between spirit_team (service) and the
// team runner so both compute the same ID for parent-child nesting.
type GraphStageActivityID string

// TeamStageActivityID is the deterministic team_stage activity ID derived
// from a team ID. Shared between spirit_team assembler and team runner.
type TeamStageActivityID string

// SessionActivityID is the deterministic member-session activity ID derived
// from a team ID + agent key. Shared between team runner (parent) and the
// child session's ActivityProjector (so member thinking/action/reply events
// parent correctly).
type SessionActivityID string

// rootTaskActivityIDKey is the context key for the root task activity ID.
// The ActivityProjector sets this in OnTurnStart so that downstream
// business orchestrators (spirit_team, team runner) can use it as the
// ParentActivityID for direct-publish events (team_stage, graph_stage,
// session), ensuring the frontend activity tree is correctly nested.
type rootTaskActivityIDKey struct{}

// ContextWithRootTaskActivityID returns a new context with the root task
// activity ID stored.
func ContextWithRootTaskActivityID(ctx context.Context, id RootTaskActivityID) context.Context {
	return context.WithValue(ctx, rootTaskActivityIDKey{}, id)
}

// RootTaskActivityIDFromCtx extracts the root task activity ID from the
// context. Returns zero value if not set.
func RootTaskActivityIDFromCtx(ctx context.Context) RootTaskActivityID {
	if v, ok := ctx.Value(rootTaskActivityIDKey{}).(RootTaskActivityID); ok {
		return v
	}
	return ""
}

// graphStageNamespace is a fixed UUID used as the namespace for deterministic
// graph_stage Activity IDs. Using deterministic IDs allows the team runner and
// spirit_team assembler to compute the same graph_stage activity ID and use it
// as ParentActivityID for team_stage events, ensuring the frontend activity
// tree has the correct 3-level nesting: graph_stage → team_stage → session.
var graphStageNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v1"))

// NewGraphStageActivityID returns the deterministic graph_stage Activity ID
// for a given spirit session ID. Both spirit_team (service) and team runner
// use this to compute the same ID, enabling team_stage events to be
// correctly parented under graph_stage.
func NewGraphStageActivityID(spiritSessionID string) GraphStageActivityID {
	return GraphStageActivityID(uuid.NewSHA1(graphStageNamespace, []byte(spiritSessionID)).String())
}

// teamStageNamespace is a fixed UUID used as the namespace for deterministic
// team_stage Activity IDs. Using deterministic IDs allows the team runner to
// compute the same team_stage activity ID and pass it to the child session's
// ActivityProjector as the ParentActivityID for member thinking/action/reply
// activities.
var teamStageNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_stage.v1"))

// NewTeamStageActivityID returns the deterministic team_stage Activity ID
// for a given team ID. Both spirit_team (service) and team runner (team
// package) use this to compute the same ID.
func NewTeamStageActivityID(teamID string) TeamStageActivityID {
	return TeamStageActivityID(uuid.NewSHA1(teamStageNamespace, []byte(teamID)).String())
}

// sessionActivityNamespace is a fixed UUID used as the namespace for
// deterministic session Activity IDs. Using deterministic IDs allows the
// team runner to compute the same session activity ID and pass it to the
// child session's ActivityProjector as the ParentActivityID for member
// thinking/action/reply activities.
var sessionActivityNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.session_activity.v1"))

// NewSessionActivityID returns the deterministic session Activity ID for a
// given team and agent key. Both the team runner (publishTeamStepActivity)
// and the child session's ActivityProjector use this to compute the same ID.
func NewSessionActivityID(teamID, agentKey string) SessionActivityID {
	return SessionActivityID(uuid.NewSHA1(sessionActivityNamespace, []byte(teamID+":"+agentKey)).String())
}

// sessionActivityIDKey is the context key for the session activity ID.
// The team runner sets this in the context before starting a member session,
// so the child session's ActivityProjector can use it as the ParentActivityID
// for member thinking/action/reply activities.
type sessionActivityIDKey struct{}

// ContextWithSessionActivityID returns a new context with the session
// activity ID stored.
func ContextWithSessionActivityID(ctx context.Context, id SessionActivityID) context.Context {
	return context.WithValue(ctx, sessionActivityIDKey{}, id)
}

// SessionActivityIDFromCtx extracts the session activity ID from the context.
// Returns zero value if not set.
func SessionActivityIDFromCtx(ctx context.Context) SessionActivityID {
	if v, ok := ctx.Value(sessionActivityIDKey{}).(SessionActivityID); ok {
		return v
	}
	return ""
}
