package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	capsql "arenea/backend/internal/capability/adapters/sqlite"
	"arenea/backend/internal/domain"
)

// ToolDataStore 是 ToolService 所需的最小持久化接口（由 SQLite 适配器或 storage 实现）。
type ToolDataStore interface {
	SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error)
	GetToolByID(id string) (domain.Tool, error)
	CreateTool(input domain.ToolUpsertInput) (domain.Tool, error)
	UpdateTool(id string, input domain.ToolUpsertInput) (domain.Tool, error)
	DeleteTool(id string) error
	UpdateToolEnabled(id string, enabled bool) (domain.Tool, error)
	SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error)
}

// AgentRuntimeSettingsStore 读写 agent_runtime_settings（可选，未设置时 ToolService 使用域默认）。
type AgentRuntimeSettingsStore interface {
	GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(settings domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error)
}

// EvolutionToolPolicySource 将自演化工具黑名单与偏好分数合并进 EffectiveForAgent。
type EvolutionToolPolicySource interface {
	ToolPolicyForAgent(ctx context.Context, agentID string) (blacklist []string, preference map[string]float64, err error)
}

// ToolService 提供工具目录、运行记录与按智能体生效策略。
type ToolService struct {
	store     ToolDataStore
	settings  AgentRuntimeSettingsStore
	evolution EvolutionToolPolicySource
}

// NewToolService 构造 ToolService；若 store 同时实现 AgentRuntimeSettingsStore，会自动接 runtime 设置。
func NewToolService(store ToolDataStore) *ToolService {
	s := &ToolService{store: store}
	if settings, ok := store.(AgentRuntimeSettingsStore); ok {
		s.settings = settings
	}
	return s
}

// SetRuntimeSettingsStore 显式注入 runtime 存储（覆盖从 store 推断的行为）。
func (s *ToolService) SetRuntimeSettingsStore(store AgentRuntimeSettingsStore) {
	s.settings = store
}

// SetEvolutionPolicySource 注入 EffectiveForAgent 使用的可选自演化策略来源。
func (s *ToolService) SetEvolutionPolicySource(src EvolutionToolPolicySource) {
	s.evolution = src
}

func (s *ToolService) Search(query domain.ToolListQuery) (domain.ToolListResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	query.Enabled = strings.TrimSpace(query.Enabled)
	if query.Enabled != "" && query.Enabled != "true" && query.Enabled != "false" {
		return domain.ToolListResult{}, validationError("enabled must be true or false")
	}
	return s.store.SearchTools(query)
}

func (s *ToolService) Get(id string) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, validationError("tool id is required")
	}
	return s.store.GetToolByID(id)
}

func (s *ToolService) Create(input domain.ToolUpsertInput) (domain.Tool, error) {
	if strings.TrimSpace(input.Key) == "" {
		return domain.Tool{}, validationError("tool key is required")
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = input.Key
	}
	if strings.TrimSpace(input.Category) == "" {
		input.Category = "custom"
	}
	if strings.TrimSpace(input.Source) == "" {
		input.Source = "external"
	}
	if strings.TrimSpace(input.RiskLevel) == "" {
		input.RiskLevel = "low"
	}
	return s.store.CreateTool(input)
}

func (s *ToolService) Update(id string, input domain.ToolUpsertInput) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, validationError("tool id is required")
	}
	return s.store.UpdateTool(id, input)
}

func (s *ToolService) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return validationError("tool id is required")
	}
	return s.store.DeleteTool(id)
}

func (s *ToolService) ToggleEnabled(id string, enabled bool) (domain.Tool, error) {
	tool, err := s.Get(id)
	if err != nil {
		return domain.Tool{}, err
	}
	if enabled && (tool.RiskLevel == "high" || tool.RiskLevel == "critical") {
		// 前端会要求确认；后端仍通过仅在显式调用后返回更新后的工具来保留风险可见性。
	}
	return s.store.UpdateToolEnabled(id, enabled)
}

func (s *ToolService) SearchRuns(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	return s.store.SearchToolInvocations(query)
}

func (s *ToolService) EffectiveForAgent(agentID string) (domain.AgentEffectiveTools, error) {
	if strings.TrimSpace(agentID) == "" {
		return domain.AgentEffectiveTools{}, validationError("agent id is required")
	}
	settings := domain.DefaultAgentRuntimeSettings()
	settings.AgentID = agentID
	if s.settings == nil {
		return s.effectiveForAgentWithSettings(agentID, settings)
	}
	var err error
	settings, err = s.settings.GetAgentRuntimeSettings(agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			settings = domain.DefaultAgentRuntimeSettings()
			settings.AgentID = agentID
		} else {
			return domain.AgentEffectiveTools{}, err
		}
	}
	return s.effectiveForAgentWithSettings(agentID, settings)
}

func (s *ToolService) effectiveForAgentWithSettings(agentID string, settings domain.AgentRuntimeSettings) (domain.AgentEffectiveTools, error) {
	all, err := s.store.SearchTools(domain.ToolListQuery{Limit: 1000})
	if err != nil {
		return domain.AgentEffectiveTools{}, err
	}
	allow := jsonList(settings.ToolsAllowJSON)
	deny := jsonList(settings.ToolsDenyJSON)
	allowedSet := s.profileAllowSet(settings.ToolsProfile)
	for _, key := range allow {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				allowedSet[member] = true
			}
			continue
		}
		allowedSet[key] = true
	}
	denySet := map[string]bool{}
	for _, key := range deny {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				denySet[member] = true
			}
			continue
		}
		denySet[key] = true
	}
	evoBlacklist, evoPreference := s.resolveEvolutionPolicy(agentID)
	evoBlacklistSet := map[string]bool{}
	for _, k := range evoBlacklist {
		evoBlacklistSet[k] = true
	}

	items := make([]domain.EffectiveAgentTool, 0, len(all.Items))
	for _, tool := range all.Items {
		state := "denied"
		reason := "global_disabled"
		enabled := settings.ToolsEnabled && tool.Enabled
		if enabled && (settings.ToolsProfile == "" || settings.ToolsProfile == "full" || allowedSet[tool.Key]) {
			state = "allowed"
			reason = "profile:" + settings.ToolsProfile
		}
		if denySet[tool.Key] {
			state = "denied"
			reason = "agent_deny"
		}
		if evoBlacklistSet[tool.Key] {
			state = "denied"
			reason = "evolution_blacklist"
		}
		if !settings.ToolsEnabled {
			reason = "agent_tools_disabled"
		}
		items = append(items, domain.EffectiveAgentTool{
			ToolKey:        tool.Key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        enabled && state == "allowed",
			EffectiveState: state,
			Reason:         reason,
		})
	}

	if len(evoPreference) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].EffectiveState != items[j].EffectiveState {
				return items[i].EffectiveState == "allowed"
			}
			return evoPreference[items[i].ToolKey] > evoPreference[items[j].ToolKey]
		})
	}

	return domain.AgentEffectiveTools{
		ToolsEnabled: settings.ToolsEnabled,
		Profile:      canonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}, nil
}

func (s *ToolService) resolveEvolutionPolicy(agentID string) ([]string, map[string]float64) {
	if s.evolution == nil {
		return nil, nil
	}
	bl, pref, err := s.evolution.ToolPolicyForAgent(context.Background(), agentID)
	if err != nil {
		return nil, nil
	}
	return bl, pref
}

func (s *ToolService) UpdateAgentPolicy(agentID string, input domain.AgentEffectiveTools) (domain.AgentEffectiveTools, error) {
	if strings.TrimSpace(agentID) == "" {
		return domain.AgentEffectiveTools{}, validationError("agent id is required")
	}
	if s.settings == nil {
		return domain.AgentEffectiveTools{}, validationError("runtime settings store is required")
	}
	settings, err := s.settings.GetAgentRuntimeSettings(agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			settings = domain.DefaultAgentRuntimeSettings()
			settings.AgentID = agentID
		} else {
			return domain.AgentEffectiveTools{}, err
		}
	}
	settings.ToolsEnabled = input.ToolsEnabled
	if strings.TrimSpace(input.Profile) != "" {
		settings.ToolsProfile = strings.TrimSpace(input.Profile)
	}
	allow, _ := json.Marshal(input.Allow)
	deny, _ := json.Marshal(input.Deny)
	settings.ToolsAllowJSON = string(allow)
	settings.ToolsDenyJSON = string(deny)
	if _, err = s.settings.UpsertAgentRuntimeSettings(settings); err != nil {
		return domain.AgentEffectiveTools{}, err
	}
	return s.EffectiveForAgent(agentID)
}

func (s *ToolService) profileAllowSet(profile string) map[string]bool {
	result := map[string]bool{}
	for _, key := range toolProfiles[strings.TrimSpace(profile)] {
		if strings.HasPrefix(key, "group:") {
			for _, member := range toolGroups[strings.TrimPrefix(key, "group:")] {
				result[member] = true
			}
			continue
		}
		result[key] = true
	}
	return result
}

var toolGroups = map[string][]string{
	"filesystem": {"read_file", "write_file", "list_files", "edit_file"},
	"web":        {"web_search", "web_fetch"},
	"memory":     {"memory_search", "memory_get"},
	"skill":      {"skill_search", "use_skill"},
	"media":      {"read_image", "read_document", "create_image", "tts"},
	"runtime":    {"shell_exec"},
	"cli_admin":  capsql.CLIAdminToolKeys(),
}

var toolProfiles = map[string][]string{
	"chat_only": {},
	"read_only": {"datetime", "read_file", "list_files"},
	"coding":    {"group:filesystem", "group:web", "group:skill", "datetime"},
	"research":  {"web_search", "web_fetch", "read_file", "list_files", "skill_search", "memory_search", "datetime"},
	"full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media", "group:runtime", "group:cli_admin", "datetime"},

	"minimal":      {},
	"safe":         {"datetime", "read_file", "list_files"},
	"system_admin": {"group:cli_admin", "web_fetch", "datetime"},
}

func canonicalToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		return ""
	case "chat_only", "minimal":
		return "chat_only"
	case "read_only", "safe":
		return "read_only"
	case "coding":
		return "coding"
	case "research":
		return "research"
	case "system_admin", "full":
		return "full"
	default:
		return profile
	}
}

func normalizeLimitOffset(limit, offset, defaultLimit int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func jsonList(raw string) []string {
	var result []string
	if json.Unmarshal([]byte(raw), &result) != nil {
		return []string{}
	}
	return result
}
