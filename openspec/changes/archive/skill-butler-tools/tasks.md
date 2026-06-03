# 技能管家工具集 任务清单

## Task 1：创建 skills_butler 包骨架与 registry.go

**描述**：创建 `internal/tools/skills_butler/` 包，实现 `registry.go`（Deps 结构体、端口接口定义、RegisterAll）。

**DoD**：
- [x] `internal/tools/skills_butler/registry.go` 存在且编译通过
- [x] `Deps` 结构体包含所有端口字段（Skills、Evolution、Queries）
- [x] 端口接口定义完整（SkillUsecasePort、EvolutionUsecasePort、SkillQueryReaderPort）
- [x] `RegisterAll(deps Deps) []trpctool.Tool` 返回 4 个工具
- [x] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 2：实现 analyze_skill_usage 工具

**描述**：实现 `analyze_skill_usage.go`，分析 Skill 调用频率、成功率、健康度。

**DoD**：
- [x] `internal/tools/skills_butler/analyze_skill_usage.go` 存在且编译通过
- [x] `analyzeSkillUsageInput` 和 `analyzeSkillUsageOutput` 结构体定义完整
- [x] 工具通过 `function.NewFunctionTool` 构建，名称为 `skills_butler_analyze_skill_usage`
- [x] 从 `SkillQueryReaderPort.GetSkillInvocationStats` 查询调用记录
- [x] 健康度判定规则正确（healthy/warning/critical）
- [x] TimeRange 参数正确映射（7d/30d/90d）
- [x] 无调用数据时返回空报告而非错误
- [x] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 3：实现 recommend_skills 工具

**描述**：实现 `recommend_skills.go`，基于使用模式和进化建议推荐 Skill。

**DoD**：
- [x] `internal/tools/skills_butler/recommend_skills.go` 存在且编译通过
- [x] `recommendSkillsInput` 和 `recommendSkillsOutput` 结构体定义完整
- [x] 工具通过 `function.NewFunctionTool` 构建，名称为 `skills_butler_recommend_skills`
- [x] 从 `SkillUsecasePort.ListProposals` 获取 pending 提案
- [x] 从 `SkillQueryReaderPort.GetSkillInvocationStats` 获取使用统计
- [x] 基于健康度生成推荐（warning→优化建议，critical→移除建议）
- [x] 无候选时返回空列表
- [x] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 4：实现 evolve_skill 工具

**描述**：实现 `evolve_skill.go`，创建 Skill 进化提议。

**DoD**：
- [x] `internal/tools/skills_butler/evolve_skill.go` 存在且编译通过
- [x] `evolveSkillInput` 和 `evolveSkillOutput` 结构体定义完整
- [x] 工具通过 `function.NewFunctionTool` 构建，名称为 `skills_butler_evolve_skill`
- [x] 调用 `SkillUsecasePort.CreateProposal` 创建提案
- [x] PatternHash 自动生成
- [x] 参数校验使用 kerrors
- [x] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 5：实现 optimize_skill 工具

**描述**：实现 `optimize_skill.go`，基于使用统计生成优化建议。

**DoD**：
- [x] `internal/tools/skills_butler/optimize_skill.go` 存在且编译通过
- [x] `optimizeSkillInput` 和 `optimizeSkillOutput` 结构体定义完整
- [x] 工具通过 `function.NewFunctionTool` 构建，名称为 `skills_butler_optimize_skill`
- [x] 从 `SkillQueryReaderPort.GetSkillInvocationStats` 获取调用统计
- [x] 健康度评估和优化建议生成
- [x] 无调用数据时返回 unknown 健康度
- [x] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 6：服务层注入——skillsButlerTools + 适配器

**描述**：在 `internal/service/cli_admin_tools.go` 中新增 `skillsButlerTools()` 方法和适配器类型，在 `chat_orchestrator_turn.go` 中追加注入。

**DoD**：
- [x] `skills_butler_adapter.go` 中新增 `skillsButlerSkillUsecaseAdapter`、`skillsButlerEvolutionAdapter`、`skillsButlerQueryAdapter` 适配器类型
- [x] 每个适配器类型实现对应的端口接口
- [x] `skillsButlerTools(ctx, ag)` 方法仅在 Agent 启用 `EvolutionSkillEvolve` 时返回工具列表
- [x] `chat_orchestrator_turn.go` 中 `CustomTools` 追加 `o.skillsButlerTools(ctx, ag)`
- [x] `ChatOrchestratorDeps` 新增 `SkillEvo`、`Evolution`、`SkillStats` 字段
- [x] `go build ./internal/service/...` 通过

---

## Task 7：集成验证

**描述**：端到端验证工具注册和注入路径。

**DoD**：
- [x] `go build ./...` 通过
- [x] `go vet ./internal/tools/skills_butler/...` 无警告
- [x] `go vet ./internal/data/...` 无警告
- [x] `make wire` 通过
- [x] `make wire-clean` 通过
- [x] `go test ./internal/biz/... -count=1` 通过
- [x] 确认启用 `EvolutionSkillEvolve` 的 Agent 的 `CustomTools` 包含 4 个工具（代码路径验证通过，需运行时验证）

---

## 补充：Data 层和 Wire 注入补全

**描述**：补全 data 层缺失的 Repo 和 Wire 注入链。

**DoD**：
- [x] `internal/biz/skill_invocation_stats.go` 定义 `SkillInvocationStatsReader` 接口和 `SkillInvocationStat` 类型
- [x] `internal/data/skill_invocation_stats.go` 实现 `skillInvocationStatsRepo`
- [x] `NewSkillProposalRepo` 加入 `data.ProviderSet`
- [x] `NewSkillInvocationStatsRepo` 加入 `data.ProviderSet`
- [x] `skill_proposals` DDL 注册到迁移框架（版本 20260706）
- [x] `NewSkillEvolutionUsecase` 在 `biz.ProviderSet` 中
- [x] `provideSkillAutoCreator` 和 `provideSkillRegistrationPort` 在 `wire.go` 中
- [x] `wire.Bind(new(biz.PatternReader), new(biz.PatternReadWriter))` 在 `wire.go` 中
- [x] `NewSkillsButlerRegistrationAdapter` 在 service 层
