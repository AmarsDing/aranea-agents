# 23 Tools Review

> **评分**：80 / 100 | **风险等级**：P1  
> **文档**：[23-tools-development.md](../需求/23-tools-development.md)  
> **代码锚点**：`internal/tools/` · `internal/tools/trpc/` · `internal/biz/tool.go` · `internal/biz/tool_catalog_runtime.go` · `web/src/pages/ToolsPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | ToolOverride/requires_confirmation/统计/TestTool/Agent 覆盖面板均已落地；`23-tools-development.md` 标注"待同步" |
| 架构一致性 | 22 | 25 | `internal/tools` 作为工具注册与装配层职责清晰；`tools/trpc` 作为框架适配层 ✅；`trpc/toolsets.go` 构建 ToolSet |
| 后端实现质量 | 17 | 20 | Catalog/Policy/Runtime/Invocation 分层初具形态；调用审计落库 ✅；MCP 认证/重连/Broker 主路径已通 |
| 前端实现质量 | 13 | 15 | Tool 目录管理 ✅；TestTool ✅；Agent Override 面板 ✅；调用记录 `/tools/runs` + `/tools/audits` ✅ |
| 测试与验证 | 6 | 10 | `pkg/trpc-agent-go/internal/tool/tool_test.go` 框架层有测试；Aranea 适配层测试待补 |
| 文档一致性 | 6 | 10 | 开发计划标注"待本轮同步"；changelog 已同步 P2–P4 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Tool 目录（Catalog） | ✅ |
| Tool 分类/风险级 | ✅ |
| Tool Schema 编辑 | ✅ |
| `requires_confirmation` 策略 | ✅ |
| TestTool（沙箱调用） | ✅ |
| Agent 级 ToolOverride 运行时 | ✅ |
| Agent 覆盖面板（AgentToolOverridesPanel）| ✅ |
| 调用统计 | ✅ |
| 调用审计落库 | ✅ |
| `/tools/runs` 调用记录页 | ✅ |
| `/tools/audits` 审计页 | ✅ |
| MCP 默认超时 60s | ✅ |
| Tool 结果缓存（`cache_enabled`）| ✅ |
| MCP Broker 默认发现 | 🟡 文档化待补 |
| 工具确认 UI（`await_kind`）| ✅ |

---

## 分层架构

```
Tool Catalog (internal/biz/tool.go) — 工具注册与元数据
    ↓
Tool Policy (tool_catalog_runtime.go) — Override/confirmation
    ↓
Tool Runtime (tools/toolset.go) — 装配 ToolSet
    ↓
Tool Invocation (tool_audit.go) — 调用记录
```

**状态**：四层初具形态，但分层文件组织可再明确（文档化）。

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| TOOL-P1-01 | `23-tools-development.md` 标注"待同步"：MCP Broker 默认发现、认证文档化缺失 | 本迭代补全 Tools 开发计划文档 |
| TOOL-P1-02 | Aranea Tools 适配层（`internal/tools/trpc/`）无专项测试 | 补工具挂载路径测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| TOOL-P2-01 | `ToolAuditsPage` 在路由中存在但未列入 `frontend-pages.md` 侧栏导航表 | 更新 `frontend-pages.md` |
| TOOL-P2-02 | TestTool 沙箱对网络/文件系统的权限边界未明确 | 在 Tool 配置中添加沙箱策略说明 |
| TOOL-P2-03 | 内置工具种子（`builtin_tools_seed.go`）与 MCP 工具的优先级/覆盖规则未文档化 | 在 `23 tools.md` 中补充优先级规则 |

---

## 建议优化路径

1. 同步 `23-tools-development.md` 文档（标注已通项）。
2. 补 Tools 适配层测试。
3. 文档化 MCP Broker 默认发现逻辑。
4. 更新 `frontend-pages.md` 的 `/tools/audits` 路由记录。
