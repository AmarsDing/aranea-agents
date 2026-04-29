package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// EffectiveAgentTool is one row in the agent effective tools matrix (legacy JSON shape).
type EffectiveAgentTool struct {
	ToolKey        string
	DisplayName    string
	Category       string
	Source         string
	Enabled        bool
	EffectiveState string
	Reason         string
}

// AgentEffectiveTools matches pkg/backend domain.AgentEffectiveTools JSON for API compatibility.
type AgentEffectiveTools struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
	Items        []EffectiveAgentTool
}

// AgentToolPolicyInput is the writable subset for PUT .../tools/policy.
type AgentToolPolicyInput struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
}

func jsonStringList(raw string) []string {
	var result []string
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &result) != nil {
		return []string{}
	}
	return result
}

var toolGroupsFilesystem = []string{"read_file", "write_file", "list_files", "edit_file"}
var toolGroupsWeb = []string{"web_search", "web_fetch"}
var toolGroupsMemory = []string{"memory_search", "memory_get"}
var toolGroupsSkill = []string{"skill_search", "use_skill"}
var toolGroupsMedia = []string{"read_image", "read_document", "create_image", "tts"}
var toolGroupsRuntime = []string{"shell_exec"}

func cliAdminKeysFromCatalog(catalog []Tool) []string {
	var keys []string
	for _, t := range catalog {
		if strings.HasPrefix(t.Key, "cli_admin_") {
			keys = append(keys, t.Key)
		}
	}
	return keys
}

func expandToolGroup(name string, catalog []Tool) []string {
	switch strings.TrimSpace(name) {
	case "filesystem":
		return append([]string{}, toolGroupsFilesystem...)
	case "web":
		return append([]string{}, toolGroupsWeb...)
	case "memory":
		return append([]string{}, toolGroupsMemory...)
	case "skill":
		return append([]string{}, toolGroupsSkill...)
	case "media":
		return append([]string{}, toolGroupsMedia...)
	case "runtime":
		return append([]string{}, toolGroupsRuntime...)
	case "cli_admin":
		return cliAdminKeysFromCatalog(catalog)
	default:
		return nil
	}
}

func profileAllowSet(profile string, catalog []Tool) map[string]bool {
	result := map[string]bool{}
	for _, key := range toolProfiles[strings.TrimSpace(profile)] {
		if strings.HasPrefix(key, "group:") {
			gn := strings.TrimPrefix(key, "group:")
			for _, member := range expandToolGroup(gn, catalog) {
				result[member] = true
			}
			continue
		}
		result[key] = true
	}
	return result
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

func buildAgentEffectiveTools(settings AgentRuntimeSettings, catalog []Tool) AgentEffectiveTools {
	allow := jsonStringList(settings.ToolsAllowJSON)
	deny := jsonStringList(settings.ToolsDenyJSON)

	allowedSet := profileAllowSet(settings.ToolsProfile, catalog)
	for _, key := range allow {
		if strings.HasPrefix(key, "group:") {
			for _, member := range expandToolGroup(strings.TrimPrefix(key, "group:"), catalog) {
				allowedSet[member] = true
			}
			continue
		}
		allowedSet[key] = true
	}

	denySet := map[string]bool{}
	for _, key := range deny {
		if strings.HasPrefix(key, "group:") {
			for _, member := range expandToolGroup(strings.TrimPrefix(key, "group:"), catalog) {
				denySet[member] = true
			}
			continue
		}
		denySet[key] = true
	}

	items := make([]EffectiveAgentTool, 0, len(catalog))
	for _, tool := range catalog {
		state := "denied"
		reason := "global_disabled"
		baseEnabled := settings.ToolsEnabled && tool.Enabled
		if baseEnabled && (settings.ToolsProfile == "" || settings.ToolsProfile == "full" || allowedSet[tool.Key]) {
			state = "allowed"
			reason = "profile:" + settings.ToolsProfile
		}
		if denySet[tool.Key] {
			state = "denied"
			reason = "agent_deny"
		}
		if !settings.ToolsEnabled {
			reason = "agent_tools_disabled"
		}
		items = append(items, EffectiveAgentTool{
			ToolKey:        tool.Key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        baseEnabled && state == "allowed",
			EffectiveState: state,
			Reason:         reason,
		})
	}

	return AgentEffectiveTools{
		ToolsEnabled: settings.ToolsEnabled,
		Profile:      canonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}
}

// GetEffectiveTools returns merged tool catalog + agent runtime policy (legacy semantics).
func (u *AgentUsecase) GetEffectiveTools(ctx context.Context, agentID string) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, kerrors.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.repo.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: 1000, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	return buildAgentEffectiveTools(settings, all.Items), nil
}

func (u *AgentUsecase) runtimeSettingsForEffective(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	settings, err := u.repo.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s := DefaultAgentRuntimeSettings()
			s.AgentID = agentID
			return withSettingDefaults(s), nil
		}
		return AgentRuntimeSettings{}, err
	}
	return withSettingDefaults(settings), nil
}

// UpdateAgentToolPolicy updates agent_runtime_settings tool columns and returns recomputed effective tools.
func (u *AgentUsecase) UpdateAgentToolPolicy(ctx context.Context, agentID string, in AgentToolPolicyInput) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, kerrors.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.repo.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings.ToolsEnabled = in.ToolsEnabled
	if strings.TrimSpace(in.Profile) != "" {
		settings.ToolsProfile = strings.TrimSpace(in.Profile)
	}
	allowJSON, _ := json.Marshal(in.Allow)
	denyJSON, _ := json.Marshal(in.Deny)
	settings.ToolsAllowJSON = string(allowJSON)
	settings.ToolsDenyJSON = string(denyJSON)
	if _, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: 1000, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err = u.repo.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings = withSettingDefaults(settings)

	return buildAgentEffectiveTools(settings, all.Items), nil
}
