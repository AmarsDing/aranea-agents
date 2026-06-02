# Channel 访问控制（allowed_user_ids / allowed_group_ids）

**日期**：2026-05-22

## 问题

`config_json.config.allowed_user_ids` 与 `allowed_group_ids` 仅在前端保存，入站路径未校验，配置不生效。

## 方案

- **策略层**（`internal/biz/channel_access.go`）：解析 allowlist（JSON 数组 / 逗号字符串）与 `require_mention`；空列表不限制；`"0"` 拒绝该维度；多维度 AND。
- **执行层**（`internal/service/channel_ingress_access.go`）：`ProcessInbound` 入口调用；拒绝时 `access_denied` + 用户可见拒绝文案。
- **入站 meta**：飞书 WS/Webhook 补充 `sender_open_id`、`chat_type`、`mentioned`，供策略匹配。

## 文档

- `docs/需求/17 channel.md` §6 用法与示例
- `docs/需求/17 channel.design.md` §5.1 访问控制流程
- `docs/需求/17-channel-development.md` D2 ✅

## 验证

```bash
go test ./internal/biz/... ./internal/service/... ./internal/channel/...
```
