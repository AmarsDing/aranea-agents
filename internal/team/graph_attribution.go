package team

import (
	"strings"

	"aranea-agents/internal/biz"
)

// buildAttributionFromCompiledTeam derives member attribution maps (node→member,
// node→step sort index, observatory registry) from the actually executed graph
// config — the single source of truth for topology (C1 全量物化).
//
// 为什么不能用 def 二次派生：物化资产（含 custom 编辑）的节点 ID/拓扑可能与
// EnabledMembers+memberNodeID 的 def 派生结果漂移（0 基 sort_order、手动换
// 序、自由节点 ID），漂移时 step 会记到错误成员名下（张冠李戴）。
//
// 匹配规则：agent 节点的 AgentName 是编译期 resolveCompileAgentKey 的产物
// （agentKey(agentID) → member.Name → agentID），反查按同一优先级建键。
//
// 返回 ok=false 表示无法从执行图构建归因（ct 为 nil / 无 agent 成员节点 /
// def 解析失败），调用方应回退 def 派生映射（buildResumeSessionContext）。
func buildAttributionFromCompiledTeam(ct *biz.CompiledTeam, defJSON string, agentKeyFn func(agentID string) string) (map[string]MemberDef, map[string]int, biz.OrchestrationRegistry, bool) {
	if ct == nil {
		return nil, nil, biz.OrchestrationRegistry{}, false
	}
	def, err := ParseDefinition(defJSON)
	if err != nil {
		return nil, nil, biz.OrchestrationRegistry{}, false
	}
	if agentKeyFn == nil {
		agentKeyFn = func(agentID string) string { return strings.TrimSpace(agentID) }
	}
	// 键 → member：与 resolveCompileAgentKey 同优先级（agentKey > Name >
	// agentID）；同键先到者优先（EnabledMembers 有效顺序）。
	byKey := make(map[string]MemberDef)
	for _, m := range EnabledMembers(def) {
		id := strings.TrimSpace(m.AgentID)
		if id == "" {
			continue
		}
		for _, k := range []string{strings.TrimSpace(agentKeyFn(id)), strings.TrimSpace(m.Name), id} {
			if k == "" {
				continue
			}
			if _, exists := byKey[k]; !exists {
				byKey[k] = m
			}
		}
	}
	memberByNode := make(map[string]MemberDef)
	stepSortIndex := make(map[string]int)
	entries := make([]biz.OrchestrationNodeRegistryEntry, 0, len(ct.Nodes))
	for _, n := range ct.Nodes {
		if strings.ToLower(strings.TrimSpace(n.Type)) != biz.NodeTypeAgent {
			continue
		}
		nodeID := strings.TrimSpace(n.ID)
		if nodeID == "" {
			continue
		}
		m, ok := byKey[strings.TrimSpace(n.AgentName)]
		if !ok {
			// 节点对应的成员已不在 def 中（如成员被移出团队）：跳过该节点归因，
			// 其 step 不持久化，不影响其余成员。
			continue
		}
		if _, dup := memberByNode[nodeID]; dup {
			continue
		}
		memberByNode[nodeID] = m
		stepSortIndex[nodeID] = len(memberByNode) - 1
		agentID := strings.TrimSpace(m.AgentID)
		key := strings.TrimSpace(agentKeyFn(agentID))
		// AgentName 兜底链与 BuildOrchestrationRegistry（agentNameFn=identity）
		// 保持一致，避免观测面显示行为漂移。
		name := agentID
		if name == "" {
			name = strings.TrimSpace(m.Name)
		}
		if name == "" {
			name = key
		}
		entries = append(entries, biz.OrchestrationNodeRegistryEntry{
			NodeID:    nodeID,
			AgentID:   agentID,
			AgentKey:  key,
			AgentName: name,
			Role:      strings.TrimSpace(m.Role),
		})
	}
	if len(memberByNode) == 0 {
		return nil, nil, biz.OrchestrationRegistry{}, false
	}
	return memberByNode, stepSortIndex, biz.NewOrchestrationRegistry(entries), true
}
