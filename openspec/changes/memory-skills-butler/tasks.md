# Memory & Skills Butler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建经验分析引擎 + 记忆管家 + 技能管家，为系统内置管家提供数据驱动的决策能力。所有设计原则可追溯到学术论文。

**Architecture:** 经验分析引擎（biz 层 Usecase）→ 记忆管家工具集（memory_butler）+ 技能管家工具集（skills_butler）→ Service 层注入 → 种子数据 + Cron 定时任务。

**Tech Stack:** Go, trpc-agent-go (function tool), Ent ORM, Wire DI, safego, loggateway

**Design Spec:** `design.md`

---

## File Structure

### New Files

| 操作 | 文件 | 职责 | 状态 |
|------|------|------|------|
| Create | `internal/biz/experience_analytics.go` | 经验分析引擎 Usecase | ✅ 已完成 |
| Create | `internal/biz/experience_analytics_types.go` | 分析报告类型定义 | ✅ 已完成 |
| Create | `internal/tools/memory_butler/registry.go` | 记忆管家工具注册 + Deps | ✅ 已完成 |
| Create | `internal/tools/memory_butler/errors.go` | 错误常量定义 | ✅ 已完成 |
| Create | `internal/tools/memory_butler/analyze_quality.go` | analyze_memory_quality | ✅ 已完成 |
| Create | `internal/tools/memory_butler/selective_remember.go` | selective_remember | ✅ 已完成 |
| Create | `internal/tools/memory_butler/forget_low_quality.go` | forget_low_quality | ✅ 已完成 |
| Create | `internal/tools/memory_butler/forget_inactive.go` | forget_inactive | ✅ 已完成 |
| Create | `internal/tools/memory_butler/deduplicate.go` | deduplicate_memories | ✅ 已完成 |
| Create | `internal/tools/memory_butler/consolidate_episodes.go` | consolidate_episodes | ✅ 已完成 |
| Create | `internal/tools/memory_butler/dream_cycle.go` | dream_cycle（复合操作） | ✅ 已完成 |
| Create | `internal/tools/skills_butler/registry.go` | 技能管家工具注册 + Deps + Port 接口 | ✅ 已完成 |
| Create | `internal/tools/skills_butler/errors.go` | 错误常量定义 | ✅ 已完成 |
| Create | `internal/tools/skills_butler/analyze_skill_health.go` | analyze_skill_health | ✅ 已完成 |
| Create | `internal/tools/skills_butler/analyze_skill_usage.go` | analyze_skill_usage | ✅ 已完成 |
| Create | `internal/tools/skills_butler/evolve_skill.go` | evolve_skill | ✅ 已完成 |
| Create | `internal/tools/skills_butler/recommend_skills.go` | recommend_skills | ✅ 已完成 |
| Create | `internal/tools/skills_butler/optimize_skill.go` | optimize_skill（替代 retire_skill） | ✅ 已完成 |
| Create | `internal/tools/skills_butler/analyze_tool_weights.go` | analyze_tool_weights | ✅ 已完成 |
| Create | `internal/tools/skills_butler/analyze_orchestration.go` | analyze_orchestration | ✅ 已完成 |
| Create | `internal/tools/skills_butler/optimize_orchestration.go` | optimize_orchestration | ✅ 已完成 |
| Create | `internal/service/skills_butler_adapter.go` | Analytics 端口适配器 | ✅ 已完成 |
| Create | `internal/scenario/system/prompts/memory/memory.md` | 记忆管家 system prompt | ✅ 已完成 |
| Create | `internal/scenario/system/prompts/skills/skills.md` | 技能管家 system prompt | ✅ 已完成 |

### Modified Files

| 操作 | 文件 | 变更 | 状态 |
|------|------|------|------|
| Modify | `internal/biz/biz.go` | Wire ProviderSet 新增 NewExperienceAnalyticsUsecase | ✅ 已完成 |
| Modify | `internal/service/cli_admin_tools.go` | 新增 memoryButlerTools + skillsButlerTools | ✅ 已完成 |
| Modify | `internal/service/chat_orchestrator_turn.go` | 注入管家工具 | ✅ 已完成 |
| Modify | `internal/data/seed_system_admin.go` | 新增 SeedMemoryAgent + SeedSkillsAgent + SeedButlerPromptFiles + SeedCronTasks | ✅ 已完成 |
| Modify | `cmd/admin/wire.go` | 新增 ExperienceAnalyticsUsecase 注入链 | ✅ 已完成 |

---

### Task 1: ExperienceAnalyticsUsecase 核心分析能力

**Files:**
- Create: `internal/biz/experience_analytics.go`
- Create: `internal/biz/experience_analytics_types.go`
- Modify: `internal/biz/biz.go`

**DoD:**
- 5 个分析方法实现：AnalyzeToolWeights / AnalyzeSkillHealth / AnalyzeOrchestration / AnalyzeMemoryQuality / AnalyzeAgentCapability
- 结果类型定义完整
- Wire ProviderSet 注册
- `go build ./internal/biz/...` 通过

- [x] **Step 1: 创建 experience_analytics_types.go**

定义所有结果类型：ToolWeightReport, SkillHealth, OrchestrationQuality, OrchestrationModeReport, MemoryQualityReport, AgentCapabilityProfile, ForgetConfig, DreamSnapshot, FactSnapshot。

- [x] **Step 2: 创建 experience_analytics.go**

实现 ExperienceAnalyticsUsecase，包含 7 个依赖注入字段（metricsRepo, skillRepo, teamRunRepo, usageRepo, memoryAdmin, sessionRepo, toolInvData, lg）和 5 个分析方法。

- [x] **Step 3: 在 biz.go 中注册 Wire ProviderSet**

新增 `NewExperienceAnalyticsUsecase` 到 ProviderSet。

- [x] **Step 4: 验证编译通过**

Run: `go build ./internal/biz/...`
Expected: PASS

---

### Task 2: 记忆管家工具集

**Files:**
- Create: `internal/tools/memory_butler/registry.go`
- Create: `internal/tools/memory_butler/errors.go`
- Create: `internal/tools/memory_butler/analyze_quality.go`
- Create: `internal/tools/memory_butler/selective_remember.go`
- Create: `internal/tools/memory_butler/forget_low_quality.go`
- Create: `internal/tools/memory_butler/forget_inactive.go`
- Create: `internal/tools/memory_butler/deduplicate.go`
- Create: `internal/tools/memory_butler/consolidate_episodes.go`
- Create: `internal/tools/memory_butler/dream_cycle.go`

**DoD:**
- 7 个工具实现：analyze_quality / selective_remember / forget_low_quality / forget_inactive / deduplicate / consolidate_episodes / dream_cycle
- RegisterAll() 返回所有工具
- Deps 结构体定义完整
- `go build ./internal/tools/memory_butler/...` 通过

> **实现偏差**：
> - `selective_remember`：P0 使用子串匹配（非 embedding 余弦相似度），TODO 标注 P1 升级
> - `deduplicate_memories`：P0 使用 trigram 相似度（非 embedding），TODO 标注 P1 升级
> - `consolidate_episodes`：P0 使用去重拼接（非 LLM 蒸馏），TODO 标注 P1 升级
> - `dream_cycle`：8 步流程（非设计文档的 6 步），增加快照保存和前后质量测量

- [x] **Step 1: 创建 registry.go + errors.go**

定义 Deps 结构体（Analytics, MemoryAdmin, Embedder, EventBus, Agents）和错误常量。

- [x] **Step 2: 创建 7 个工具文件**

每个工具使用 `function.NewFunctionTool[I, O]` 泛型构建。

- [x] **Step 3: 验证编译通过**

Run: `go build ./internal/tools/memory_butler/...`
Expected: PASS

---

### Task 3: 技能管家工具集

**Files:**
- Create: `internal/tools/skills_butler/registry.go`
- Create: `internal/tools/skills_butler/errors.go`
- Create: `internal/tools/skills_butler/analyze_skill_health.go`
- Create: `internal/tools/skills_butler/analyze_skill_usage.go`
- Create: `internal/tools/skills_butler/evolve_skill.go`
- Create: `internal/tools/skills_butler/recommend_skills.go`
- Create: `internal/tools/skills_butler/optimize_skill.go`
- Create: `internal/tools/skills_butler/analyze_tool_weights.go`
- Create: `internal/tools/skills_butler/analyze_orchestration.go`
- Create: `internal/tools/skills_butler/optimize_orchestration.go`

**DoD:**
- 8 个工具实现（含 2 个额外工具）
- RegisterAll() 返回所有工具，Analytics 工具条件注入
- Deps 使用端口接口（Port）模式
- `go build ./internal/tools/skills_butler/...` 通过

> **实现偏差**：
> - `retire_skill` 未独立实现，功能由 `optimize_skill` 的 critical 健康状态建议覆盖
> - 额外新增 `analyze_skill_usage` 和 `optimize_skill` 工具
> - Deps 使用端口接口（SkillUsecasePort, EvolutionUsecasePort, SkillQueryReaderPort, AnalyticsPort）而非直接依赖 biz 层

- [x] **Step 1: 创建 registry.go + errors.go + 端口接口**

定义 Deps 结构体和 4 个端口接口。

- [x] **Step 2: 创建 8 个工具文件**

- [x] **Step 3: 验证编译通过**

Run: `go build ./internal/tools/skills_butler/...`
Expected: PASS

---

### Task 4: Service 层集成

**Files:**
- Create: `internal/service/skills_butler_adapter.go`
- Modify: `internal/service/cli_admin_tools.go`
- Modify: `internal/service/chat_orchestrator_turn.go`

**DoD:**
- skills_butler_adapter.go 桥接 ExperienceAnalyticsUsecase → AnalyticsPort
- cli_admin_tools.go 新增 memoryButlerTools + skillsButlerTools
- chat_orchestrator_turn.go 注入管家工具
- `go build ./internal/service/...` 通过

- [x] **Step 1: 创建 skills_butler_adapter.go**

适配器将 `*biz.ExperienceAnalyticsUsecase` 桥接到 `skills_butler.AnalyticsPort`。

- [x] **Step 2: 修改 cli_admin_tools.go**

新增 `memoryButlerTools()` 和 `skillsButlerTools()` 方法。

- [x] **Step 3: 修改 chat_orchestrator_turn.go**

在 turn 处理中注入管家工具。

- [x] **Step 4: 验证编译通过**

Run: `go build ./internal/service/...`
Expected: PASS

---

### Task 5: 种子数据 + Prompt 文件

**Files:**
- Create: `internal/scenario/system/prompts/memory/memory.md`
- Create: `internal/scenario/system/prompts/skills/skills.md`
- Modify: `internal/data/seed_system_admin.go`

**DoD:**
- 记忆管家 system prompt 包含核心原则 + 工作流程 + 7 个工具使用指南
- 技能管家 system prompt 包含核心原则 + 工作流程 + 8 个工具使用指南
- SeedMemoryAgent + SeedSkillsAgent 种子数据
- SeedButlerPromptFiles 加载 prompt 文件
- SeedCronTasks 注册定时任务

- [x] **Step 1: 创建 prompt 文件**

- [x] **Step 2: 修改 seed_system_admin.go**

新增 SeedMemoryAgent, SeedSkillsAgent, SeedButlerPromptFiles, SeedCronTasks。

- [x] **Step 3: 验证编译通过**

Run: `go build ./internal/data/...`
Expected: PASS

---

### Task 6: Wire 注入

**Files:**
- Modify: `cmd/admin/wire.go`

**DoD:**
- ExperienceAnalyticsUsecase 完整注入链
- skills_butler_adapter 正确桥接
- `make wire && go build ./cmd/admin` 通过

- [x] **Step 1: 更新 wire.go**

新增 ExperienceAnalyticsUsecase 和 skills_butler_adapter 的注入。

- [x] **Step 2: 验证 wire + build**

Run: `make wire && go build ./cmd/admin`
Expected: PASS

---

### Task 7: 全量验证

**DoD:**
- `go test ./internal/biz/... -count=1` 通过
- `go test ./internal/tools/memory_butler/... -count=1` 通过
- `go test ./internal/tools/skills_butler/... -count=1` 通过
- `make api && make wire && make build && make test && make lint` 通过

- [ ] **Step 1: 运行 biz 层测试**

Run: `go test ./internal/biz/... -count=1`
Expected: PASS

- [ ] **Step 2: 运行工具层测试**

Run: `go test ./internal/tools/memory_butler/... ./internal/tools/skills_butler/... -count=1`
Expected: PASS

- [ ] **Step 3: 运行全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: PASS

---

## 实现偏差汇总

| 设计文档 | 实际实现 | 原因 |
|----------|----------|------|
| `retire_skill.go` 独立文件 | 未独立实现，功能由 `optimize_skill.go` 覆盖 | 退役是优化的特例，合并更内聚 |
| 直接依赖 `*biz.ExperienceAnalyticsUsecase` | 使用端口接口（Port）+ 适配器模式 | 解耦工具层与 biz 层，便于测试 |
| Prompt 路径 `prompts/memory.md` | `prompts/memory/memory.md` | 按管家分目录，更清晰 |
| 注入路径 `system_builtin_tools.go` | `cli_admin_tools.go` | 与项目现有工具注入模式一致 |
| `selective_remember` 使用 embedding | P0 使用子串匹配 | 节省 Token，P1 阶段升级 |
| `deduplicate_memories` 使用 embedding | P0 使用 trigram 相似度 | 无需 embedding 服务依赖 |
| `consolidate_episodes` 使用 LLM 蒸馏 | P0 使用去重拼接 | 节省 Token，P1 阶段升级 |
| `dream_cycle` 6 步流程 | 8 步流程（增加快照和前后质量测量） | 更完善的实现，支持回滚 |
