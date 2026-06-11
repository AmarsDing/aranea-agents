package pack

import (
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"
)

// BuildOrgKey 从 company/department/position key 组合构建 org_key 路径。
// 格式：company_key/department_key/position_key
func BuildOrgKey(companyKey, deptKey, posKey string) string {
	parts := []string{companyKey}
	if deptKey != "" {
		parts = append(parts, deptKey)
	}
	if posKey != "" {
		parts = append(parts, posKey)
	}
	return strings.Join(parts, "/")
}

// ParseOrgKeyPath 解析 org_key 路径为 company/department/position 三段。
// 输入格式：company_key/department_key/position_key
func ParseOrgKeyPath(path string) (company, dept, pos string, err error) {
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 1:
		return parts[0], "", "", nil
	case 2:
		return parts[0], parts[1], "", nil
	case 3:
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", apierror.BadRequest("PACK_ORG_KEY_INVALID", "无效的 org_key 路径: %s", path)
	}
}

// KeyMapper 维护 Pack 导入过程中的 key→ID 映射。
// 所有访问通过 RWMutex 保护，防御未来并发导入场景。
type KeyMapper struct {
	mu             sync.RWMutex
	agentKeyToID   map[string]string // agent_key → agent_id
	teamKeyToID    map[string]string // team_key → team_id
	orgKeyToID     map[string]string // org_key 路径 → organization_node_id
	graphIDMap     map[string]string // 原始 graph_id → 新 graph_id
}

// NewKeyMapper 创建新的映射器。
func NewKeyMapper() *KeyMapper {
	return &KeyMapper{
		agentKeyToID: make(map[string]string),
		teamKeyToID:  make(map[string]string),
		orgKeyToID:   make(map[string]string),
		graphIDMap:   make(map[string]string),
	}
}

// RegisterAgent 记录 agent_key → agent_id 映射。
func (m *KeyMapper) RegisterAgent(agentKey, agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentKeyToID[agentKey] = agentID
}

// AgentID 通过 agent_key 查找 agent_id。
func (m *KeyMapper) AgentID(agentKey string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.agentKeyToID[agentKey]
	return id, ok
}

// RegisterTeam 记录 team_key → team_id 映射（ConflictSkip 路径需要）。
func (m *KeyMapper) RegisterTeam(teamKey, teamID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teamKeyToID[teamKey] = teamID
}

// ResolveTeamKey 将 team_key 解析为 team_id。
func (m *KeyMapper) ResolveTeamKey(teamKey string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.teamKeyToID[teamKey]
	return id, ok
}

// RegisterOrg 记录 org_key 路径 → organization_node_id 映射。
func (m *KeyMapper) RegisterOrg(orgKey, nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgKeyToID[orgKey] = nodeID
}

// OrgID 通过 org_key 路径查找 node_id。
func (m *KeyMapper) OrgID(orgKey string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.orgKeyToID[orgKey]
	return id, ok
}

// RegisterGraph 记录原始 graph_id → 新 graph_id 映射。
func (m *KeyMapper) RegisterGraph(origID, newID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.graphIDMap[origID] = newID
}

// GraphID 通过原始 graph_id 查找新 graph_id。
func (m *KeyMapper) GraphID(origID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.graphIDMap[origID]
	return id, ok
}

// ResolveAgentKey 将 agent_key 解析为 agent_id，若未找到返回错误。
func (m *KeyMapper) ResolveAgentKey(agentKey string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.agentKeyToID[agentKey]
	if !ok {
		return "", apierror.BadRequest("PACK_AGENT_KEY_NOT_FOUND", "agent_key %q 未在映射表中找到", agentKey)
	}
	return id, nil
}

// ResolvePositionKey 将 position_key 路径解析为 position_id。
func (m *KeyMapper) ResolvePositionKey(positionKey string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.orgKeyToID[positionKey]
	if !ok {
		return "", apierror.BadRequest("PACK_POSITION_KEY_NOT_FOUND", "position_key %q 未在映射表中找到", positionKey)
	}
	return id, nil
}
