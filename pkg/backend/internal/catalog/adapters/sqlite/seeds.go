package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"

	"arenea/backend/internal/domain"
)

// CreatePlatformResourceFn 与 legacy repository 创建平台资源行对齐（INSERT 或去重失败时短路）。
type CreatePlatformResourceFn func(domain.PlatformResource) (domain.PlatformResource, error)

// SeedPlatformDefaults 安装引导用 LLM 模型与系统智能体分类树。约束失败短路，已有行保持不变。
func SeedPlatformDefaults(create CreatePlatformResourceFn) error {
	defaults := []domain.PlatformResource{
		{ID: "model_openrouter_gpt41mini", Resource: "llm-provider-models", Key: "openrouter:gpt-4.1-mini", Name: "GPT 4.1 Mini", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "默认 OpenRouter 兼容模型", Enabled: true, SortOrder: 10},
		{ID: "model_anthropic_sonnet", Resource: "llm-provider-models", Key: "anthropic:claude-sonnet", Name: "Claude Sonnet", Provider: "anthropic", Model: "claude-sonnet", Description: "Anthropic 兼容模型", Enabled: true, SortOrder: 20},
	}
	defaults = append(defaults, agentCategorySeeds...)
	for _, row := range defaults {
		if _, err := create(row); err != nil && !strings.Contains(err.Error(), "constraint failed") {
			return err
		}
	}
	return nil
}

// UpsertRuntimeSettingsFn 与 catalog AgentRepository 写入 agent_runtime_settings 对齐。
type UpsertRuntimeSettingsFn func(domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error)

// SeedSystemAdminAgent 插入内置 `__system_admin__` 智能体，支撑 Aranea CLI 交互式 REPL。
func SeedSystemAdminAgent(db *sql.DB, upsert UpsertRuntimeSettingsFn) error {
	const id = "agent_system_admin"
	const key = "__system_admin__"
	now := nowISO()

	systemPrompt := `你是 Aranea 平台的"系统管家" Agent，运行在命令行的交互式控制台中。

职责：
  * 帮助管理员通过自然语言完成 Skill / Agent / Tool / Plugin / MCP /
    定时任务 / 渠道 / 会话 / 监控等系统级操作。
  * 当用户描述"装一个 GitHub 上的 skill" 等需求时，回复一条可直接复制
    执行的 aranea 命令（例如 ` + "`aranea skill install <url>`" + `），
    并解释每一步的影响范围与回滚方式。
  * 涉及高风险动作（删除、提权、禁用核心插件等）时主动提示用户加上
    --yes 或在确认后再执行。

输出风格：先给一行结论，然后用简短的步骤说明，最后附可执行命令。`

	configJSON := `{"system_prompt":` + jsonString(systemPrompt) + `,"is_system":true,"readonly":true,"kind":"system_admin"}`

	_, err := db.Exec(
		`INSERT INTO agents(
		   id, agent_key, display_name, provider, model, status, is_default, is_favorite, icon, agent_description,
		   category_position_id, system_prompt_mode, context_window, budget_monthly_cents, config_json,
		   created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 1, ?, ?, '', 'manual', 16000, 0, ?, ?, ?, '')
		ON CONFLICT(agent_key) DO UPDATE SET
		   display_name      = excluded.display_name,
		   icon              = excluded.icon,
		   agent_description = excluded.agent_description,
		   config_json       = excluded.config_json,
		   updated_at        = excluded.updated_at,
		   deleted_at        = ''`,
		id, key, "系统管家", "openrouter", "gpt-4.1-mini", "active",
		"settings", "Aranea CLI 的内置 Agent，负责把自然语言转成系统级操作指令。", configJSON,
		now, now,
	)
	if err != nil {
		return err
	}
	return seedSystemAdminAgentSettings(upsert, id)
}

func seedSystemAdminAgentSettings(upsert UpsertRuntimeSettingsFn, agentID string) error {
	allow := []string{"group:cli_admin", "web_fetch", "read_file", "datetime"}
	deny := []string{"shell_exec", "write_file", "edit_file", "create_image", "tts"}
	allowJSON, err := json.Marshal(allow)
	if err != nil {
		return err
	}
	denyJSON, err := json.Marshal(deny)
	if err != nil {
		return err
	}
	_, err = upsert(domain.AgentRuntimeSettings{
		AgentID:                           agentID,
		SubagentsEnabled:                  false,
		SubagentsMaxConcurrency:           4,
		SubagentsMaxGenerationDepth:       1,
		SubagentsMaxChildrenPerAgent:      2,
		SubagentsArchiveAfterMinutes:      60,
		SubagentsMaxRetries:               1,
		ToolsEnabled:                      true,
		ToolsProfile:                      "system_admin",
		ToolsAllowJSON:                    string(allowJSON),
		ToolsDenyJSON:                     string(denyJSON),
		ToolsConcurrentAllowJSON:          "[]",
		MemoryEnabled:                     false,
		MemoryMaxChunkLength:              1000,
		MemoryMaxResults:                  6,
		MemoryMinScore:                    0.35,
		HeartbeatEnabled:                  false,
		HeartbeatIntervalMinutes:          30,
		EvolutionSelfEvolve:               false,
		EvolutionSkillEvolve:              false,
		EvolutionMetricsEnabled:           true,
		EvolutionSuggestionsEnabled:       false,
		GuardrailMaxChangePerPeriod:       0.1,
		GuardrailMinDataPoints:            100,
		GuardrailRollbackOnDeclinePercent: 20,
	})
	return err
}

func jsonString(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			if r < 0x20 {
				out = append(out, []byte{'\\', 'u', '0', '0', hex(byte(r >> 4)), hex(byte(r & 0x0f))}...)
				continue
			}
			out = append(out, []byte(string(r))...)
		}
	}
	out = append(out, '"')
	return string(out)
}

func hex(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + (b - 10)
}

var agentCategorySeeds = []domain.PlatformResource{
	{ID: "cat_it", Resource: "agent-categories", Key: "it-industry", Name: "IT行业", Description: "系统预置行业：研发、平台、游戏与 AI 工程", Enabled: true, SortOrder: 10, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_game", Resource: "agent-categories", Key: "it-game-dev", Name: "游戏开发部", Description: "游戏研发、引擎、场景与玩法", Enabled: true, SortOrder: 10, ParentID: "cat_it", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_game_ue5", Resource: "agent-categories", Key: "it-game-ue5-scene-designer", Name: "UE5场景设计师", Description: "负责 UE5 场景、光照、材质与关卡协作", Enabled: true, SortOrder: 10, ParentID: "cat_it_game", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_system", Resource: "agent-categories", Key: "it-system-dev", Name: "系统开发部", Description: "后端、平台工程、工具链与基础设施", Enabled: true, SortOrder: 20, ParentID: "cat_it", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_it_system_go", Resource: "agent-categories", Key: "it-system-golang-senior", Name: "golang后端高级工程师", Description: "负责 Go 服务、接口、数据库与可靠性", Enabled: true, SortOrder: 10, ParentID: "cat_it_system", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai", Resource: "agent-categories", Key: "ai-industry", Name: "AI行业", Description: "系统预置行业：智能体、模型应用与数据工程", Enabled: true, SortOrder: 20, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai_agent", Resource: "agent-categories", Key: "ai-agent-platform", Name: "Agent平台部", Description: "Agent 编排、工具、记忆与工作流", Enabled: true, SortOrder: 10, ParentID: "cat_ai", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_ai_agent_architect", Resource: "agent-categories", Key: "ai-agent-architect", Name: "Agent架构师", Description: "设计 Agent 能力、提示词、工具策略与运行闭环", Enabled: true, SortOrder: 10, ParentID: "cat_ai_agent", Level: "position", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design", Resource: "agent-categories", Key: "design-industry", Name: "创意设计行业", Description: "系统预置行业：品牌、界面、内容与视觉生产", Enabled: true, SortOrder: 30, Level: "industry", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design_ui", Resource: "agent-categories", Key: "design-uiux", Name: "UIUX设计部", Description: "用户体验、界面系统与视觉规范", Enabled: true, SortOrder: 10, ParentID: "cat_design", Level: "department", MetadataJSON: `{"is_system":true}`},
	{ID: "cat_design_ui_senior", Resource: "agent-categories", Key: "design-uiux-senior", Name: "高级UIUX设计师", Description: "负责设计系统、交互流程与高保真界面", Enabled: true, SortOrder: 10, ParentID: "cat_design_ui", Level: "position", MetadataJSON: `{"is_system":true}`},
}
