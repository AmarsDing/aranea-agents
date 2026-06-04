# Spirit Observability

## Overview
Spirit 编排可观测性体系，spirit_trace_id 贯穿三阶段，StepID 注册表统一命名，每步结构化日志。

## Requirements

### REQ-SO-01: spirit_trace_id 贯穿
- 同一用户消息的完整编排链路共享同一个 spirit_trace_id
- 从 Phase 1 到 Phase 3，所有日志、事件、持久化记录都携带此 ID
- TraceID 生成：tr_ + UUID

### REQ-SO-02: StepID 注册表
- Phase 1: spirit.planner.assess / route / memory / decompose / persist / confirm
- Phase 2: spirit.allocator.match / conflict / persist
- Phase 3: spirit.orchestrator.strategy / graph_build / graph_agent / execute / checkpoint / synthesize / learn / recover
- 所有日志必须携带：TraceID + SessionID + StepID + 业务关键字段

### REQ-SO-03: 新增 EnvelopeType
- spirit_plan_created: Phase 1 完成
- spirit_allocation_created: Phase 2 完成
- spirit_orchestration_started: Phase 3 开始执行
- spirit_orchestration_checkpoint: Checkpoint 保存
- spirit_orchestration_interrupted: 异常中断

### REQ-SO-04: 旧事件兼容
- 保留现有 6 个 Spirit EnvelopeType
- 新旧事件并行发布（双发）
- 前端双消费：handleSpiritEnvelope 同时处理新旧事件
- 2 个版本后停止双发旧事件

### REQ-SO-05: 日志规范
- 使用 pkg/loggateway.Logger，禁止 log/slog
- 每条日志必须携带：TraceID + SessionID + StepID
- 业务关键字段使用 loggateway.Str/Float64/Int 等语义化字段
- 合成结果内容写入日志（当前缺失）
