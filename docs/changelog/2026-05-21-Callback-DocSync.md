# Callback 文档同步 + 产品链 SRP

**日期**：2026-05-21  
**模块**：Callback (28)

## 摘要

- 三份 Callback 文档与代码对齐：Phase 1–3 已落地；四层编排（Runner / Chain / ModelSelector / Hook）写入 design。
- 产品链 Agent/Model 指标遥测拆至 `product_chain_builtins.go`，`callback_chain.go` 专注装配。
- 更新 `callbacks.PluginCallback` 注释，消除过时 S4 占位描述。

## 文档

- `docs/需求/28 callback.md`
- `docs/需求/28 callback.design.md`
- `docs/需求/28-callback-development.md`

## 代码

| 文件 | 变更 |
|------|------|
| `internal/agent/product_chain_builtins.go` | 新增：生命周期 Prometheus 钩子 |
| `internal/agent/callback_chain.go` | 装配职责收敛 |
| `internal/agent/callbacks/callbacks.go` | PluginCallback 注释 |

## 验证

```bash
go test ./internal/agent/... ./internal/plugin/trpc/... -count=1
```
