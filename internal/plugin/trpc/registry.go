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

const auditLogSchema = `{"type":"object","properties":{"log_model_request":{"type":"boolean","default":true},"log_model_response":{"type":"boolean","default":true},"log_tool_args":{"type":"boolean","default":true},"max_content_length":{"type":"integer","default":500},"redact_sensitive":{"type":"boolean","default":true}}}`

const auditLogDefaults = `{"log_model_request":true,"log_model_response":true,"log_tool_args":true,"max_content_length":500,"redact_sensitive":true}`

const retryReflectSchema = `{"type":"object","properties":{"max_retries":{"type":"integer","default":3},"tracking_scope":{"type":"string","enum":["invocation","global"],"default":"invocation"},"error_if_retry_exceeded":{"type":"boolean","default":false},"excluded_tools":{"type":"array","items":{"type":"string"},"default":[]},"high_risk_tools_need_confirm":{"type":"boolean","default":true}}}`

const sensitiveMaskSchema = `{"type":"object","properties":{"mask_email":{"type":"boolean","default":true},"mask_phone":{"type":"boolean","default":true},"mask_secret":{"type":"boolean","default":true},"custom_patterns":{"type":"array","items":{"type":"object"},"default":[]},"block_leak_output":{"type":"boolean","default":true}}}`

const confirmationGuardSchema = `{"type":"object","properties":{"confirm_tools":{"type":"array","items":{"type":"string"},"default":[]},"confirm_patterns":{"type":"array","items":{"type":"string"},"default":[]},"timeout_seconds":{"type":"integer","default":300},"default_action":{"type":"string","enum":["reject","allow"],"default":"reject"}}}`

const costGuardSchema = `{"type":"object","properties":{"daily_token_budget":{"type":"integer","default":0},"max_prompt_tokens":{"type":"integer","default":0},"blocked_models":{"type":"array","items":{"type":"string"},"default":[]},"fallback_model":{"type":"string","default":""}}}`

const modelRouterSchema = `{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"model":{"type":"string"},"contains":{"type":"array","items":{"type":"string"}},"regex":{"type":"string"},"min_chars":{"type":"integer"},"priority":{"type":"integer"}}},"default":[]},"default_model":{"type":"string","default":""},"code_model":{"type":"string","default":""},"long_context_model":{"type":"string","default":""},"fallback_model":{"type":"string","default":""}}}`

const permissionGuardSchema = `{"type":"object","properties":{"deny_tools":{"type":"array","items":{"type":"string"},"default":[]},"agent_allowlist":{"type":"array","items":{"type":"string"},"default":[]}}}`

const outputPolicySchema = `{"type":"object","properties":{"blocked_patterns":{"type":"array","items":{"type":"string"},"default":[]},"dangerous_command_check":{"type":"boolean","default":true},"block_on_violation":{"type":"boolean","default":true},"replacement_message":{"type":"string","default":""}}}`

const skillTrackerSchema = `{"type":"object","properties":{"track_success":{"type":"boolean","default":true},"track_failure":{"type":"boolean","default":true},"capture_input_preview":{"type":"boolean","default":true},"capture_output_preview":{"type":"boolean","default":true},"max_preview_length":{"type":"integer","default":500}}}`

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
			CallbackPoints: []string{"after_tool"},
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
