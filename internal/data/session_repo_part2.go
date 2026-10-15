package data

import (
	"aranea-agents/internal/biz"
	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/apierror"
	"context"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
)

// GetSessionTree returns the complete session tree (arbitrary depth) rooted at
// the given spirit session ID. Uses a single query on root_session_id index,
// then builds the recursive tree in memory to avoid N recursive queries.
func (r *sessionRepo) GetSessionTree(ctx context.Context, spiritSessionID string) (*biz.SessionTree, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SESSION", "spirit_session_id is required")
	}
	c := r.data.RW().Read(ctx)

	rows, err := c.Session.Query().
		Where(
			entsession.Or(
				entsession.RootSessionIDEQ(spiritSessionID),
				entsession.IDEQ(spiritSessionID),
			),
			entsession.DeletedAtEQ(""),
		).
		Order(entsession.ByAgentDepth(entsql.OrderAsc()), entsession.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}

	tree := &biz.SessionTree{}
	nodeMap := make(map[string]*biz.SessionTreeNode, len(rows))

	for _, s := range rows {

		if s.ID == spiritSessionID {
			tree.Root = entSessionToBiz(s)
			continue
		}
		node := &biz.SessionTreeNode{Session: entSessionToBiz(s)}
		nodeMap[s.ID] = node

		if parent, ok := nodeMap[s.ParentSessionID]; ok {
			parent.Children = append(parent.Children, node)
		} else {

			tree.Children = append(tree.Children, node)
		}
	}
	return tree, nil
}

// ListChildSessions returns direct child sessions (single level, non-recursive).
func (r *sessionRepo) ListChildSessions(ctx context.Context, parentSessionID string) ([]biz.Session, error) {
	return r.ListByParentSessionID(ctx, parentSessionID)
}

// ListTeamAgentSessions returns all agent-type sessions under a team
// (members and their sub-agents).
func (r *sessionRepo) ListTeamAgentSessions(ctx context.Context, teamID string) ([]biz.Session, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, apierror.BadRequest("SESSION", "team_id is required")
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.Session.Query().
		Where(
			entsession.TeamIDEQ(teamID),
			entsession.SessionTypeEQ(string(biz.SessionTypeAgent)),
			entsession.DeletedAtEQ(""),
		).
		Order(entsession.ByAgentDepth(entsql.OrderAsc()), entsession.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	out := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSessionToBiz(row))
	}
	return out, nil
}
