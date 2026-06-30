package agent

import (
	"context"

	"github.com/google/uuid"
)

// rootTaskActivityIDKey is the context key for the root task activity ID.
// The ActivityProjector sets this in OnTurnStart so that downstream
// business orchestrators (spirit_team, team runner) can use it as the
// ParentActivityID for direct-publish events (team_stage, graph_stage,
// session), ensuring the frontend activity tree is correctly nested.
type rootTaskActivityIDKey struct{}

// ContextWithRootTaskActivityID returns a new context with the root task
// activity ID stored.
func ContextWithRootTaskActivityID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, rootTaskActivityIDKey{}, id)
}

// RootTaskActivityIDFromCtx extracts the root task activity ID from the
// context. Returns empty string if not set.
func RootTaskActivityIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(rootTaskActivityIDKey{}).(string); ok {
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

// GraphStageActivityID returns the deterministic graph_stage Activity ID for a
// given spirit session ID. Both spirit_team (service) and team runner use this
// to compute the same ID, enabling team_stage events to be correctly parented
// under graph_stage.
func GraphStageActivityID(spiritSessionID string) string {
	return uuid.NewSHA1(graphStageNamespace, []byte(spiritSessionID)).String()
}

// teamStageNamespace is a fixed UUID used as the namespace for deterministic
// team_stage Activity IDs. Using deterministic IDs allows the team runner to
// compute the same team_stage activity ID and use it as ParentActivityID for
// member session events, without needing to query the database.
var teamStageNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_stage.v1"))

// TeamStageActivityID returns the deterministic team_stage Activity ID for a
// given team ID. Both spirit_team (service) and team runner (team package) use
// this to compute the same ID.
func TeamStageActivityID(teamID string) string {
	return uuid.NewSHA1(teamStageNamespace, []byte(teamID)).String()
}

// sessionActivityNamespace is a fixed UUID used as the namespace for
// deterministic session Activity IDs. Using deterministic IDs allows the
// team runner to compute the same session activity ID and pass it to the
// child session's ActivityProjector as the ParentActivityID for member
// thinking/action/reply activities.
var sessionActivityNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.session_activity.v1"))

// SessionActivityID returns the deterministic session Activity ID for a
// given team and agent key. Both the team runner (publishTeamStepActivity)
// and the child session's ActivityProjector use this to compute the same ID.
func SessionActivityID(teamID, agentKey string) string {
	return uuid.NewSHA1(sessionActivityNamespace, []byte(teamID+":"+agentKey)).String()
}

// sessionActivityIDKey is the context key for the session activity ID.
// The team runner sets this in the context before starting a member session,
// so the child session's ActivityProjector can use it as the ParentActivityID
// for member thinking/action/reply activities.
type sessionActivityIDKey struct{}

// ContextWithSessionActivityID returns a new context with the session
// activity ID stored.
func ContextWithSessionActivityID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionActivityIDKey{}, id)
}

// SessionActivityIDFromCtx extracts the session activity ID from the context.
// Returns empty string if not set.
func SessionActivityIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(sessionActivityIDKey{}).(string); ok {
		return v
	}
	return ""
}