# Monitor Logs — Code Review & 文档同步

> 日期：2026-05-20  
> 范围：Phase 1c（流程/进程 Tab 拆分）+ 进程日志 config/UI 第二轮  
> 规范：[docs/README.md](../README.md) · [AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

## Review 结论

| 维度 | 评级 | 摘要 |
|------|------|------|
| 架构 | ✅ 良好 | 分层正确：config → `internal/server` WS 门控 → `service.GetMonitorLogs` 快照 → 前端 Hub 分流；`flow_log` 与 `log` 语义分离符合 52-flow-logger 设计 |
| 业务逻辑 | ✅ 良好 | 流程 Tab 始终收 `flow_log`；进程由 config 控制 + Tab 级暂停；legacy 重复 `EnvelopeTypeLog` 已清理 |
| 接口质量 | ✅ 良好 | `GetMonitorLogsResponse.enabled` 语义已文档化；WS `enable_log` 受 config 约束（review 修复） |
| 代码质量 | ✅ 良好 | Hub 状态机清晰；review 修复 `inject<Ref<boolean>>` 类型；新增 `ProcessLogEnabled` 单测 |
| 文档 | ✅ 已同步 | 18 monitor*、52-flow-logger*、frontend-pages、本 changelog |

## Review 中发现并已修复

1. **安全/一致性**：`ws.go` 上游 `enable_log(true)` 在 `process_log_enabled: false` 时被忽略，防止客户端绕过配置。
2. **前端类型**：`ProcessLogStream.vue` 的 `processLogConfigured` inject 改为 `Ref<boolean>`。
3. **Proto 注释**：`GetMonitorLogsResponse.enabled` 注明镜像 `server.monitor.process_log_enabled`。
4. **单测**：`internal/conf/monitor_test.go` 覆盖默认 true / 显式 false / true。

## 已知限制（接受，已写入需求文档）

| 项 | 说明 |
|----|------|
| 进程 Tab 暂停 | 丢弃入站行，不缓冲；切回 Tab 只能看到之后的新日志 |
| 带宽 | config 开启时 WS 仍推送 `log`，即使未切到进程 Tab（客户端 paused 丢弃）；后续可优化为 Tab 可见时才 `enable_log` |
| 双路径 enable | 服务端 globalMode 自动 `logEnabled` + 前端 `setProcessEnabled(true)` 冗余但无害 |

## 变更文件（review 轮）

- `internal/server/ws.go` — config 门控 `enable_log`
- `internal/conf/monitor_test.go` — 新建
- `web/src/components/monitor/ProcessLogStream.vue` — inject 类型
- `api/kratos/monitor/v1/monitor.proto` — `enabled` 注释
- 文档：`18 monitor.md`、`18 monitor.design.md`、`18-monitor-development.md`、`52-flow-logger*.md`、`frontend-pages.md`

## 验证

```bash
go test ./internal/conf/... ./internal/server/...
make api   # monitor.proto 注释变更
cd web && pnpm build
```
