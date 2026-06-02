# Callback 模块 Review 跟进（P0–P2）

**日期**：2026-05-21

## P0

- `StatsRecorder` 增加 `RecordEvent`；`recordHookAudit` 经接口写入，移除 `*RepoStatsRecorder` 类型断言。
- `HookNotifier.EnqueueNotify`：`hook_deliveries` Insert 与投递在 `safego` 中异步执行。

## P1

- `Manager.MergeChain`：Chain 镜像插件条目经 `wrapResilientHooks` 包装。
- Hook `notify`：入队成功指标 `queued`；投递耗尽重试记 `delivery_failed`。
- `chainAllowlistBuiltinKeys`：内置插件 Chain 白名单（默认 `skill_usage_tracker`）；设计 §4.8 文档化。

## P2

- API：`ListHookDeliveries`（`GET /v1/hooks/deliveries`）。
- 前端：`/hooks/deliveries` 投递队列页。
- SSRF：`pkg/webhookurl.ValidateNotifyURL`（Hook notify 与投递前校验）。

## 验证

```bash
go test ./internal/plugin/trpc/... ./pkg/webhookurl/...
make api
```
