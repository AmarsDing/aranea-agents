# 使命驱动的任务匹配与团队配方复用 — 实施计划

> 日期：2026-07-25 | 状态：✅ 已完成
> 需求：[1-chat.md §MM](../../development/1-chat.md) | 设计：[1-chat.design.md §B.10.21](../../development/1-chat.design.md) | 进度：[1-chat.development.md §MM-D](../../development/1-chat.development.md)

## 目标

将编排匹配锚点从"单次任务文本"迁移到"使命（Mission）+ 领域路径（domain_path）+ 履历"，修复同类任务（如"写诗"）重复创建 Agent/Team 的问题。

## 根因（评审确认）

| # | 根因 | 修复 |
|---|------|------|
| R1 | `AgentFactory.buildDynamicAgentKey = sha1(domain\|taskDescription\|...)`，任务文本细微差别即生成新 Agent | key 改为域派生；创建前同域使命相似 ≥0.85 直接复用 |
| R2 | `findReusableTeam` 仅防重试，不具备能力复用 | 配方复用替代 Team 实体物理复用 |
| R3 | OrchestrationCache write-only（只写不读） | `BestRecipeForDomain` 读取 + `RecordDomainRecipe` 域键记录 |
| R4 | `extractCapabilityHints` 只认英文关键词，中文任务无匹配通道 | 领域词表 + L1 使命匹配（embedding/TF-IDF） |

## 设计要点

1. **领域词表**（`internal/agent/domain_lexicon.go`）：内置约束词表 + `NormalizeDomainPath` 归一化（词表外路径归并最近已知域），防止路径漂移
2. **四层匹配管线**（`agent_allocator_impl.go` + `agent_domain_match.go`）：
   - L0 `domain_recipe`：OrchestrationCache 同域高 DQ 配方直接组队（短路后续层）
   - L1 `mission`：同域候选 → 使命相似度 ×0.4 + 履历成功率 ×0.6 排序（阈值 0.3）
   - L2 `performance` + exact / L3 `llm_cold_start`：沿用旧管线
3. **AgentFactory**：域派生 key + definition 输出 mission/domain_path + 出生登记（mission 缺省回退 description）
4. **学习闭环**：`learnFromOrchestration` 以 `RecordDomainRecipe` 记录域配方；`AgentPerformance.TaskType` 语义扩展为 `domain:<path>`
5. **懒兼容**：存量 Agent 无使命时 `missionMatchText` 回退 Description；空 DomainPath 跳过 L0/L1 走旧管线

## 任务清单

| # | 任务 | 状态 |
|---|------|------|
| T1 | 领域词表 + 单测（TDD） | ✅ |
| T2 | 数据模型：Ent schema 加列 → DDL 迁移（20261110）→ biz 字段 → repo 映射 | ✅ |
| T3 | OrchestrationCache：DomainPath + BestRecipeForDomain + RecordDomainRecipe + 单测 | ✅ |
| T4 | TaskPlanner：prompt 约束 + 解析 domain_path（零额外 LLM 调用） | ✅ |
| T5 | Allocator 管线：L0/L1 层插入 matchSubTask/matchWholePlan | ✅ |
| T6 | AgentFactory：key 修正 + definition 解析 + 同域复用检查 | ✅ |
| T7 | 学习闭环：域配方记录 + TaskType 语义扩展 | ✅ |
| T8 | 全量验证 | ✅ |

## 验证结果（2026-07-25）

- `go build ./...`：exit 0
- `go test ./internal/agent/ ./internal/biz/ ./internal/data/ ./internal/service/`：全部通过（55.8s / 9.6s / 173.5s / 5.4s）
- `make lint`：全绿（araneactl lint 0 violations / go vet / fmtcheck / fieldguide-lint）
- 新增测试：`domain_lexicon_test.go`（归一化/相关域/主导域）、`agent_domain_match_test.go`（L0 命中/短路、L1 履历破平/低分拒配/空域跳过）、`spirit_orchestration_cache_test.go`（双向前缀匹配/DQ 只升不降/旧 JSON 兼容）、`task_planner_impl_test.go`（domain_path 解析归一化）、`agent_factory_test.go`（出生登记/同域复用）
