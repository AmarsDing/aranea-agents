## Why

Monitor 模块当前只有**被动响应**能力（告警评估、根因分析、事件驱动自愈），缺乏**主动巡检**机制。系统中的多个子系统（FlowFileAppender、TraceProjector、AlertEvalWorker、EventBus 订阅、WebSocket 连接）可能静默退化（如磁盘满导致落盘失败、Trace 投影积压、告警评估停滞），直到用户手动发现。需要一套主动自检 + 自动修复机制，在问题影响用户前发现并修复。

## What Changes

- 新增 `SelfCheckScheduler`：定时触发自检任务（默认 5 分钟间隔），覆盖 Monitor 所有子系统健康状态
- 新增 `SelfChecker` 接口 + 6 个内置检查器：DB 连接/Schema、FlowFileAppender 磁盘与写入、TraceProjector 积压、AlertEvalWorker 存活、EventBus 订阅健康、WebSocket 连接
- 新增 `SelfCheckRepair`：自检发现问题后自动执行修复动作（重启停滞 Worker、重连断开订阅、清理孤立数据、回填缺失 Trace）
- 新增 `SelfCheckReport`：自检结果持久化 + 暴露 API，支持前端展示自检状态面板
- 自检结果集成到现有告警体系：连续自检失败触发告警
- 自检结果集成到 DiagBundle：诊断包包含自检快照

## Capabilities

### New Capabilities
- `self-check-scheduler`: 定时自检调度器，管理自检任务的生命周期（注册、调度、超时、并发控制）
- `self-check-repair`: 自检修复引擎，根据自检结果执行自动修复动作并记录修复历史

### Modified Capabilities
- `root-cause-engine`: RootCauseCondition 增加 SelfCheckStatus 维度（自检通过/失败/修复中），根因分析可参考自检结果
- `diag-bundle`: DiagBundle 增加自检快照数据

## Impact

- **Biz 层**（internal/biz/monitor）: 新增 SelfCheckScheduler、SelfChecker 接口、6 个 Checker 实现、SelfCheckRepair、SelfCheckReport 持久化
- **Data 层**（internal/data）: 新增 self_check_reports Ent Schema + Repo
- **Service 层**（internal/service）: 新增 ListSelfCheckReports / TriggerSelfCheck RPC
- **Proto**（api/kratos/monitor/v1）: 新增 2 个 RPC
- **CronRunner**（internal/cronrunner/jobs）: 新增 SelfCheckJob
- **前端**（web）: Monitor 页面新增自检状态面板
- **Wire**（cmd/admin）: 注入 SelfCheckScheduler 和相关依赖

## Non-goals

- 不替代现有告警体系（自检是补充，不是替代）
- 不做跨实例自检协调（当前 SQLite 单实例）
- 不做自检规则的用户自定义（内置检查器，后续可扩展）
- 不修改 trpc-agent-go 运行时（与 monitor-self-healing 变更互补，不重叠）
- 不做前端自检仪表盘的完整 UI（仅基础状态面板，详细 UI 后续变更）
