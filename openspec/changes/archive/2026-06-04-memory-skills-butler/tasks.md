## Non-goals (P0)

- P0 不做 LLM 预测误差调用（用语义新颖度规则替代）
- P0 不实现遗忘策略可配置（默认 hybrid）
- 不修改 trpc-agent-go 框架的 Assemble() 方法
- 不实现 Agent 能力画像自动更新（P2）
- 不做前端记忆健康仪表盘 / Skill 健康仪表盘（P2）

## 1. Schema & Interface Layer

- [ ] 1.1 在 `internal/data/ent/schema/agent_runtime_setting.go` 新增 `forget_policy_json` 字段（Default("{}")），运行 `make api` 生成 Ent 代码
- [ ] 1.2 在 `internal/data/ent/schema/agent_runtime_setting.go` 新增 `tool_weight_json` 字段（Default("{}")），运行 `make api` 生成 Ent 代码
- [ ] 1.3 在 `internal/data/ent/schema/agent_runtime_setting.go` 新增 `dream_snapshot_json` 字段（Default("")），运行 `make api` 生成 Ent 代码
- [ ] 1.4 在 `internal/biz/memory_admin_store.go` 的 `L3FactAdminStore` 接口新增 `DeleteFactRow(ctx, factID) error` 和 `DeleteFactRowsByIDs(ctx, factIDs) (int, error)` 方法
- [ ] 1.5 在 `internal/biz/memory_admin_store.go` 定义 `L3FactWriter` 子接口，嵌入 `L3FactAdminStore` 并包含 `DeleteFactRow` + `DeleteFactRowsByIDs`
- [ ] 1.6 在 `internal/biz/experience_analytics_types.go` 定义所有分析报告类型：`ToolWeightReport`, `SkillHealth`, `OrchestrationQuality`, `OrchestrationModeReport`, `MemoryQualityReport`, `ForgetConfig`, `DreamSnapshot`, `FactSnapshot`

## 2. Data Layer

- [ ] 2.1 在 `internal/data/` 实现 `L3FactWriter` 的 `DeleteFactRow` 和 `DeleteFactRowsByIDs` 方法（通过 Ent ORM 删除 memory_facts 记录 + 调用 MemoryFactIndexSyncer 清理向量索引）
- [ ] 2.2 在 `internal/data/data.go` 新增 Wire 绑定：`wire.Bind(new(biz.L3FactWriter), new(*L3FactAdminStoreImpl))`（或对应实现类型）
- [ ] 2.3 更新 `internal/biz/` 中 `AgentRuntimeSettings` 结构体，新增 `ForgetConfigJSON`、`ToolWeightJSON`、`DreamSnapshotJSON` 字段
- [ ] 2.4 更新 data 层 `AgentRuntimeSettings` 的转换逻辑（ent schema → biz struct），映射新增的 3 个 JSON 字段

## 3. Biz Layer — ExperienceAnalyticsUsecase

- [ ] 3.1 创建 `internal/biz/experience_analytics.go`，定义 `ExperienceAnalyticsUsecase` 结构体和 `NewExperienceAnalyticsUsecase` 构造函数（7 个依赖：EvolutionMetricsRepo, SkillQueryReader, TeamRepository, usage.AnalyticsRepo, MemoryAdminUsecase, SessionReader, ToolInvocationReader）
- [ ] 3.2 实现 `AnalyzeToolWeights(ctx) ([]ToolWeightReport, error)`：从 EvolutionMetricsRepo 获取整体成功率 + 从 ToolInvocationReader 获取按工具分组明细，计算 weight_score 和 recommendation
- [ ] 3.3 实现 `AnalyzeSkillHealth(ctx) ([]SkillHealth, error)`：从 SkillQueryReader.SearchSkillInvocations 查询 7 天调用记录，按 skill_id 聚合计算健康度
- [ ] 3.4 实现 `AnalyzeOrchestration(ctx, timeRange, modeFilter) ([]OrchestrationModeReport, error)`：从 TeamRepository 查询 team_runs，按 mode 分组计算 DQ score
- [ ] 3.5 实现 `AnalyzeMemoryQuality(ctx, agentID) (*MemoryQualityReport, error)`：从 MemoryAdminUsecase.ListFactRows 获取事实 + 从 EvolutionMetricsRepo 获取检索质量，计算 HealthScore
- [ ] 3.6 实现 `AnalyzeAgentCapability(ctx, agentID) (*AgentCapabilityProfile, error)`：组合工具、技能、编排、成本数据
- [ ] 3.7 在 `internal/biz/biz.go` ProviderSet 新增 `NewExperienceAnalyticsUsecase`
- [ ] 3.8 更新 `MemoryAdminUsecase` 构造函数，新增 `factWriter L3FactWriter` 参数，暴露 `DeleteFactRow` 和 `DeleteFactRowsByIDs` 代理方法

## 4. Tools — Memory Butler

- [ ] 4.1 创建 `internal/tools/memory_butler/registry.go`，定义 `Deps` 结构体（Analytics, MemoryAdmin, Embedder, EventBus）和 `RegisterAll(deps Deps) []trpctool.Tool`
- [ ] 4.2 实现 `analyze_quality.go`：`analyze_memory_quality` 工具，调用 `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality`
- [ ] 4.3 实现 `selective_remember.go`：`selective_remember` 工具，P0 语义新颖度规则（embedding + cosine > 0.85 判冗余），写入用 `MemoryAdminUsecase.UpsertFactRow`
- [ ] 4.4 实现 `forget_low_quality.go`：`forget_low_quality` 工具，检测 misaligned 记忆（检索>=3次 + 负反馈率>50%），删除用 `MemoryAdminUsecase.DeleteFactRowsByIDs`，支持 dry_run
- [ ] 4.5 实现 `forget_inactive.go`：`forget_inactive` 工具，识别超期未检索记忆，删除用 `MemoryAdminUsecase.DeleteFactRowsByIDs`，支持 dry_run
- [ ] 4.6 实现 `deduplicate.go`：`deduplicate_memories` 工具，embedding cosine > sim_threshold 的记忆合并（保留最新，删除其余）
- [ ] 4.7 实现 `consolidate_episodes.go`：`consolidate_episodes` 工具，LLM 蒸馏情景记忆为语义知识
- [ ] 4.8 实现 `dream_cycle.go`：`dream_cycle` 复合工具，编排 5 步操作 + 快照保存 + 返回报告

## 5. Tools — Skills Butler

- [ ] 5.1 创建 `internal/tools/skills_butler/registry.go`，定义 `Deps` 结构体（Analytics, SkillUC, ProviderCatalog, RoundTrip, ProviderCode, ModelAPIID, EventBus）和 `RegisterAll(deps Deps) []trpctool.Tool`
- [ ] 5.2 实现 `analyze_skill_health.go`：`analyze_skill_health` 工具，调用 `ExperienceAnalyticsUsecase.AnalyzeSkillHealth`
- [ ] 5.3 实现 `evolve_skill.go`：`evolve_skill` 工具，加载 Skill → 检索失败案例 → LLM 分析 → 创建新版本（disabled）→ 返回 diff preview
- [ ] 5.4 实现 `retire_skill.go`：`retire_skill` 工具，验证健康度 → 检查依赖 → ToggleEnabled(false) → 发布 AlertNotify 事件
- [ ] 5.5 实现 `recommend_skills.go`：`recommend_skills` 工具，调用 `SkillUsecase.ScoreByEmbedding`
- [ ] 5.6 实现 `analyze_tool_weights.go`：`analyze_tool_weights` 工具，调用 `ExperienceAnalyticsUsecase.AnalyzeToolWeights`，结果写入 `agent_runtime_settings.tool_weight_json`
- [ ] 5.7 实现 `analyze_orchestration.go`：`analyze_orchestration` 工具，调用 `ExperienceAnalyticsUsecase.AnalyzeOrchestration`
- [ ] 5.8 实现 `optimize_orchestration.go`：`optimize_orchestration` 工具，基于分析结果生成优化建议（不自动执行）

## 6. Service Layer — Tool Injection

- [ ] 6.1 在 `internal/service/system_builtin_tools.go` 新增 `memoryButlerTools(ctx, ag)` 方法，当 `ag.AgentKey == "__memory__"` 时调用 `memory_butler.RegisterAll`
- [ ] 6.2 在 `internal/service/system_builtin_tools.go` 新增 `skillsButlerTools(ctx, ag)` 方法，当 `ag.AgentKey == "__skills__"` 时调用 `skills_butler.RegisterAll`
- [ ] 6.3 在 `ChatOrchestrator` 的工具组装流程中集成 `memoryButlerTools` 和 `skillsButlerTools`
- [ ] 6.4 在 `ChatOrchestrator` 构建 `TRPCBuilderDeps` 时读取 `agent_runtime_settings.tool_weight_json`，过滤 disabled 工具 + 注入 Prompt 优先级提示

## 7. Seed Data & Prompts

- [ ] 7.1 在 `seed_system_agents.go` 新增 `__memory__` Agent 种子数据（AgentKey, DisplayName, Description, Kind, ToolsProfile, Model, ForgetPolicyJSON 默认值）
- [ ] 7.2 在 `seed_system_agents.go` 新增 `__skills__` Agent 种子数据
- [ ] 7.3 在 `seed_system_agents.go` 新增 cron task 种子数据：dream_cycle（0 3 * * *）和 skill_health_scan（0 4 * * 1）
- [ ] 7.4 创建 `internal/scenario/system/prompts/memory.md` 记忆管家 system prompt
- [ ] 7.5 创建 `internal/scenario/system/prompts/skills.md` 技能管家 system prompt

## 8. Wire & Build Integration

- [ ] 8.1 更新 `cmd/admin/wire.go` 的 `provideChatServiceDeps`，新增 `expAnalytics *biz.ExperienceAnalyticsUsecase` 参数
- [ ] 8.2 运行 `make wire` 验证 Wire 注入成功
- [ ] 8.3 运行 `make build` 验证编译通过
- [ ] 8.4 运行 `make test` 验证所有测试通过
