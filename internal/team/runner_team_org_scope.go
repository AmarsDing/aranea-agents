package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// resolveTeamOrgScope 计算团队 run 的 org scope 并集（G-08，ADR-KN-20261020-D）：
// 借调成员把本部门读权带进队伍（M78 借调语义），故 stamp 的是 ENABLED 成员的
// {position, department} 并集（覆盖 runCtx 中可能存在的调用方 scope——团队读面
// 由花名册定义）。成员 lookup / 挂岗解析失败仅丢该成员 scope、记 warn，不阻断 run
// （该成员 fail-closed，不传染全队）。
//
// getNode 为 nil（OrganizationUC 未接线，如离线/测试装配）或全队零挂岗时返回 nil
// （不 stamp）。
func (r *Runner) resolveTeamOrgScope(ctx context.Context, def Definition, getNode biz.KnowledgeOrgScopeGetter) []string {
	if getNode == nil {
		return nil
	}
	seen := map[string]struct{}{}
	// 节点级 memo（审查 C2）：同部门成员的 parent 节点查询共享一次命中，
	// 失败不缓存（瞬时错误下一位成员重试，fail-closed 语义不变）。
	nodeCache := map[string]biz.OrganizationNode{}
	cachedGet := func(c context.Context, id string) (biz.OrganizationNode, error) {
		if n, ok := nodeCache[id]; ok {
			return n, nil
		}
		n, err := getNode(c, id)
		if err != nil {
			return biz.OrganizationNode{}, err
		}
		nodeCache[id] = n
		return n, nil
	}
	var scope []string
	for _, m := range EnabledMembers(def) {
		agentID := strings.TrimSpace(m.AgentID)
		if agentID == "" {
			continue
		}
		ag, err := r.lookupAgent(ctx, agentID)
		if err != nil {
			r.lg.Warn("team org scope: member agent lookup failed, member excluded from org scope",
				loggateway.StepID("team.run.org_scope_member_lookup_fail"),
				loggateway.Str("agent_id", agentID),
				loggateway.Err(err))
			continue
		}
		for _, id := range biz.ResolveKnowledgeOrgScope(ctx, cachedGet, ag.PositionID) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			scope = append(scope, id)
		}
	}
	return scope
}
