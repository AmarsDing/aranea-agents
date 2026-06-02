# Callback Review 跟进 — P0–P2 二次优化

**日期**：2026-05-21

## P0

- `EnqueueNotify` 返回 `error`；`executeHookAction` 入队失败记 `error` 而非假 `queued`。
- Hook `notify` 校验集中在 `EnqueueNotify`（去掉重复 URL 校验）。

## P1

- `HookDeliveryUsecase` + `HookService.ListHookDeliveries` 使用 `PageToLimitOffset`。
- `ValidateHookConfigForSave`：Hook CRUD 期校验 notify `webhook_url`（`pkg/webhookurl`）。
- `recordEvent`：Hook 审计路径跳过重复 `PluginInvokeTotal`（defer 已计数）。
- Chain 镜像插件不再 `wrapResilientHooks`。

## P2

- Gateway：`validateWebhookConfig` / `WebhookDispatcher.postOne` 复用 `webhookurl`。
- `webhookurl.NewOutboundHTTPClient`：禁止重定向、统一出站超时。
- `frontend-pages.md` 登记 `/hooks/deliveries`。

## 验证

```bash
go test ./internal/biz/... -run 'Hook|Webhook' -count=1
go test ./pkg/webhookurl/... -count=1
```
