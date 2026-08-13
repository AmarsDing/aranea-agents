package plugintrpc

import "aranea-agents/internal/biz"

// BuiltinPluginDef describes a built-in plugin row synced into the plugins table on startup.
type BuiltinPluginDef struct {
	Key               string
	Name              string
	Description       string
	Category          string
	RiskLevel         string
	DefaultEnabled    bool
	Scope             string
	CallbackPoints    []string
	SortOrder         int
	ConfigSchemaJSON  string
	DefaultConfigJSON string
}

const auditLogSchema = `{"type":"object","properties":{"log_model_request":{"type":"boolean","default":true,"title":"记录模型请求","description":"是否记录模型请求摘要"},"log_model_response":{"type":"boolean","default":true,"title":"记录模型响应","description":"是否记录模型响应摘要"},"log_tool_args":{"type":"boolean","default":true,"title":"记录工具参数","description":"是否记录工具调用参数摘要"},"max_content_length":{"type":"integer","default":500,"title":"日志最大长度","description":"单段日志摘要的最大字符数"},"redact_sensitive":{"type":"boolean","default":true,"title":"脱敏敏感字段","description":"日志中是否脱敏密钥、Token 等敏感内容"}}}`

const auditLogDefaults = `{"log_model_request":true,"log_model_response":true,"log_tool_args":true,"max_content_length":500,"redact_sensitive":true}`

const retryReflectSchema = `{"type":"object","properties":{"max_retries":{"type":"integer","default":3,"title":"最大重试次数","description":"单个工具失败后最大反思重试次数"},"tracking_scope":{"type":"string","enum":["invocation","global"],"default":"invocation","title":"重试统计范围","description":"invocation=按本次调用计数；global=跨调用累计"},"error_if_retry_exceeded":{"type":"boolean","default":false,"title":"超限后返回错误","description":"超过最大次数后是否直接向用户返回原始错误"},"excluded_tools":{"type":"array","items":{"type":"string"},"default":[],"title":"排除工具","description":"不允许自动重试的工具名单"},"high_risk_tools_need_confirm":{"type":"boolean","default":true,"title":"高风险工具需确认","description":"高风险工具重试前是否需要人工确认"}}}`

const sensitiveMaskSchema = `{"type":"object","properties":{"mask_email":{"type":"boolean","default":true,"title":"脱敏邮箱","description":"是否脱敏邮箱地址"},"mask_phone":{"type":"boolean","default":true,"title":"脱敏手机号","description":"是否脱敏手机号码"},"mask_secret":{"type":"boolean","default":true,"title":"脱敏密钥","description":"是否脱敏 API Key、Token 等凭据"},"custom_patterns":{"type":"array","items":{"type":"object"},"default":[],"title":"自定义脱敏规则","description":"自定义正则脱敏规则列表"},"block_leak_output":{"type":"boolean","default":true,"title":"阻断泄漏输出","description":"模型输出疑似包含敏感信息时是否阻断"}}}`

const confirmationGuardSchema = `{"type":"object","properties":{"confirm_tools":{"type":"array","items":{"type":"string"},"default":[],"title":"需确认的工具","description":"调用前需要人工确认的工具名单"},"confirm_patterns":{"type":"array","items":{"type":"string"},"default":[],"title":"需确认的参数模式","description":"参数命中任一模式时需要确认"},"timeout_seconds":{"type":"integer","default":300,"title":"确认超时（秒）","description":"等待人工确认的超时时间"},"default_action":{"type":"string","enum":["reject","allow"],"default":"reject","title":"超时默认行为","description":"确认超时后的默认动作：reject=拒绝，allow=放行"}}}`

const costGuardSchema = `{"type":"object","properties":{"daily_token_budget":{"type":"integer","default":0,"title":"每日 Token 预算","description":"每日 token 消耗上限，0 表示不限制"},"max_prompt_tokens":{"type":"integer","default":0,"title":"单次 Prompt 上限","description":"单次请求最大 prompt token 数，0 表示不限制"},"blocked_models":{"type":"array","items":{"type":"string"},"default":[],"title":"禁用模型","description":"禁止调用的模型名单"},"fallback_model":{"type":"string","default":"","title":"降级模型","description":"预算不足时自动切换的备用模型"}}}`

const modelRouterSchema = `{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"model":{"type":"string"},"contains":{"type":"array","items":{"type":"string"}},"regex":{"type":"string"},"min_chars":{"type":"integer"},"priority":{"type":"integer"}}},"default":[],"title":"路由规则","description":"按优先级匹配的路由规则（关键词/正则/最小字符数）"},"default_model":{"type":"string","default":"","title":"默认模型","description":"未命中任何规则时使用的模型"},"code_model":{"type":"string","default":"","title":"代码任务模型","description":"代码生成类任务使用的模型"},"long_context_model":{"type":"string","default":"","title":"长上下文模型","description":"长上下文任务使用的模型"},"fallback_model":{"type":"string","default":"","title":"回退模型","description":"目标模型调用失败时回退的模型"}}}`

const permissionGuardSchema = `{"type":"object","properties":{"deny_tools":{"type":"array","items":{"type":"string"},"default":[],"title":"禁止工具","description":"禁止调用的工具黑名单"},"agent_allowlist":{"type":"array","items":{"type":"string"},"default":[],"title":"Agent 白名单","description":"仅在名单内的 Agent 上生效，为空表示全部生效"}}}`

const outputPolicySchema = `{"type":"object","properties":{"blocked_patterns":{"type":"array","items":{"type":"string"},"default":[],"title":"禁止输出模式","description":"模型输出禁止包含的模式列表"},"dangerous_command_check":{"type":"boolean","default":true,"title":"危险命令检查","description":"是否检查 rm -rf 等危险命令"},"block_on_violation":{"type":"boolean","default":true,"title":"命中即阻断","description":"命中策略时是否阻断输出"},"replacement_message":{"type":"string","default":"","title":"阻断提示语","description":"阻断时返回给用户的说明文案"}}}`

const skillTrackerSchema = `{"type":"object","properties":{"track_success":{"type":"boolean","default":true,"title":"记录成功调用","description":"是否记录成功的 Skill 调用"},"track_failure":{"type":"boolean","default":true,"title":"记录失败调用","description":"是否记录失败的 Skill 调用"},"capture_input_preview":{"type":"boolean","default":true,"title":"记录输入摘要","description":"是否记录调用输入摘要"},"capture_output_preview":{"type":"boolean","default":true,"title":"记录输出摘要","description":"是否记录调用输出摘要"},"max_preview_length":{"type":"integer","default":500,"title":"摘要最大长度","description":"输入/输出摘要的最大字符数"}}}`

// BuiltinPluginDefs returns built-in plugin metadata seeded into the database on startup.
func BuiltinPluginDefs() []BuiltinPluginDef {
	return []BuiltinPluginDef{
		{
			Key: "audit_log", Name: "运行日志和审计",
			Description: "记录 Agent 执行链路的工具调用与模型交互摘要（内置实现）",
			Category:    "observability", RiskLevel: "low",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_agent", "after_agent", "before_model", "after_model", "before_tool", "after_tool", "on_event"},
			SortOrder:      100, ConfigSchemaJSON: auditLogSchema, DefaultConfigJSON: auditLogDefaults,
		},
		{
			Key: "skill_usage_tracker", Name: "Skill 使用追踪",
			Description: "追踪 Skill 工具调用与执行摘要",
			Category:    "tracking", RiskLevel: "low",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_tool", "after_tool"},
			SortOrder:      110, ConfigSchemaJSON: skillTrackerSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "retry_and_reflect", Name: "重试与反思",
			Description: "工具失败时自动重试与反思策略",
			Category:    "reliability", RiskLevel: "medium",
			DefaultEnabled: false, Scope: "global",
			// GAP-02：Register() 同时注册 AfterAgent（重试计数清理）与 AfterTool，
			// 种子声明必须与实现一致（存量行由 bootstrap SyncBuiltinMeta 同步）。
			CallbackPoints: []string{"after_agent", "after_tool"},
			SortOrder:      120, ConfigSchemaJSON: retryReflectSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "sensitive_data_mask", Name: "敏感数据脱敏",
			Description: "在模型请求/响应中脱敏邮箱、手机号与密钥",
			Category:    "security", RiskLevel: "medium",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_model", "after_model"},
			SortOrder:      130, ConfigSchemaJSON: sensitiveMaskSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "confirmation_guard", Name: "工具确认守卫",
			Description: "对高风险工具调用执行确认或拒绝策略",
			Category:    "security", RiskLevel: "high",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_tool"},
			SortOrder:      140, ConfigSchemaJSON: confirmationGuardSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "cost_guard", Name: "成本守卫",
			Description: "按 Token 预算与模型黑名单限制调用",
			Category:    "governance", RiskLevel: "medium",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_model"},
			SortOrder:      150, ConfigSchemaJSON: costGuardSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "model_router", Name: "模型路由",
			Description: "按规则将请求路由到不同模型",
			Category:    "routing", RiskLevel: "low",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_model"},
			SortOrder:      160, ConfigSchemaJSON: modelRouterSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "permission_guard", Name: "权限守卫",
			Description: "按工具黑名单与 Agent 白名单限制工具调用",
			Category:    "security", RiskLevel: "high",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"before_tool"},
			SortOrder:      170, ConfigSchemaJSON: permissionGuardSchema, DefaultConfigJSON: "{}",
		},
		{
			Key: "output_policy", Name: "输出策略",
			Description: "检测并阻断不符合策略的模型输出",
			Category:    "security", RiskLevel: "medium",
			DefaultEnabled: false, Scope: "global",
			CallbackPoints: []string{"after_model", "on_event"},
			SortOrder:      180, ConfigSchemaJSON: outputPolicySchema, DefaultConfigJSON: "{}",
		},
	}
}

// ToBizPlugin maps a built-in definition to a biz.Plugin ready for persistence.
func (d BuiltinPluginDef) ToBizPlugin() biz.Plugin {
	cfg := d.DefaultConfigJSON
	if cfg == "" {
		cfg = "{}"
	}
	schema := d.ConfigSchemaJSON
	if schema == "" {
		schema = "{}"
	}
	return biz.Plugin{
		ID:                "builtin-" + d.Key,
		Key:               d.Key,
		Name:              d.Name,
		Description:       d.Description,
		Category:          d.Category,
		RiskLevel:         d.RiskLevel,
		Enabled:           d.DefaultEnabled,
		Scope:             d.Scope,
		CallbackPoints:    append([]string(nil), d.CallbackPoints...),
		SortOrder:         d.SortOrder,
		ConfigSchemaJSON:  schema,
		ConfigJSON:        cfg,
		DefaultConfigJSON: cfg,
		Permissions:       biz.AdminPluginPerms(),
	}
}
