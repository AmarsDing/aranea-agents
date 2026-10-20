package biz

import (
	"context"
	"strings"
)

// KnowledgeOrgScopeGetter 是 G-08（ADR-KN-20261020-F）运行期 org scope 解析所需的
// 最小组织读取端口。*OrganizationUsecase.Get 与 data 层 OrganizationReader.GetOrgNode
// 都以方法值直接满足，调用侧无需包装适配。
type KnowledgeOrgScopeGetter func(ctx context.Context, id string) (OrganizationNode, error)

// ResolveKnowledgeOrgScope 解析调用 Agent 的知识共享组织域（ADR-KN-20261020-D）：
//   - agent 挂 position 岗 → {positionID, parentDepartmentID}（当 parent 确为 department）
//   - agent 挂 department 节点（dept_lead 治理岗，dept_lead.go agentDepartment 同型）→ {departmentID}
//   - positionID 为空 / 节点缺失 / 已软删 → nil（fail-closed：shared 档可见性不回退放开）
//
// 语义边界：只沿「position → 直父 department」一跳走，不做多跳公司链展开——
// shared_with 名单只允许 department/position 两级（OrgNodeValidator 强制），
// 公司级节点出现在 scope 或名单里均无匹配语义，故不收录。
func ResolveKnowledgeOrgScope(ctx context.Context, getNode KnowledgeOrgScopeGetter, positionID string) []string {
	positionID = strings.TrimSpace(positionID)
	if positionID == "" || getNode == nil {
		return nil
	}
	node, err := getNode(ctx, positionID)
	if err != nil {
		return nil
	}
	scope := []string{node.ID}
	if node.Level == "department" {
		return scope
	}
	parentID := strings.TrimSpace(node.ParentID)
	if parentID == "" {
		return scope
	}
	parent, err := getNode(ctx, parentID)
	if err != nil || parent.Level != "department" {
		return scope
	}
	return append(scope, parent.ID)
}
