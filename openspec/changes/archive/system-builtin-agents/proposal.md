# System Builtin Agents

## Why

当前系统的行业 Agent/Team 是"样例"而非"壁垒"——用户仍需自己理解行业、选择 Agent、组建 Team、配置编排。产品缺少一个智能入口，让用户只需描述任务，系统自动完成从分析到执行到汇报的全流程。需要构建系统内置管家体系，作为产品的核心壁垒。

## Goals

- 构建总管家（精灵/Spirit）作为用户唯一对话入口，自动分析意图并委派给合适的管家
- 构建编排管家（Orchestrator），实现跨行业自动编排、动态组建团队、执行任务并汇报
- 扩展 Agent Kind 体系（新增 `system_builtin`），建立管家层级与协作模型
- 实现 Session 树状模型，支持编排管家创建子 Session 的层级关系
- 将现有 `__system_admin__` 迁移为 `system_builtin` 类型

## Non-goals

- P0 阶段不实现记忆管家、技能管家、监控管家的完整功能（仅骨架）
- 不修改 trpc-agent-go 框架核心代码
- 不实现编排管家的 embedding 向量搜索（P1 阶段）
- 不实现编排模板缓存与复用（P1 阶段）
