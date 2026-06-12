# ADR-001: ChatOrchestrator 拆分为子管理器 + 通用状态机 + 事件可靠性分级

> **完整内容见**: [`docs/reports/2026-06-11-review-adr-architecture-refactoring.md`](../reports/2026-06-11-review-adr-architecture-refactoring.md)

## 状态：已接受

## 背景

ChatOrchestrator 是 service 层的核心编排器，承担了会话运行生命周期、等待/恢复协调、运行状态追踪、待处理队列管理、Agent 依赖构建等职责。随着业务增长，该 struct 出现上帝对象、EventBus 无持久化、缺少显式状态机、架构不变量无自动验证等问题。

## 决策

详见完整文档。

## 后果

详见完整文档。
