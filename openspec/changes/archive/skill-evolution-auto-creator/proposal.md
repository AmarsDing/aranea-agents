# Skill Evolution Auto Creator

## Why

当前项目 `internal/skill/` 已有导入器、渲染器、存储、执行器，但缺少"运行时自动检测重复模式并提议创建新 Skill"的能力。`EvolutionCfg.SkillEvolve` 字段已预留但未实现。Agent 在运行时反复以相似参数调用同一工具的模式无法被自动识别和沉淀为可复用的 Skill，导致重复劳动和知识浪费。需要实现从"检测重复模式 → LLM 生成 SKILL.md → 人工审批 → 注册到 Skill 仓库"的完整闭环。

## Goals

- 实现 Agent 运行时检测重复模式并自动提议创建新 Skill 的完整流程（F1~F5）
- 复用框架 `skill.Repository` 和 `tool/skill.*` 工具链，确保新创建的 Skill 可被现有工具加载和执行
- 支持人工审批环节，防止低质量 Skill 污染仓库（NF1 安全性）
- 与学习闭环模块协作：学习闭环识别模式 → 技能自创建将模式转化为 Skill（F6）
- 生成的 SKILL.md 必须通过框架 `skill.Repository.Get()` 的解析验证（NF2 质量）
- 同一模式不重复创建提议，基于 pattern hash 去重（NF4 幂等性）

## Non-goals

- P0 阶段不实现工具缺口即时检测（BabyAGI 启发的增强方向，P2 阶段）
- P0 阶段不实现 Skill 依赖声明和依赖图可视化（P2 阶段）
- P0 阶段不实现 Skill 提议 WebSocket 推送通知（F7，P2 阶段）
- 不修改 trpc-agent-go 框架核心代码
- 不实现自动注册——所有 Skill 提议必须经过人工审批
