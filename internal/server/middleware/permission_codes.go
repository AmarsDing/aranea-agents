package middleware

// 本文件是 P1-6 敏感域权限码登记表（Q7-A 拍板：默认登录可见 + 敏感域逐项收严）。
//
// 数据来源：P0 详设《02-存量rpc权限普查表》§2 敏感域清单（10 域）+
// 2026-11-08 全量 rpc 普查（temp/aranea-agents/test/p0-rpc-census/20261108-rpc-census.tsv，
// 43 service / 546 rpc）+ 各域 proto 的 google.api.http 绑定
// （提取脚本 temp/aranea-agents/test/2026-11-08-rpc-http-extract/extract.py）。
//
// 登记键：gRPC 全方法 `/kratos.<域>.v1.<Svc>/<Method>` +
// HTTP 键 `<大写方法> <pathTemplate>`（同 pathTemplate 多方法，必须带方法区分）。
// 由架构测试 permission_arch_test.go 校验敏感域全覆盖 + 无死键。
//
// 逐 rpc 码位分配规则：
//   - admin：Login/Logout/Current 不登记（登录前/自助路径）；管理面 user.*。
//   - computeruse / sandbox：整域 tool.exec（远控执行面，收严）。
//   - tool：部分收严——执行（TestTool）tool.exec、调用审计 audit.read，
//     其余 25 个 rpc 显式声明"仅需登录"（工具是用户自服务域，Q7-A 不加码）。
//   - monitor：ListAuditLogs → audit.read（审计面）；其余 → system.monitor。
//   - usage：读 → usage.read；写（配额/预算/录入/清理）→ usage.write；
//     导出 → usage.export。
//   - llm_provider_model：管理面 model.route；密钥揭示 → secops.policy（secadmin 专有）。
//
// 角色持有关系见 pkg/auth/permissions.go（P0 详设 01 §3 内置角色映射）。
// 后续 proto option（aranea.auth.permission）codegen 落地后，本表由生成文件替换，
// 键空间与判定语义不变。

var defaultPermissionRegistry = func() *PermissionRegistry {
	r := NewPermissionRegistry()
	registerSensitiveDomainPermissions(r)
	return r
}()

// DefaultPermissionRegistry 返回进程级默认登记表（init 期填充，运行期只读）。
// HTTP/gRPC server 共用。
func DefaultPermissionRegistry() *PermissionRegistry { return defaultPermissionRegistry }

// requireRPC 登记辅助：domain=proto 包中段（如 admin），svc=service 名，
// method=rpc 名，httpMethod/httpPath=HTTP 绑定（无绑定传空串），code=权限码。
func requireRPC(r *PermissionRegistry, domain, svc, rpc, httpMethod, httpPath, code string) {
	r.RequireRPC("/kratos."+domain+".v1."+svc+"/"+rpc, httpMethod, httpPath, code)
}

// requireAuthenticatedRPC 显式声明某 rpc 仅需登录（默认策略），见 RequireAuthenticatedRPC。
func requireAuthenticatedRPC(r *PermissionRegistry, domain, svc, rpc, httpMethod, httpPath string) {
	r.RequireAuthenticatedRPC("/kratos."+domain+".v1."+svc+"/"+rpc, httpMethod, httpPath)
}

func registerSensitiveDomainPermissions(r *PermissionRegistry) {
	// ── ceoboard（12 rpc，整合 P2 并入的经营驾驶舱域）──
	// view 面（查询）= ceoboard.view；ops 面（死信/审计）= ceoboard.ops。
	// 入站 webhook 不走本域 rpc：共享密钥入口直接挂 HTTP server（基础设施层），
	// 与 WS/SPA 同层，不参与本登记表。
	requireRPC(r, "ceoboard", "CeoboardService", "GetOverview", "GET", "/v1/ceoboard/overview", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "GetWorkflow", "GET", "/v1/ceoboard/workflows/{id}", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "GetNode", "GET", "/v1/ceoboard/workflows/{workflow_id}/nodes/{node_id}", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "GetPlatform", "GET", "/v1/ceoboard/platforms/{id}", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "ListConsumers", "GET", "/v1/ceoboard/consumers", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "ListEvents", "GET", "/v1/ceoboard/events", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "GetEvent", "GET", "/v1/ceoboard/events/{uid}", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "GetRegistryReport", "GET", "/v1/ceoboard/registry/report", "ceoboard.view")
	requireRPC(r, "ceoboard", "CeoboardService", "ListDeadLetters", "GET", "/v1/ceoboard/deadletters", "ceoboard.ops")
	requireRPC(r, "ceoboard", "CeoboardService", "ReplayDeadLetter", "POST", "/v1/ceoboard/deadletters/{id}/replay", "ceoboard.ops")
	requireRPC(r, "ceoboard", "CeoboardService", "ListAudit", "GET", "/v1/ceoboard/audit", "ceoboard.ops")
	requireRPC(r, "ceoboard", "CeoboardService", "ExportAudit", "GET", "/v1/ceoboard/audit/export", "ceoboard.ops")

	// ── admin（10 rpc；Login/Logout/Current 豁免）──
	requireRPC(r, "admin", "AdminService", "ListAdmins", "GET", "/v1/admins/list", "user.read")
	requireRPC(r, "admin", "AdminService", "GetAdmin", "GET", "/v1/admins/{id}", "user.read")
	requireRPC(r, "admin", "AdminService", "CreateAdmin", "POST", "/v1/admins/create", "user.write")
	requireRPC(r, "admin", "AdminService", "UpdateAdmin", "PUT", "/v1/admins/update", "user.write")
	requireRPC(r, "admin", "AdminService", "DeleteAdmin", "DELETE", "/v1/admins/{id}", "user.offboard")
	// P1-7 / P1-8 自助面：菜单按主体裁剪下发（读自己的目录）、改密只动自己的口令。
	// 两者都必须是「仅需登录」——加管理面码会让普通用户看不到菜单、改不了密码。
	// 越权面由业务层承担：GetMenus 只读 ctx 主体，ChangePassword 只改 ctx 主体。
	requireAuthenticatedRPC(r, "admin", "AdminService", "GetMenus", "GET", "/v1/auth/menus")
	requireAuthenticatedRPC(r, "admin", "AdminService", "ChangePassword", "POST", "/v1/users/change-password")

	// ── computeruse（3 rpc，整域 tool.exec）──
	requireRPC(r, "computeruse", "ComputerUseService", "KillComputerUseSession", "POST", "/v1/computeruse/sessions/{id}/kill", "tool.exec")
	requireRPC(r, "computeruse", "ComputerUseService", "ListComputerUseSteps", "GET", "/v1/computeruse/sessions/{id}/steps", "tool.exec")
	requireRPC(r, "computeruse", "ComputerUseService", "GetComputerUseStatus", "GET", "/v1/computeruse/status", "tool.exec")

	// ── cron（8 rpc，整域 system.cron）──
	requireRPC(r, "cron", "CronService", "ListCronTasks", "GET", "/v1/cron-tasks", "system.cron")
	requireRPC(r, "cron", "CronService", "CreateCronTask", "POST", "/v1/cron-tasks", "system.cron")
	requireRPC(r, "cron", "CronService", "GetCronTask", "GET", "/v1/cron-tasks/{id}", "system.cron")
	requireRPC(r, "cron", "CronService", "UpdateCronTask", "PATCH", "/v1/cron-tasks/{id}", "system.cron")
	requireRPC(r, "cron", "CronService", "DeleteCronTask", "DELETE", "/v1/cron-tasks/{id}", "system.cron")
	requireRPC(r, "cron", "CronService", "ListCronTaskRuns", "GET", "/v1/cron-task-runs", "system.cron")
	requireRPC(r, "cron", "CronService", "TriggerCronTask", "POST", "/v1/cron-tasks/{id}/trigger", "system.cron")
	requireRPC(r, "cron", "CronService", "ResetCronTaskFailures", "POST", "/v1/cron-tasks/{id}/reset-failures", "system.cron")

	// ── llm_provider_model（8 rpc：管理面 model.route；密钥揭示 secops.policy）──
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "ListProviderModels", "GET", "/v1/llm-provider-models", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "CreateProviderModel", "POST", "/v1/llm-provider-models", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "GetProviderModel", "GET", "/v1/llm-provider-models/{id}", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "UpdateProviderModel", "PATCH", "/v1/llm-provider-models/{id}", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "DeleteProviderModel", "DELETE", "/v1/llm-provider-models/{id}", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "InspectProviderModel", "POST", "/v1/llm-provider-models/inspect", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "ValidateProviderPair", "POST", "/v1/agents/validate-model", "model.route")
	requireRPC(r, "llm_provider_model", "LlmProviderModelService", "RevealProviderModelCredentials", "GET", "/v1/llm-provider-models/{id}/credentials", "secops.policy")

	// ── monitor（19 rpc：ListAuditLogs → audit.read；其余 → system.monitor）──
	requireRPC(r, "monitor", "MonitorService", "ListAuditLogs", "GET", "/v1/monitor/audit", "audit.read")
	requireRPC(r, "monitor", "MonitorService", "DeleteAuditLogs", "DELETE", "/v1/monitor/audit-logs", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListMonitorEvents", "GET", "/v1/monitor/events", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetMonitorEvent", "GET", "/v1/monitor/events/{id}", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListMonitorTraces", "GET", "/v1/monitor/traces", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetMonitorTrace", "GET", "/v1/monitor/traces/{id}", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetMonitorLogs", "GET", "/v1/monitor/logs", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListFlowLogs", "GET", "/v1/monitor/flow-logs", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListMonitorAlertRules", "GET", "/v1/monitor/alert-rules", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListAlertMetrics", "GET", "/v1/monitor/alert-metrics", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "PutMonitorAlertRules", "PUT", "/v1/monitor/alert-rules", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetRunnerMetrics", "GET", "/v1/monitor/runner-metrics", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetCodeExecutorCapabilities", "GET", "/v1/monitor/code-executor-capabilities", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GenerateDiagnosticBundle", "POST", "/v1/monitor/diagnostic-bundle", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "DiagnoseAndHeal", "POST", "/v1/monitor/diagnose-and-heal", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "TriggerSelfCheck", "POST", "/v1/monitor/self-check", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListSelfCheckReports", "GET", "/v1/monitor/self-check-reports", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "GetHealStats", "GET", "/v1/monitor/heal-stats", "system.monitor")
	requireRPC(r, "monitor", "MonitorService", "ListHealRecords", "GET", "/v1/monitor/heal-records", "system.monitor")

	// ── sandbox（3 rpc，整域 tool.exec）──
	requireRPC(r, "sandbox", "SandboxService", "ListSandboxes", "GET", "/v1/sandboxes", "tool.exec")
	requireRPC(r, "sandbox", "SandboxService", "GetSandboxMetrics", "GET", "/v1/sandboxes/metrics", "tool.exec")
	requireRPC(r, "sandbox", "SandboxService", "ForceKillSandbox", "DELETE", "/v1/sandboxes/{id}", "tool.exec")

	// ── self_improvement（11 rpc，整域 system.self_improve）──
	requireRPC(r, "self_improvement", "SelfImprovementService", "ListRuns", "GET", "/v1/self-improvement/runs", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "GetRun", "GET", "/v1/self-improvement/runs/{id}", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "ApproveRun", "POST", "/v1/self-improvement/runs/{id}/approve", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "RejectRun", "POST", "/v1/self-improvement/runs/{id}/reject", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "RollbackRun", "POST", "/v1/self-improvement/runs/{id}/rollback", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "CloseRun", "POST", "/v1/self-improvement/runs/{id}/close", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "ControlRun", "POST", "/v1/self-improvement/runs/{id}/control", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "GetOutcomeStats", "GET", "/v1/self-improvement/outcome-stats", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "GetStatus", "GET", "/v1/self-improvement/status", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "GetRiskRules", "GET", "/v1/self-improvement/risk-rules", "system.self_improve")
	requireRPC(r, "self_improvement", "SelfImprovementService", "UpdateRiskRules", "PUT", "/v1/self-improvement/risk-rules", "system.self_improve")

	// ── system_setting（3 rpc，整域 system.setting）──
	requireRPC(r, "system_setting", "SystemSettingService", "GetSystemSettings", "GET", "/v1/system-settings", "system.setting")
	requireRPC(r, "system_setting", "SystemSettingService", "UpdateSystemSettings", "PUT", "/v1/system-settings", "system.setting")
	requireRPC(r, "system_setting", "SystemSettingService", "TestWebResearch", "POST", "/v1/system-settings/web-research/test", "system.setting")

	// ── tool（3 rpc 收严；其余 25 rpc 显式声明仅需登录，Q7-A 工具自服务面不加码）──
	requireRPC(r, "tool", "ToolService", "TestTool", "POST", "/v1/tools/{id}/test", "tool.exec")
	requireRPC(r, "tool", "ToolService", "ListToolInvocationAudits", "GET", "/v1/tools/audits", "audit.read")
	requireRPC(r, "tool", "ToolService", "GetToolInvocationParams", "GET", "/v1/tools/runs/{invocation_id}/params", "audit.read")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListTools", "GET", "/v1/tools")
	requireAuthenticatedRPC(r, "tool", "ToolService", "GetTool", "GET", "/v1/tools/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "CreateTool", "POST", "/v1/tools")
	requireAuthenticatedRPC(r, "tool", "ToolService", "UpdateTool", "PUT", "/v1/tools/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "DeleteTool", "DELETE", "/v1/tools/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ToggleToolEnabled", "PATCH", "/v1/tools/{id}/enabled")
	requireAuthenticatedRPC(r, "tool", "ToolService", "UpdateToolConfig", "PUT", "/v1/tools/{id}/config")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolRuns", "GET", "/v1/tools/runs")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolRunsForTool", "GET", "/v1/tools/{tool_id}/runs")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolAgentOverrides", "GET", "/v1/tools/{tool_id}/agent-overrides")
	requireAuthenticatedRPC(r, "tool", "ToolService", "GetToolAgentBindings", "GET", "/v1/tools/{tool_id}/agent-bindings")
	requireAuthenticatedRPC(r, "tool", "ToolService", "UpsertToolAgentOverride", "PUT", "/v1/tools/{tool_id}/agent-overrides/{agent_id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "DeleteToolAgentOverride", "DELETE", "/v1/tools/{tool_id}/agent-overrides/{agent_id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolAgentOverridesByAgent", "GET", "/v1/agents/{agent_id}/tool-overrides")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolGrants", "GET", "/v1/agents/{agent_id}/tool-grants")
	requireAuthenticatedRPC(r, "tool", "ToolService", "DeleteToolGrant", "DELETE", "/v1/agents/{agent_id}/tool-grants/{tool_key}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListToolParamRules", "GET", "/v1/tool-param-rules")
	requireAuthenticatedRPC(r, "tool", "ToolService", "UpsertToolParamRule", "PUT", "/v1/tool-param-rules/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "DeleteToolParamRule", "DELETE", "/v1/tool-param-rules/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ListApprovalPolicies", "GET", "/v1/approval-policies")
	requireAuthenticatedRPC(r, "tool", "ToolService", "GetApprovalPolicy", "GET", "/v1/approval-policies/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "UpsertApprovalPolicy", "PUT", "/v1/approval-policies/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "DeleteApprovalPolicy", "DELETE", "/v1/approval-policies/{id}")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ExportApprovalPoliciesTOML", "GET", "/v1/approval-policies:exportToml")
	requireAuthenticatedRPC(r, "tool", "ToolService", "ImportApprovalPoliciesTOML", "POST", "/v1/approval-policies:importToml")

	// ── usage（16 rpc：读 usage.read / 写 usage.write / 导出 usage.export）──
	requireRPC(r, "usage", "UsageService", "GetUsageOverview", "GET", "/v1/usage/overview", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListUsageTrends", "GET", "/v1/usage/trends", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListTopModels", "GET", "/v1/usage/top-models", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListTopAgents", "GET", "/v1/usage/top-agents", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListUsageEvents", "GET", "/v1/usage/events", "usage.read")
	requireRPC(r, "usage", "UsageService", "GetUsageQuota", "GET", "/v1/usage/quotas/{scope_type}/{scope_id}", "usage.read")
	requireRPC(r, "usage", "UsageService", "CheckUsageQuota", "GET", "/v1/usage/quotas/{scope_type}/{scope_id}/check", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListBudgetAlerts", "GET", "/v1/usage/budget-alerts", "usage.read")
	requireRPC(r, "usage", "UsageService", "ListAllModelsBreakdown", "GET", "/v1/usage/all-models-breakdown", "usage.read")
	requireRPC(r, "usage", "UsageService", "GetContextBudgetStats", "GET", "/v1/usage/context-budget-stats", "usage.read")
	requireRPC(r, "usage", "UsageService", "GetCacheHitRatioStats", "GET", "/v1/usage/cache-hit-ratio-stats", "usage.read")
	requireRPC(r, "usage", "UsageService", "RecordTokenUsageEvent", "POST", "/v1/usage/token-events", "usage.write")
	requireRPC(r, "usage", "UsageService", "SetUsageQuota", "PUT", "/v1/usage/quotas/{scope_type}/{scope_id}", "usage.write")
	requireRPC(r, "usage", "UsageService", "SetBudgetAlert", "POST", "/v1/usage/budget-alerts", "usage.write")
	requireRPC(r, "usage", "UsageService", "PurgeUsageEvents", "POST", "/v1/usage/events/purge", "usage.write")
	requireRPC(r, "usage", "UsageService", "ExportUsageEvents", "GET", "/v1/usage/events/export", "usage.export")
}
