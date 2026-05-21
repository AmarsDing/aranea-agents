# 02–08 Agent 模块 Review

> **评分**：80 / 100 | **风险等级**：P1  
> **文档**：[2-8-agent-modules-development.md](../需求/2-8-agent-modules-development.md) · 各子模块 `需求/2-8 <name>*.md`  
> **代码锚点**：`internal/biz/agent_*` · `internal/agent/` · `web/src/pages/AgentSettingsPage.vue` · `web/src/pages/agent-settings/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 创建/查重/设置/文件/进化/标题/复制主要功能已落地；批量/迁移和 `GenerateAgentTitle` 待补 |
| 架构一致性 | 21 | 25 | `BuildTRPCLLMAgent` 正确隔离在 `internal/agent` ✅；`builder_deps.go` 分组类型 ✅；设置页体量偏大 |
| 后端实现质量 | 17 | 20 | CRUD + A2A Proxy + 查重 + 配置合并 + 进化 Scanner + AI 编辑 RPC 均已实现 |
| 前端实现质量 | 14 | 15 | 设置页按 Tab 拆分；列表运行态徽章 ✅；`AgentSettingsPage.vue` 仍约 488 行 |
| 测试与验证 | 6 | 10 | `agent_usecase_kind_test.go`、`agent_prompt_ai_test.go` 等已有；进化 Scanner 无测试 |
| 文档一致性 | 6 | 10 | `2-8-agent-modules-development.md` 横切状态文档同步；部分模块（8 title）待完善 |

---

## 子模块覆盖

### 2. Agent Create

| 功能 | 状态 |
|------|------|
| LLM Agent 创建（所有字段） | ✅ |
| A2A Proxy 远程代理创建 | ✅ |
| `CheckAgentKey` 防抖查重 | ✅ I8 |
| `ListAgentTemplates` 模板 | ✅ I10 |
| 模板全字段填充 | ✅ I10-LIST-02 |
| 结构化创建错误（inline reason） | ✅ I10-LIST-02 |
| `created_by` 字段 / 过滤 | ✅ |
| 批量创建 | ❌ |

### 3. Agent List

| 功能 | 状态 |
|------|------|
| 卡片 + 表格视图 | ✅ |
| 分类/Provider/状态筛选 | ✅ |
| `last_run_status` / `last_run_at` 聚合 | ✅ `ListExtrasForAgents` |
| `pending_evolution_count` 徽章 | ✅ |
| `DuplicateAgent` 复制（深拷贝） | ✅ I10 |
| `created_by` 列 + `ListAgentCreators` | ✅ LIST-02 |
| 批量操作（LIST-04） | ❌ P2 |
| 迁移功能 | ❌ |

### 4. Agent Type / 分类

| 功能 | 状态 |
|------|------|
| Platform 分类树 | ✅ |
| Agent 绑定分类 | ✅ |
| 拖拽排序 | ❌ |
| 关联统计 | ❌ |

### 5. Agent Setting

| 功能 | 状态 |
|------|------|
| 系统提示模式 | ✅ |
| Provider/Model 选择 | ✅ |
| 规划模式（`planner_kind`） | ✅ `AgentPlannerSection` |
| 能力/工具配置 | ✅ |
| 头像选择 | ✅ |
| 记忆/Skill/权限 Tab | ✅ |
| A2A Tab（Endpoint/Proxy） | ✅ |
| 用户实例配置 | ✅ |
| `config_json` PATCH 浅合并 | ✅ `MergeAgentConfigJSON` |
| 设置页再瘦身（< 300 行） | 🟡 P2，约 488 行 |

### 6. Agent Setting File

| 功能 | 状态 |
|------|------|
| 提示文件 CRUD | ✅ |
| 文件注入（系统提示组合） | ✅ |
| `EditPromptFileByAI` RPC | ✅ I10 |
| `PromptFileAIEditor` 前端 | ✅ I10 |

### 7. Agent Evolution

| 功能 | 状态 |
|------|------|
| `EvolutionScanner` 30min worker | ✅ I10 |
| 阈值建议 | ✅ |
| API + 指标 + Scanner | ✅ |
| 进化 chip + `pending_evolution_count` | ✅ |
| 趋势图 / diff | ❌ AGT-16 P3 |
| 护栏 | ❌ |

### 8. Agent Title

| 功能 | 状态 |
|------|------|
| 顶栏展示 + 预览 | ✅ |
| 手动编辑标题 | ✅ |
| `GenerateAgentTitle` 自动生成 | ❌ P3 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| AGT-P1-01 | `AgentSettingsPage.vue` 约 560 行（目标 < 300 行）；测试覆盖缺失 | 继续按 Tab 拆分子组件；补 Tab 页单测 |
| AGT-P1-02 | `EvolutionScanner` 无单测；TTL 策略（30 min）缺乏异常路径测试 | 补 Scanner 单测，含异常和无数据场景 |
| AGT-P1-03 | `ListExtrasForAgents` 批量 RPC 若 Agent 数量大（> 1000）可能性能问题 | 加 limit 分页；或改为服务端聚合 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| AGT-P2-01 | 批量操作（LIST-04）和迁移功能未实现 | 规划 LIST-04 sprint |
| AGT-P2-02 | `GenerateAgentTitle` 自动生成缺失 | 规划 P3 LLM 自动标题 RPC |
| AGT-P2-03 | 进化趋势图（AGT-16）未实现 | 规划前端趋势组件 |

---

## 前端分层评价

```
AgentSettingsPage.vue (488 行) — 目标 < 300 行
    ├─ pages/agent-settings/AgentTab.vue ✅ 拆分
    ├─ pages/agent-settings/MemoryTab.vue ✅
    ├─ pages/agent-settings/SkillTab.vue ✅
    └─ ... 更多 Tab 子组件
features/agents/useAgentsPage.ts — 编排
features/agents/api.ts — HTTP 门面
stores/agents + stores/agents/detail — 状态
```

**问题**：设置页壳仍偏重；`useAgentSettingsPage.ts` 需验证行数是否合理。

---

## 建议优化路径

1. 拆分 `AgentSettingsPage.vue` 至 < 300 行。
2. 补 `EvolutionScanner` 单测。
3. 规划批量操作（LIST-04）。
4. `GenerateAgentTitle` LLM 自动生成（P3）。
