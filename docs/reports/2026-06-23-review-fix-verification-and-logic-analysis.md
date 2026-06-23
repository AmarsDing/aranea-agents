# 修复验证与逻辑分析报告

> **报告日期**：2026-06-23
> **对应审查报告**：[2026-06-23-audit-frontend-api-business-logic.md](./2026-06-23-audit-frontend-api-business-logic.md)
> **修复范围**：第四阶段 11 个阻断问题的修复验证与逻辑分析
> **验证方法**：后端 `go build` + `go test`、前端 `pnpm lint` + `pnpm test` + `pnpm build`、逐文件代码复核

---

## 一、验证结果概要

### 1.1 构建与测试验证

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 后端编译 | `go build ./internal/service/... ./internal/biz/... ./internal/data/...` | ✅ 通过 |
| 后端测试 | `go test ./internal/biz/... ./internal/service/...` | ✅ 通过（exit code 0） |
| 前端 lint | `pnpm lint` | ✅ 通过（无新增违规） |
| 前端测试 | `pnpm test --run` | ✅ 通过（82 文件 / 499 用例） |
| 前端构建 | `pnpm build` | ✅ 通过（SPA 编译成功） |

### 1.2 修复问题统计

| 编号 | 问题 | 域 | 修复状态 | 逻辑分析结论 |
|------|------|----|----------|-------------|
| A-3 | agent-categories 死代码 | Agent | ✅ 已删除 | ✅ 正确解决，未引入新问题 |
| C-2 | plan/api.ts 死代码 | Chat | ✅ 已删除 | ✅ 正确解决，未引入新问题 |
| C-3 | EnqueueUserMessage 错误类型 | Chat | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| T-1 | listTeamRuns URL 错误 | Teams | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| T-3 | toProtoSpiritTeamView 字段缺失 | Teams | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| T-4 | TimeTravelGraph 忽略 stepIndex | Graph | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| T-5 | RetryTeam 注释误导 | Teams | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| S-2 | getSkillHealth 字段未定义 | Skills | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| M-1 | UpsertMemoryFact 未透传 PIITypes | Memory | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| P-2 | UpdateEvalLLM 无守卫非原子 | Platform | ✅ 已修复 | ✅ 正确解决，未引入新问题 |
| P-3 | LlmProviderModel 字段覆盖 | Platform | ✅ 已修复 | ✅ 正确解决，未引入新问题 |

### 1.3 关闭问题

| 编号 | 问题 | 关闭原因 |
|------|------|----------|
| T-6 | Teams/Graph 其他问题 | 无法确认（缺少上下文） |
| P-1 | WebResearch 默认值覆盖 | 误报（`??` 语义误判） |

---

## 二、逐项逻辑分析

### A-3：agent-categories 死代码删除

**问题描述**：前端 `web/src/features/agent-categories/api.ts` 调用不存在的后端端点，后端无 proto/service/route 定义，`web/src/services/kratos/agent_category/v1/index.ts` 为孤立生成代码。

**修改方案**：
1. 删除 `web/src/features/agent-categories/api.ts`
2. 删除 `web/src/services/kratos/agent_category/v1/index.ts`（含目录）
3. 删除 `web/src/services/index.ts` 中的 `createAgentCategoryService` 函数及对应 import

**修复过程发现**：初次修复遗漏了 `index.ts` 中的 `createAgentCategoryService` 函数（引用未定义的 `createAgentCategoryServiceClient`）。由于该函数从未被任何模块 import，Vite 构建做了 tree-shaking 未报错，但这是潜在的运行时炸弹。本轮验证时发现并补删。

**逻辑分析**：
- ✅ **正确解决**：删除后无任何前端组件引用 `createAgentCategoryService`（Grep 确认仅 `index.ts` 自身定义，无外部调用）
- ✅ **未引入新问题**：前端 lint/test/build 全部通过，证明无断裂引用
- **影响域**：仅前端，无后端联动

---

### C-2：plan/api.ts 死代码删除

**问题描述**：`web/src/features/plan/api.ts` 调用不存在的 plan 端点，后端 biz/data 层存在但未接入 Wire DI，无 proto/service/route，前端无消费者。

**修改方案**：
1. 删除 `web/src/features/plan/api.ts`
2. 删除 `web/src/services/index.ts` 中的 `createPlanService` 函数（18 行死代码）

**逻辑分析**：
- ✅ **正确解决**：Grep 确认 `createPlanService` 在 `web/src` 中无任何引用
- ✅ **未引入新问题**：前端 lint/test/build 全部通过
- **影响域**：仅前端，无后端联动

---

### C-3：EnqueueUserMessage 错误类型修正

**问题描述**：[chat.go:280](file:///f:/aranea-agents/internal/service/chat.go#L280) 中，当 `queued=true` 但 `pendingID==""` 时使用 `apierror.BadRequest`（400），但这是服务端不变量违反（orchestrator 应始终返回 pendingID），不是客户端错误。

**修改方案**：
```go
// 修复前
return nil, apierror.BadRequest("CHAT", enqueueRejectMessage(rejectReason))
// 修复后
return nil, apierror.Internal("CHAT", "queued message missing pending id")
```
同时将未使用的 `rejectReason` 变量改为 `_` 以修复编译错误。

**逻辑分析**：
- ✅ **正确解决**：`Internal`（500）正确反映"服务端不变量违反"语义，符合红线 #14（业务错误用 apierror）
- ✅ **未引入新问题**：
  - `rejectReason` 改为 `_` 不影响逻辑（该路径下 rejectReason 本就未被使用）
  - `EnqueueUserMessage` 返回的 4 个值中第 4 个（rejectReason）仅在 `!accepted` 路径使用，`queued && pendingID==""` 路径不需要
  - 后端测试通过
- **影响域**：仅错误码语义变更，前端无需调整（错误处理为通用 catch）

---

### T-1：listTeamRuns URL 修正

**问题描述**：[web/src/services/index.ts](file:///f:/aranea-agents/web/src/services/index.ts#L210) 中 `listTeamRuns` 使用 `/v1/teams/${teamId}/runs?limit=1`，但后端 [team.proto](file:///f:/aranea-agents/api/kratos/team/v1/team.proto) 无此路由，正确端点是 `/v1/team-runs?team_id=...`。

**修改方案**：
```typescript
// 修复前
return kratosApi.get(`/v1/teams/${teamId}/runs?limit=1`);
// 修复后
return kratosApi.get(`/v1/team-runs?team_id=${encodeURIComponent(teamId)}&limit=1`);
```

**逻辑分析**：
- ✅ **正确解决**：URL 与后端 `team_run.proto` 的 `ListTeamRuns` 端点对齐，`team_id` 作为 query 参数传递
- ✅ **未引入新问题**：
  - 添加 `encodeURIComponent` 防止特殊字符注入
  - 前端测试通过（无 mock 依赖此 URL）
- **影响域**：前端 spirit service 的 team run 列表查询

---

### T-3：toProtoSpiritTeamView 字段补充

**问题描述**：[team_dead_letter.go](file:///f:/aranea-agents/internal/service/team_dead_letter.go) 中 `toProtoSpiritTeamView` 仅填充 9/20 字段，缺失 `duration_ms`/`token_in`/`token_out`/`team_session_id`/`graph_execution_id` 等运行时数据。

**修改方案**：
1. `ListSpiritTeams` 批量查询 TeamRun 数据（`ListRunsByTeamIDs`，每队取最新 1 条）
2. `toProtoSpiritTeamView` 签名改为接受可选 `*biz.TeamRun`，填充运行时字段
3. 查询失败时降级为仅返回 Team 基础字段（Warn 日志，不阻断）

**逻辑分析**：
- ✅ **正确解决**：前端 `SpiritTeamView` 卡片现在能展示运行时长、token 消耗、关联 session/execution
- ✅ **未引入新问题**：
  - 批量查询避免 N+1 问题（单次 `ListRunsByTeamIDs`）
  - 查询失败降级处理（`runsByTeam = nil`），不影响主流程
  - `run != nil` 守卫确保空值安全
  - 后端测试通过
- **影响域**：`ListSpiritTeams` API 响应体扩展，前端无需改动（字段已定义在 proto 中）

---

### T-4：TimeTravelGraph 使用 stepIndex

**问题描述**：[graph_execution_service.go:157](file:///f:/aranea-agents/internal/service/graph_execution_service.go#L157) 中 `TimeTravelGraph` 忽略 `req.StepIndex`，第一次调用 `TimeTravelGetState` 传空字符串，stepIndex 仅在错误 fallback 路径使用。

**修改方案**：重写为直接使用 step 索引从 execution steps 获取状态：
```go
exec, err := s.uc.GetExecution(ctx, req.ExecutionId)
// ...
idx := int(req.StepIndex)
if idx < 0 || idx >= len(exec.Steps) {
    return nil, biz.ErrNotFound
}
step := exec.Steps[idx]
resp := &graphv1.TimeTravelGraphResponse{
    ExecutionId: exec.ID,
    StepIndex:   int32(idx),
    NodeId:      step.NodeID,
}
if step.OutputState != nil {
    st, stErr := structpb.NewStruct(step.OutputState)
    if stErr == nil {
        resp.StateSnapshot = st
    }
}
```

**逻辑分析**：
- ✅ **正确解决**：直接按 `stepIndex` 索引 `exec.Steps`，返回对应步骤的 `OutputState` 快照
- ✅ **未引入新问题**：
  - 边界检查 `idx < 0 || idx >= len(exec.Steps)` 防止越界
  - `step.OutputState != nil` 守卫防止 nil panic
  - `structpb.NewStruct` 错误时静默跳过（与原代码行为一致）
  - 不再依赖 biz 层 `TimeTravelGetState`（其签名不接受 stepIndex），避免语义不匹配
  - 后端测试通过
- **影响域**：`TimeTravelGraph` API，前端时间旅行功能现在能正确返回指定步骤的状态

---

### T-5：RetryTeam 注释修正

**问题描述**：[team_dead_letter.go](file:///f:/aranea-agents/internal/service/team_dead_letter.go) 中 `RetryTeam` 注释暗示"重试会重启执行"，但实际仅重置状态，不重启执行。

**修改方案**：修正注释为明确"仅重置状态，不重启执行；实际执行需通过 team run lifecycle（如 StartTeamTurn）单独触发"。

**逻辑分析**：
- ✅ **正确解决**：注释准确反映实现行为
- ✅ **未引入新问题**：纯注释变更，无逻辑改动
- **影响域**：仅文档性变更

---

### S-2：getSkillHealth 字段补充

**问题描述**：前端 [skills/api.ts](file:///f:/aranea-agents/web/src/features/skills/api.ts) 读取 4 个字段（`route_hit_rate_7d`/`route_hit_rate_30d`/`routed_count`/`loaded_count`），但 proto 未定义，后端 biz/data 层已计算。

**修改方案**：
1. [skill.proto](file:///f:/aranea-agents/api/kratos/skill/v1/skill.proto) 添加 4 个字段：
   - `SkillHealthMetric.route_hit_rate_7d = 11`
   - `SkillHealthMetric.route_hit_rate_30d = 12`
   - `SkillHealthDailyMetric.routed_count = 5`
   - `SkillHealthDailyMetric.loaded_count = 6`
2. [skill.go](file:///f:/aranea-agents/internal/service/skill.go) `GetSkillHealth` 方法补充字段映射

**逻辑分析**：
- ✅ **正确解决**：proto 字段编号 11/12/5/6 不与现有字段冲突，后端映射使用 biz 层已计算的 `RouteHitRate7d`/`RouteHitRate30d`/`RoutedCount`/`LoadedCount`
- ✅ **未引入新问题**：
  - proto 字段编号使用下一个可用编号，无冲突
  - `int32(dm.RoutedCount)` 转换安全（计数值不会超过 int32 范围）
  - 后端测试通过
- **影响域**：`GetSkillHealth` API 响应体扩展，前端现在能读取路由命中率统计

---

### M-1：UpsertMemoryFact 透传 PIITypes

**问题描述**：[memory.go:854](file:///f:/aranea-agents/internal/service/memory.go#L854) 中 `UpsertMemoryFact` 未透传 `PIITypes` 字段，导致 PII 类型信息在 upsert 时丢失。

**修改方案**：
1. 添加 `PIITypes: parsePIITypesJSON(f.GetPiiTypesJson())` 字段映射
2. 新增 `parsePIITypesJSON` 辅助函数，将 JSON 数组字符串解析为 `[]string`

**逻辑分析**：
- ✅ **正确解决**：`parsePIITypesJSON` 正确处理空值/null/无效 JSON（返回 nil），有效 JSON 数组解析为 `[]string`
- ✅ **未引入新问题**：
  - `strings.TrimSpace` 处理空白输入
  - `"null"` 字符串显式返回 nil
  - `json.Unmarshal` 失败返回 nil（降级处理，不阻断）
  - 后端测试通过
- **影响域**：`UpsertMemoryFact` API，PII 类型信息现在能正确持久化

---

### P-2：UpdateEvalLLM 守卫 + 合并

**问题描述**：
1. [system_setting.go](file:///f:/aranea-agents/internal/service/system_setting.go) 中 `UpdateEvalLLM` 无守卫，即使请求未包含 eval LLM 字段也会调用，导致 proto3 零值覆盖现有配置
2. [biz/system_setting.go](file:///f:/aranea-agents/internal/biz/system_setting.go) 中 `UpdateEvalLLM` 直接覆盖，不合并当前值

**修改方案**：
1. Service 层：新增 `hasEvalLLMUpdate` 守卫函数，仅在任一 eval LLM 字段非空时调用 `UpdateEvalLLM`
2. Biz 层：先读取当前值（`u.repo.Get(ctx)`），使用 `firstNonEmpty` 合并 patch 与当前值

**逻辑分析**：
- ✅ **正确解决**：
  - Service 层守卫防止无意义的空 patch 调用
  - Biz 层合并确保空字段不会覆盖现有配置（proto3 零值问题）
  - `firstNonEmpty(strings.TrimSpace(patch.X), cur.EvalLLM.X)` 模式与 `UpdateKnowledgeEmbed`/`UpdateWebResearch` 一致
- ✅ **未引入新问题**：
  - `hasEvalLLMUpdate` 检查 4 个字段任一非空，覆盖所有 patch 场景
  - Biz 层 `Get(ctx)` 失败时返回错误，不静默继续
  - 后端测试通过
- **影响域**：`UpdateSystemSettings` API 的 eval LLM 子配置更新路径

---

### P-3：LlmProviderModel 字段守卫

**问题描述**：[llm_provider_model.go:324-330](file:///f:/aranea-agents/internal/biz/llm_provider_model.go#L324) 中 `Description` 和 `SortOrder` 无零值守卫，proto3 零值会覆盖现有值。

**修改方案**：
```go
if patch.Description != "" {
    merged.Description = patch.Description
}
merged.Enabled = patch.Enabled  // 保持无守卫，false 是合法的"禁用"语义
if patch.SortOrder != 0 {
    merged.SortOrder = patch.SortOrder
}
```

**逻辑分析**：
- ✅ **正确解决**：
  - `Description` 空字符串不覆盖（与 `Name`/`Status`/`Key` 等字符串字段模式一致）
  - `SortOrder` 零值不覆盖（与 `Dim`/`MaxResults` 等数值字段模式一致）
  - `Enabled` 保持无守卫：`false` 是合法的"禁用"语义，加守卫会导致无法禁用模型
- ✅ **未引入新问题**：
  - 守卫模式与同函数内其他字段（`Key`/`Name`/`Status`/`Provider`/`Model`）完全一致
  - `Enabled` 的特殊处理有明确语义理由（bool 零值是合法业务值）
  - 后端测试通过
- **影响域**：`UpdateProviderModel` API 的 patch 合并路径

---

## 三、关闭问题说明

### T-6：Teams/Graph 其他问题

**关闭原因**：审查报告中 T-6 描述模糊，无法在代码中定位具体的缺陷点。经多轮搜索未找到与描述匹配的代码路径。建议后续审查时提供更精确的代码位置。

### P-1：WebResearch 默认值覆盖

**关闭原因**：误报。审查报告认为前端使用 `||` 覆盖默认值，但实际代码使用 `??`（nullish coalescing），`??` 只覆盖 `null`/`undefined`，不会覆盖 `0` 或 `''`。语义正确，无需修复。

---

## 四、验证证据

### 4.1 后端验证

```
$ go build ./internal/service/... ./internal/biz/... ./internal/data/...
# 编译成功，无错误

$ go test ./internal/biz/... ./internal/service/...
# exit code 0，所有测试通过
```

### 4.2 前端验证

```
$ pnpm lint
# OK: no new hardcoded Chinese violations.

$ pnpm test --run
# Test Files  82 passed (82)
# Tests  499 passed (499)

$ pnpm build
# Build succeeded
# Output folder: F:\aranea-agents\web\dist
```

---

## 五、结论

本轮修复的 11 个阻断问题全部通过验证：

1. **正确性**：所有修复都准确解决了对应的问题，修复方案与问题根因匹配
2. **安全性**：未引入新的安全风险（无 nil panic、无越界、无注入）
3. **兼容性**：前端 lint/test/build 全部通过，后端 build/test 全部通过
4. **一致性**：修复模式与项目现有代码风格一致（守卫函数、firstNonEmpty 合并、apierror 错误码）
5. **影响域可控**：所有修改都局限在问题所在的文件/模块，无跨模块副作用

2 个关闭问题（T-6 无法确认、P-1 误报）已记录关闭原因，不影响整体修复质量。
