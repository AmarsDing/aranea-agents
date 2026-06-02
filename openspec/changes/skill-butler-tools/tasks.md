# 技能管家工具集 任务清单

## Task 1：创建 skills_butler 包骨架与 registry.go

**描述**：创建 `internal/tools/skills_butler/` 包，实现 `registry.go`（Deps 结构体、端口接口定义、RegisterAll、IsSkillsButlerAllowed）。

**DoD**：
- [ ] `internal/tools/skills_butler/registry.go` 存在且编译通过
- [ ] `Deps` 结构体包含所有端口字段（SkillUC、SkillQuery、EvolutionRepo、ToolInvReader、ProviderCatalog、RoundTrip、ProviderCode、ModelAPIID）
- [ ] 端口接口定义完整（SkillUsecasePort、SkillQueryPort、EvolutionMetricsPort、ToolInvocationPort、ProviderCatalogPort、RoundTripPort）
- [ ] `RegisterAll(deps Deps) []trpctool.Tool` 返回 4 个工具（占位实现即可）
- [ ] `IsSkillsButlerAllowed(agentKey string) bool` 仅对 `__skills__` 返回 true
- [ ] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 2：实现 analyze_skill_usage 工具

**描述**：实现 `analyze_skill_usage.go`，分析 Skill 调用频率、成功率、趋势，输出健康度报告。

**DoD**：
- [ ] `internal/tools/skills_butler/analyze_skill_usage.go` 存在且编译通过
- [ ] `AnalyzeSkillUsageInput` 和 `AnalyzeSkillUsageOutput` 结构体定义完整（含 SkillHealthReport）
- [ ] 工具通过 `trpcfunction.NewFunctionTool` 构建，名称为 `analyze_skill_usage`
- [ ] 从 `SkillQueryPort.SearchSkillInvocations` 查询调用记录
- [ ] 从 `EvolutionMetricsPort.GetToolSuccessRate` 获取成功率趋势
- [ ] 健康度判定规则正确（healthy/warning/critical/dormant）
- [ ] 趋势判定规则正确（rising/stable/declining/dormant）
- [ ] SkillID 为空时分析全部 Skill
- [ ] TimeRange 参数正确映射（7d/30d/90d）
- [ ] 无调用数据时返回空报告而非错误
- [ ] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 3：实现 recommend_skills 工具

**描述**：实现 `recommend_skills.go`，基于任务描述推荐 Skill 组合。

**DoD**：
- [ ] `internal/tools/skills_butler/recommend_skills.go` 存在且编译通过
- [ ] `RecommendSkillsInput` 和 `RecommendSkillsOutput` 结构体定义完整（含 SkillRecommendation）
- [ ] 工具通过 `trpcfunction.NewFunctionTool` 构建，名称为 `recommend_skills`
- [ ] 从 `SkillUsecasePort.List` 获取候选 Skill 列表（enabled=true）
- [ ] 调用 `SkillUsecasePort.ScoreByEmbedding` 计算相似度
- [ ] TopK 参数默认值为 5
- [ ] 按 Score 降序排列
- [ ] 为每个推荐生成 Reason（基于 tags 匹配度 + 相似度）
- [ ] 无候选 Skill 时返回空列表
- [ ] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 4：实现 evolve_skill 工具

**描述**：实现 `evolve_skill.go`，基于失败模式分析优化 Skill body，创建新版本。

**DoD**：
- [ ] `internal/tools/skills_butler/evolve_skill.go` 存在且编译通过
- [ ] `EvolveSkillInput` 和 `EvolveSkillOutput` 结构体定义完整
- [ ] 工具通过 `trpcfunction.NewFunctionTool` 构建，名称为 `evolve_skill`
- [ ] 从 `SkillUsecasePort.Get` 加载当前 Skill
- [ ] FailurePatterns 为空时自动从 `SkillQueryPort.SearchSkillInvocations` 查询失败案例
- [ ] LLM 调用使用 `provider.TRPCModelForProviderModel` 方式
- [ ] 失败分析 prompt 模板完整（见 design §4.1）
- [ ] 解析 LLM 返回的 JSON（failure_analysis、optimized_body、changes、confidence）
- [ ] JSON 解析失败时降级返回原始文本作为 analysis
- [ ] 调用 `SkillUsecasePort.Create` 创建新版本（slug 加 "_v2" 后缀）
- [ ] 调用 `SkillUsecasePort.ToggleEnabled(new_id, false)` 标记 pending review
- [ ] 生成 diff preview（当前 body vs 优化 body 的对比摘要）
- [ ] Skill 不存在时返回 BadRequest 错误
- [ ] 无失败案例时返回提示信息
- [ ] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 5：实现 optimize_skill 工具

**描述**：实现 `optimize_skill.go`，分析工具调用权重，生成调整建议。

**DoD**：
- [ ] `internal/tools/skills_butler/optimize_skill.go` 存在且编译通过
- [ ] `OptimizeSkillInput` 和 `OptimizeSkillOutput` 结构体定义完整（含 ToolWeightReport、ToolSuggestion）
- [ ] 工具通过 `trpcfunction.NewFunctionTool` 构建，名称为 `optimize_skill`
- [ ] 从 `EvolutionMetricsPort.GetToolSuccessRate` 获取整体成功率
- [ ] 从 `ToolInvocationPort.SearchToolInvocations` 获取调用明细
- [ ] WeightScore 计算公式正确（success_rate*0.5 + call_count*0.3 + 1/duration*0.2，归一化）
- [ ] Recommendation 判定规则正确（promote/demote/keep）
- [ ] LLM 调用使用 `provider.TRPCModelForProviderModel` 方式
- [ ] 建议生成 prompt 模板完整（见 design §4.2）
- [ ] 无调用数据时返回空报告
- [ ] `go build ./internal/tools/skills_butler/...` 通过

---

## Task 6：服务层注入——skillsButlerTools + 适配器

**描述**：在 `internal/service/cli_admin_tools.go` 中新增 `skillsButlerTools()` 方法和适配器类型，在 `chat_orchestrator_turn.go` 中追加注入。

**DoD**：
- [ ] `cli_admin_tools.go` 中新增 `skillsButlerSkillUC`、`skillsButlerSkillQuery`、`skillsButlerEvolutionRepo`、`skillsButlerToolInvReader` 适配器类型
- [ ] 每个适配器类型实现对应的端口接口
- [ ] `skillsButlerTools(ctx, ag)` 方法仅在 `ag.AgentKey == "__skills__"` 时返回工具列表
- [ ] `chat_orchestrator_turn.go` 中 `CustomTools` 追加 `o.skillsButlerTools(ctx, ag)`
- [ ] Deps 中 `ProviderCatalog` 和 `RoundTrip` 正确传入
- [ ] Deps 中 `ProviderCode` 和 `ModelAPIID` 从 `ag` 获取
- [ ] `go build ./internal/service/...` 通过

---

## Task 7：集成验证

**描述**：端到端验证工具注册和注入路径。

**DoD**：
- [ ] `make build` 通过
- [ ] `go test ./internal/tools/skills_butler/... -count=1` 通过（如有测试）
- [ ] `go vet ./internal/tools/skills_butler/...` 无警告
- [ ] `go vet ./internal/service/...` 无警告
- [ ] 确认 `__skills__` Agent 的 `CustomTools` 包含 4 个工具（evolve_skill、optimize_skill、recommend_skills、analyze_skill_usage）
