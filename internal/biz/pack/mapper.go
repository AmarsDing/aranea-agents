package pack

import (
	"fmt"
	"strings"
	"sync"

	kratos "github.com/go-kratos/kratos/v2/errors"
)

// BuildTaxonomyKey 从 industry/department/position key 组合构建 taxonomy_key 路径。
// 格式：industry_key/department_key/position_key
func BuildTaxonomyKey(industryKey, deptKey, posKey string) string {
	parts := []string{industryKey}
	if deptKey != "" {
		parts = append(parts, deptKey)
	}
	if posKey != "" {
		parts = append(parts, posKey)
	}
	return strings.Join(parts, "/")
}

// ParseTaxonomyKeyPath 解析 taxonomy_key 路径为 industry/department/position 三段。
// 输入格式：industry_key/department_key/position_key
func ParseTaxonomyKeyPath(path string) (industry, dept, pos string, err error) {
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 1:
		return parts[0], "", "", nil
	case 2:
		return parts[0], parts[1], "", nil
	case 3:
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", kratos.BadRequest("PACK_TAXONOMY_KEY_INVALID", fmt.Sprintf("无效的 taxonomy_key 路径: %s", path))
	}
}

// KeyMapper 维护 Pack 导入过程中的 key→ID 映射。
// 所有访问通过 RWMutex 保护，防御未来并发导入场景。
type KeyMapper struct {
	mu             sync.RWMutex
	agentKeyToID   map[string]string // agent_key → agent_id
	teamKeyToID    map[string]string // team_key → team_id
	taxonomyKeyToID map[string]string // taxonomy_key 路径 → taxonomy_node_id
	graphIDMap     map[string]string // 原始 graph_id → 新 graph_id
}

// NewKeyMapper 创建新的映射器。
func NewKeyMapper() *KeyMapper {
	return &KeyMapper{
		agentKeyToID:    make(map[string]string),
		teamKeyToID:     make(map[string]string),
		taxonomyKeyToID: make(map[string]string),
		graphIDMap:      make(map[string]string),
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

// RegisterTaxonomy 记录 taxonomy_key 路径 → taxonomy_node_id 映射。
func (m *KeyMapper) RegisterTaxonomy(taxonomyKey, nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taxonomyKeyToID[taxonomyKey] = nodeID
}

// TaxonomyID 通过 taxonomy_key 路径查找 node_id。
func (m *KeyMapper) TaxonomyID(taxonomyKey string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.taxonomyKeyToID[taxonomyKey]
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
		return "", kratos.BadRequest("PACK_AGENT_KEY_NOT_FOUND", fmt.Sprintf("agent_key %q 未在映射表中找到", agentKey))
	}
	return id, nil
}

// ResolvePositionKey 将 position_key 路径解析为 taxonomy_position_id。
func (m *KeyMapper) ResolvePositionKey(positionKey string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.taxonomyKeyToID[positionKey]
	if !ok {
		return "", kratos.BadRequest("PACK_POSITION_KEY_NOT_FOUND", fmt.Sprintf("position_key %q 未在映射表中找到", positionKey))
	}
	return id, nil
}
