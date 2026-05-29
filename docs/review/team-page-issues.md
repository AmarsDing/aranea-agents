# Team 管理页面问题清单

> 审查日期：2026-05-29
> 审查范围：`web/src/pages/TeamsPage.vue` 及其所有子组件、composable、store、api、后端 service 层

---

## 一、页面 UI 功能概览

### 1.1 页面结构

| 区域 | 组件 | 功能 |
|------|------|------|
| 页头 | `AppPageHero` | 标题 + "新增 Team" 按钮 |
| 工具栏 | `TeamToolbar` | 搜索 / 编排模式筛选 / 状态筛选 / 行业筛选 / 刷新 |
| 列表区 | `TeamCard` × N | 按行业分组展示 Team 卡片 |
| 编辑弹窗 | `TeamEditorDialog` | 新增/编辑 Team（基础信息 + 编排模式 + 成员 + 运行时策略 + A2A + 编译预览） |
| 运行轨迹 | `TeamRunsDialog` | 查看运行历史 + 步骤 + 汇总 + 实时 WS 事件 |
| 测试弹窗 | `TeamTestDialog` | API 级别运行测试 |

### 1.2 数据流追踪

```
services/kratos/team/v1/index.ts (protoc 生成 gRPC-Web 客户端)
  → features/teams/api.ts (wireTeam/wireRun/wireStep 类型归一化)
    → stores/teams/page.ts (useTeamsPageStore: 薄 API 门面)
      → features/teams/useTeamsPage.ts (composable: 状态 + 逻辑)
        → pages/TeamsPage.vue (布局 + 传参)
          → components/teams/*.vue (纯展示: props in / emits out)
```

---

## 二、发现的问题

### P0 — 严重（功能缺失）

#### BUG-01: `has_active_run` 在 ListTeams 响应中始终为 false

- **现象**：TeamCard 上的"运行观测台"按钮（insights 图标）永远不显示；TeamOrchestratePage 无法连接实时运行流
- **根因**：
  - 后端 `toProtoTeam()` 不设置 `HasActiveRun` 字段
  - `ListTeams` RPC 不调用 `HasActiveRun` 检查
  - 仅 `GetTeam` RPC 填充了该字段（`team.go:220-221`）
- **影响范围**：
  - `TeamCard.vue:62` — `v-if="team.has_active_run"` 永远为 false
  - `useTeamOrchestratePage.ts:131` — `connectLiveRun()` 提前返回
  - `useTeamOrchestratePage.ts:156,162` — 编排页只读模式无法激活
- **数据追踪**：
  ```
  api.ts:wireTeam() → t?.hasActiveRun ?? false  (映射正确)
  team.go:toProtoTeam() → 未设置 HasActiveRun   (根因)
  team.go:ListTeams() → 只调 toProtoTeam()       (未补充)
  ```
- **修复方案**：后端 `ListTeams` 中为每个 team 调用 `uc.HasActiveRun()` 填充字段

---

### P1 — 高（数据完整性）

#### BUG-02: 编辑→新增切换时 definition 可选属性残留

- **现象**：编辑一个含 `failure_policy` 的 Team 后，点"新增 Team"，新 Team 的 definition_json 中会包含上一个 Team 的 `failure_policy`
- **根因**：`Object.assign(definition, defaultDefinition())` 不会删除目标对象上源对象不存在的属性
- **残留属性**：`failure_policy`、`linked_graph_id`、`synthesizer_agent_id`、`enable_checkpoint`
- **影响文件**：`useTeamsPage.ts:127-128`
- **修复方案**：重置时先删除所有可选属性，再 assign

#### BUG-03: applyTemplate 不完全重置 definition

- **现象**：应用模板后，之前编辑的 `failure_policy` 等可选属性仍残留
- **根因**：同 BUG-02，`Object.assign(definition, definitionFromTemplate(...))` 不删除多余属性
- **影响文件**：`useTeamsPage.ts:184`
- **修复方案**：同 BUG-02，先清理再赋值

---

### P2 — 中（UX / 代码质量）

#### BUG-04: `pickAgentID` 函数为死代码

- **现象**：`teamUtils.ts:180-182` 定义了 `pickAgentID` 但从未被调用
- **修复方案**：删除

#### BUG-05: TeamRunsDialog 未处理 "cancelled" 状态

- **现象**：已取消的运行/步骤显示为灰色 + "schedule" 图标，语义不明确
- **根因**：`stepStatusColor` / `runStatusIcon` 只处理了 running/pending/success/failed/error
- **影响文件**：`TeamRunsDialog.vue:110-128`
- **修复方案**：添加 cancelled → warning 色 + cancel 图标

#### BUG-06: findActiveTeamRun 仅检查前 20 条运行

- **现象**：若活跃运行不在最近 20 条内，`openTeamObservatory` 会错误地导航到编排页而非观测台
- **影响文件**：`api.ts:197`
- **修复方案**：将 limit 从 20 提升到 50（后端已有 `HasActiveTeamRun` 按状态索引查询，长期应改用专用接口）

#### BUG-07: stepsByRun 未在切换 Team 时清理

- **现象**：打开 A Team 的运行轨迹后关闭，再打开 B Team 的运行轨迹，A Team 的步骤数据仍在内存中
- **影响文件**：`useTeamsPage.ts:248-254`
- **修复方案**：`openRuns` 中增加 `stepsByRun.value = {}`

---

### P3 — 低（设计层面）

#### DESIGN-01: useTeamsStore 与 useTeamsPageStore 状态不一致

- **现象**：两个 Store 各自维护独立的 team 缓存，composable 使用 `useTeamsPageStore` 而 `TeamOrchestratePage` 使用 `useTeamsStore`，变更不会互相同步
- **建议**：长期考虑合并或通过事件总线同步

---

## 三、修复优先级

| 优先级 | 编号 | 修复类型 | 涉及文件 |
|--------|------|----------|----------|
| P0 | BUG-01 | 后端 | `internal/service/team.go` |
| P1 | BUG-02 | 前端 | `web/src/features/teams/useTeamsPage.ts` |
| P1 | BUG-03 | 前端 | `web/src/features/teams/useTeamsPage.ts` |
| P2 | BUG-04 | 前端 | `web/src/components/teams/teamUtils.ts` |
| P2 | BUG-05 | 前端 | `web/src/components/teams/TeamRunsDialog.vue` |
| P2 | BUG-06 | 前端 | `web/src/features/teams/api.ts` |
| P2 | BUG-07 | 前端 | `web/src/features/teams/useTeamsPage.ts` |

---

## 四、aranea-review 代码审查报告

> 审查日期：2026-05-29
> 审查范围：BUG-01 ~ BUG-07 修复代码（5 个文件）
> 审查依据：`aranea-review` SKILL 结构化审查清单

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 1 | 0 | 1 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 1 | 0 | 1 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 0 | 0 | 0 | 0 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — 聊天消息分组** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |
| **合计** | **0** | **2** | **0** | **2** |

### 阻断项（必须修复）

无。

### 建议项（推荐修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| S01 | 后端 — 架构合规 (BA4) | 后端 | `internal/service/team.go:187-193` | `ListTeams` 在循环中逐个调用 `uc.HasActiveRun()`，形成 N+1 查询。当 team 数量较多时，每个 team 都触发一次 `SELECT COUNT(*) FROM team_runs WHERE team_id=? AND status IN (...)` 数据库查询。当前 `HasActiveTeamRun` 实现使用了 `Limit(1).Count(ctx)` 且有 status 索引，单次查询效率尚可，但 N 次循环累积仍可能成为瓶颈。 | 长期方案：在 biz 层增加 `BatchHasActiveRun(teamIDs []string) (map[string]bool, error)` 批量查询方法，data 层用单条 `SELECT team_id, COUNT(*) FROM team_runs WHERE team_id IN (?) AND status IN (...) GROUP BY team_id` 实现。短期可接受当前实现，因 team 数量通常有限（<50）。 |
| S02 | 后端 — 错误处理 (BE4) | 后端 | `internal/service/team.go:189` | `if active, aerr := s.uc.HasActiveRun(ctx, items[i].ID); aerr == nil` 静默吞掉了 `aerr` 错误。当 `HasActiveRun` 查询失败时，`pb.HasActiveRun` 保持默认值 `false`，不会导致页面崩溃，但运维无法感知数据库异常。 | 建议在 `aerr != nil` 分支添加 `FlowLog` 记录：`event.FlowLog.Warn("team.has_active_run_check_failed", ...)`。不改为返回错误，因为单个 team 的检查失败不应阻断整个列表。 |

### 提示项（记录备忘）

无。

### 亮点

1. **resetDefinition 设计合理**：`teamUtils.ts` 中的 `resetDefinition()` 函数显式列出所有可选 key 并逐一 `delete`，再 `Object.assign` 赋值。这比 `Object.keys(target).forEach(k => delete target[k])` 更安全——只清理已知可选属性，不会误删 `version`/`mode` 等必要字段。`optionalKeys` 列表与 `TeamDefinition` 类型声明对齐，可维护性好。

2. **stepsByRun 清理时机正确**：`openRuns` 中 `stepsByRun.value = {}` 放在 `runsOpen.value = true` 之前，确保切换 team 时旧数据被完全清除，避免跨 team 步骤数据串扰。同时 `summariesByRun.value = {}` 也一并清理，保持一致性。

3. **cancelled 状态双拼写处理**：`TeamRunsDialog.vue` 同时处理了 `cancelled`（双 l）和 `canceled`（单 l）两种拼写，覆盖了英式/美式英语差异，也兼容了后端可能的状态值不一致。

4. **展示组件纯度保持**：`TeamRunsDialog.vue` 修复后仍严格遵守 props/emits 模式，未引入任何 Store 或 API 直接调用，符合前端红线 #1。

5. **findActiveTeamRun limit 调整合理**：从 20 提升到 50，与 `listTeamRuns` 默认参数一致，降低了遗漏活跃运行的概率。同时文档中已标注长期应改用后端 `HasActiveTeamRun` 专用接口。

### 后端合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] Runner 装配在 Service 层
- [x] Service 层无业务逻辑（`ListTeams` 中的 `HasActiveRun` 调用属于数据填充/编排，非业务逻辑）
- [x] 跨模块通过窄接口
- [x] Wire 绑定在 Service 层
- [x] 无工具生成代码的手动修改
- [x] goroutine 走 safego（本次修改未涉及 goroutine）
- [x] 业务错误用 kerrors（`team.go` 中无 `fmt.Errorf`）
- [x] 日志用 FlowLog（本次修改未涉及日志）
- [x] 共享状态有锁保护（本次修改未涉及共享状态）
- [x] 无上帝对象注入
- [x] 接口方法 ≤ 5
- [x] Repository 接口方法 ≤ 5（否则拆子接口）

### 前端合规性清单

- [x] 展示组件无 Store/API import（`TeamRunsDialog.vue` 仅 import types + teamUtils 纯函数）
- [x] Page 无直接 API import（`TeamsPage.vue` 通过 composable 访问）
- [x] Dialog/浮层 emit 而非内部调 API（`TeamRunsDialog.vue` 全部通过 `$emit`）
- [x] 新 HTTP 调用在 api.ts（`findActiveTeamRun` 在 `api.ts` 中）
- [x] 跨 Store 同步走 sessionSync 事件总线（本次修改未涉及跨 Store 同步）
- [x] 聊天消息分组用堆栈模型（本次修改未涉及聊天消息）
- [x] 浮层 backdrop-filter 成对（本次修改未涉及浮层样式）
- [x] 主按钮用 --color-accent（本次修改未涉及按钮样式）
- [x] Dialog 用 app-dialog-card（本次修改未涉及 Dialog 样式）
- [x] Registry 表格用 AppRegistryTable + registryCol()（本次修改未涉及表格）
- [x] 表格列定义在 *Ui.ts（非 .vue 内）（本次修改未涉及表格）
- [x] Page script ≤~200 行（`TeamsPage.vue` 逻辑在 composable 中，Page 本身很薄）

### 逐文件审查详情

#### 1. `internal/service/team.go`（BUG-01 修复）

**修改内容**：`ListTeams` 方法中为每个 team 调用 `uc.HasActiveRun()` 填充 `HasActiveRun` 字段。

| 检查项 | 结果 | 说明 |
|--------|------|------|
| BA2: biz 不 import trpc-agent-go | ✅ 通过 | 无新增 import |
| BA3: biz 不 import proto | ✅ 通过 | 无新增 import |
| BA4: Service 层无业务逻辑 | ✅ 通过 | `HasActiveRun` 调用属于数据填充编排，非业务逻辑 |
| BL1: 类型转换用 toProtoXxx | ✅ 通过 | 继续使用 `toProtoTeam` |
| BL2: 错误映射用 kerrors | ✅ 通过 | 无新增错误返回 |
| BC2: 跨层调用传 ctx | ✅ 通过 | `s.uc.HasActiveRun(ctx, items[i].ID)` 正确传递 ctx |
| BE4: 不吞错误 | 🟡 建议 | `aerr` 被静默忽略，见 S02 |

#### 2. `web/src/components/teams/teamUtils.ts`（BUG-02/03/04 修复）

**修改内容**：新增 `resetDefinition()` 函数；删除死代码 `pickAgentID`。

| 检查项 | 结果 | 说明 |
|--------|------|------|
| FD1: 展示组件无 Store/API import | ✅ 通过 | `teamUtils.ts` 是纯函数工具文件，无 Store/API 依赖 |
| FL4: 类型从 types.ts 引入 | ✅ 通过 | `import type { Team, TeamDefinition, ... } from "../../features/teams/types"` |
| FB1: 数据转换在正确层 | ✅ 通过 | `resetDefinition` 是纯数据转换函数，放在组件共址工具文件中合理 |
| optionalKeys 完整性 | ✅ 通过 | 覆盖了 `failure_policy`、`linked_graph_id`、`synthesizer_agent_id`、`enable_checkpoint`、`team_graph_runtime`、`graph` 六个可选属性，与 `TeamDefinition` 类型定义对齐 |
| 死代码清理 | ✅ 通过 | `pickAgentID` 已删除，`pickAgent` 保留（被 `templateMember` 使用） |

#### 3. `web/src/features/teams/useTeamsPage.ts`（BUG-02/03/07 修复）

**修改内容**：`openCreate` 和 `applyTemplate` 使用 `resetDefinition`；`openRuns` 增加 `stepsByRun.value = {}`。

| 检查项 | 结果 | 说明 |
|--------|------|------|
| FD2: Page 无直接 API import | ✅ 通过 | 通过 `useTeamsPageStore` 访问 |
| FD4: Dialog 不内部调 API | ✅ 通过 | Dialog 通过 emit 交互 |
| FD6: HTTP 调用在 api.ts | ✅ 通过 | `findActiveTeamRun` 在 api.ts |
| resetDefinition 使用正确性 | ✅ 通过 | `openCreate` 中先 reset 再打开编辑器；`applyTemplate` 中先 reset 再赋值模板 |
| stepsByRun 清理时机 | ✅ 通过 | 在 `openRuns` 中 `runsOpen.value = true` 之前清理 |
| summariesByRun 清理 | ✅ 通过 | 与 stepsByRun 同步清理 |

#### 4. `web/src/components/teams/TeamRunsDialog.vue`（BUG-05 修复）

**修改内容**：`stepStatusColor` 和 `runStatusIcon` 增加 cancelled/canceled 状态处理。

| 检查项 | 结果 | 说明 |
|--------|------|------|
| FD1: 展示组件无 Store/API import | ✅ 通过 | 仅 import types + teamUtils 纯函数 |
| FL1: 组件在 components/<域>/ | ✅ 通过 | 位于 `components/teams/` |
| FL4: 类型从 types.ts 引入 | ✅ 通过 | `import type { ... } from "../../features/teams/types"` |
| cancelled 状态处理完整性 | ✅ 通过 | `stepStatusColor` 返回 "warning"，`runStatusIcon` 返回 "cancel"，语义清晰 |
| 双拼写覆盖 | ✅ 通过 | 同时处理 `cancelled` 和 `canceled` |

#### 5. `web/src/features/teams/api.ts`（BUG-06 修复）

**修改内容**：`findActiveTeamRun` 的 `listTeamRuns` 调用 limit 从 20 提升到 50。

| 检查项 | 结果 | 说明 |
|--------|------|------|
| FD6: HTTP 调用在 api.ts | ✅ 通过 | `findActiveTeamRun` 在 api.ts 中 |
| limit 值合理性 | ✅ 通过 | 50 与 `listTeamRuns` 默认参数一致，覆盖更广 |
| 长期方案标注 | ✅ 通过 | 问题文档已标注应改用后端 `HasActiveTeamRun` 专用接口 |
