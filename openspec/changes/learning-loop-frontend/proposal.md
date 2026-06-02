# 学习闭环前端可视化

## Why

后端已实现 `LearningLoopUsecase`（Observation → Pattern → Proposal → Validation → Registration 完整闭环），包含 `LearningLoopService`（6 个 HTTP/gRPC 端点）和完整的 Proto 定义（`api/kratos/learning_loop/v1/learning_loop.proto`）。但前端完全没有对应的可视化界面——用户无法查看 Agent 的学习状态、已识别的模式、待审批的提议、已注册的知识。当前 AgentEvolutionPanel 仅展示进化指标和建议列表，缺少学习闭环的完整视图。

## Goals

- 在 Agent 详情页新增"学习闭环"Tab，展示闭环运行状态和各阶段数据
- 展示已识别模式列表（Pattern），支持按状态筛选（detected/confirmed/dismissed）
- 提供待审批提议管理界面（Proposal），支持审批/拒绝操作
- 展示已注册知识查看（applied 状态的 Proposal）
- 展示观察记录列表（Observation），了解原始行为数据
- 支持手动触发闭环运行（RunLoop）
- 遵循 aranea-frontend-guide 数据流铁律：API → Store → Composable → Page → Component

## Non-goals

- 不修改后端 API（Proto 和 Service 层已完成）
- 不实现学习闭环的 WebSocket 实时推送（P2 阶段）
- 不实现模式详情的 LLM 解释展示（P2 阶段）
- 不实现批量审批/拒绝操作（P2 阶段）
- 不修改 AgentEvolutionPanel 的现有功能
