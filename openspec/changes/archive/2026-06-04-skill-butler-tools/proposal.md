# 技能管家工具集（Skill Butler Tools）

## Why

在 `memory-skills-butler` 变更提案中，技能管家（`__skills__`）被定义为系统内置管家，负责 Skill 进化/消亡、工具权重优化和编排分析。然而该提案覆盖范围极广（经验分析引擎 + 记忆管家 + 技能管家 + 7 篇论文 15 条原则），当前需要聚焦于**技能管家的核心工具实现**——让 Agent 能通过工具调用主动进化自身技能。

当前系统已有 `EvolutionUsecase`（进化指标查询 + 建议应用），但缺少：
- **技能进化工具**：Agent 无法基于失败模式主动优化 Skill body
- **技能优化工具**：Agent 无法分析工具权重并生成调整建议
- **技能推荐工具**：Agent 无法基于任务描述推荐 Skill 组合
- **使用分析工具**：Agent 无法查看 Skill 调用频率、成功率、趋势

这些工具是技能管家从"被动展示数据"升级为"主动进化技能"的关键能力。

## Goals

- 实现 4 个技能管家核心工具：`evolve_skill`、`optimize_skill`、`recommend_skills`、`analyze_skill_usage`
- 工具通过 `function.NewFunctionTool[I, O]` 构建，注册到 `internal/tools/skills_butler/` 包
- 工具通过 `ChatOrchestrator.skillsButlerTools()` 注入到 `__skills__` Agent
- 复用现有 `EvolutionUsecase`、`SkillUsecase`（`skill.Usecase`）、`SkillQueryReader` 等接口
- `evolve_skill` 和 `optimize_skill` 需调用 LLM 分析失败模式和生成优化方案

## Non-goals

- 不实现 `ExperienceAnalyticsUsecase`（属于 `memory-skills-butler` 变更范围）
- 不实现记忆管家工具（属于 `memory-skills-butler` 变更范围）
- 不实现 `retire_skill`、`analyze_orchestration`、`optimize_orchestration` 工具（P1 阶段）
- 不修改 `tools.Assemble()` 排序逻辑（工具权重应用方式见 `memory-skills-butler` design §11.4）
- 不实现编排拓扑缓存与复用（P1 阶段）
- 不实现前端 Skill 健康仪表盘（P2 阶段）

## Scope

### 工具清单

| 工具名 | 功能 | 核心依赖 |
|--------|------|----------|
| `evolve_skill` | 基于失败模式分析优化 Skill body，创建新版本 | `SkillUsecase` + LLM + `SkillQueryReader` |
| `optimize_skill` | 分析工具权重并生成调整建议 | `EvolutionMetricsRepo` + `ToolInvocationReader` + LLM |
| `recommend_skills` | 基于任务描述推荐 Skill 组合 | `SkillUsecase.ScoreByEmbedding` |
| `analyze_skill_usage` | 分析 Skill 调用频率、成功率、趋势 | `SkillQueryReader.SearchSkillInvocations` + `EvolutionMetricsRepo` |

### 依赖关系

```
skill-butler-tools（本变更）
  ├── 依赖 memory-skills-butler design 中的类型定义（SkillHealth、ToolWeightReport 等）
  ├── 依赖现有 EvolutionUsecase（进化指标查询）
  ├── 依赖现有 SkillUsecase（Skill CRUD + ScoreByEmbedding）
  ├── 依赖现有 SkillQueryReader（Skill 调用记录查询）
  └── 依赖现有 EvolutionMetricsRepo（工具成功率、检索质量）
```

## Success Criteria

- 4 个工具均可在 `__skills__` Agent 会话中通过工具调用执行
- `evolve_skill` 能基于失败案例生成优化后的 Skill body，创建新版本并标记为 pending review
- `optimize_skill` 能输出工具权重报告和调整建议
- `recommend_skills` 能基于任务描述返回按相似度排序的 Skill 列表
- `analyze_skill_usage` 能返回 Skill 健康度报告（调用频率、成功率、趋势）
- 所有工具通过 `make build` 和 `make test`
