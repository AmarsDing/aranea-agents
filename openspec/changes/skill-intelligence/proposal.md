# Skill Intelligence 子系统

## Why

当前 Skill 管理已具备完整的 CRUD、导入炼化、运行记录和运行时路由能力，但存在四条核心缺口：

1. **选不准**：路由主要靠 taxonomy + 关键词匹配，无历史成功率加权，同类任务有时命中错误 Skill
2. **坏不知**：失败调用有记录但无结构化根因与趋势，运维需人工翻日志才能发现退化
3. **优不动**：炼化仅在上传冲突时触发，运行期问题无法反哺 Skill 正文，Skill 池静态膨胀
4. **版本难管**：版本历史 / 回滚 API 未实现，无法安全试验改进版

Skill 管理的下一阶段不是再做一个「会写 Skill 的 Agent」，而是先把「调用可观测 → 问题可诊断 → 变更可审计」做扎实，在此之上用半自动推荐与人工审批逐步开放进化能力。

## Goals

- **P0 可观测**：每次 Skill 调用能回答「为什么选它、结果如何、耗时/成本多少」（SI-01~SI-04）
- **P1 可诊断**：对失败/低效调用自动生成 Experience Report，供人决策而非直接改生产 Skill（SI-10~SI-15）
- **P2 可选得准**：在现有 skillrouter 候选集内引入轻量推荐排序（成功率、耗时、语义匹配），不扩大候选范围（SI-20~SI-24）
- **P3 可进化**：在 Sandbox + 人工审批门控下，支持从报告生成 Skill 草案并 A/B 对比（SI-30~SI-36）
- 复用 `evolution_scanner` 的 Worker 骨架与 `evolution_suggestions` 队列 UX 模式
- 与 `skill-evolution-auto-creator` 变更互补：auto-creator 管从零创建新 Skill，intelligence 管已有 Skill 的诊断与优化

## Non-goals

- 不做生产环境双 Skill 并行对决、直接替换用户可见结果（成本翻倍、结果不一致）
- 不做无审批自动发布进化版 Skill（安全风险与回归不可控）
- 不做 Skill-Agent 替代 Manage-Agent 做任务规划（职责重叠）
- 不做实时毫秒级进化（与对话热路径解耦，一律异步）
- 不做 Skill-Agent 常驻 goroutine 自主改库（拆为 Worker + 按需 Curator）
- Phase 4 之前不做 Shadow 模式默认开启
- 不修改 trpc-agent-go 框架核心代码
