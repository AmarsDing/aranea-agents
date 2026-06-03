# Session Status Monitoring

## Why

当前系统缺少对 Session 生命周期的实时监控能力：Session 状态变更无法被前端及时感知，长时间运行的 Session 无法追踪进度，异常中断的 Session 缺少告警机制。用户和系统管家都无法获取 Session 的实时状态，影响用户体验和系统可靠性。

## Goals

- 实现 Session 状态变更的实时事件推送（通过 WebSocket）
- 支持 Session 生命周期各阶段的状态监控（创建/运行/空闲/完成/失败）
- 为系统监控管家（`__monitor__`）提供 Session 状态查询能力
- 支持长时间运行 Session 的进度追踪和超时告警

## Non-goals

- 不改变 Session 的数据库 Schema 核心结构
- 不实现 Session 历史状态的时间序列存储
- 不涉及 Session 级别的资源使用配额限制
