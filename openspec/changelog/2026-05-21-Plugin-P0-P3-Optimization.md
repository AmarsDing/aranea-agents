# Plugin P0–P3 架构收敛与产品化

**日期**：2026-05-21 · **模块**：Plugin (22)

## 摘要

按 Review 结论完成 P0 架构收敛、P1 质量补全、P2 API/Schema 对齐、P1 前端产品化；文档同步 `22-plugin-development.md`。

## P0 架构

- **回调编排边界**：`internal/plugin/trpc/orchestration.go` 文档化 Runner Plugin / Chain / ModelSelector / Hook 四层分工。
- **model_router 单一路由**：BeforeModel 仅 telemetry；catalog 路由仅 `agent.PluginModelSelector`。
- **cost_guard 分工**：blocked_models 由 ModelSelector；BeforeModel 负责 token 预算。
- **permission_guard**：`confirm_tools` 不再等同 `deny_tools`；确认走 Chain / confirmation_guard。
- **OnEvent scope**：`Manager.OnEvent` / hook on_event 解析 platform agent_id。

## P1 质量

- **audit_log**：统一 `PluginSafeLogger`，移除裸 stderr。
- **CallbackTelemetry**：`stats.go` 拆分 `CallbackEvent`，Run 写入 agent/session/action。
- **Bootstrap**：`NewPluginServiceWithBootstrap`，构造器无副作用。
- **UpdateScope**：校验 agent_id 存在。

## P2

- **ListPluginRuns** 筛选：agent_id、callback_point、status、from/to。
- **skill_usage_tracker** Schema 与实现对齐产品字段。
- **sensitive_data_mask** 支持 `custom_patterns`。

## P1 前端

- API：`updatePluginScope` / `updatePluginSortOrder` / `listPluginRuns`。
- 页面：`/plugins/runs`；管理页作用域、排序、服务端分页。

## P3（未实现，文档保留）

- 插件沙箱（gRPC/MCP/WASM 方案）。
- `plugin_versions` 版本与回滚。
