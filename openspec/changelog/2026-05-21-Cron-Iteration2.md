# Cron 迭代 2 — 文档同步 + 手动触发 + 重试配置

**日期**：2026-05-21  
**模块**：Cron (21)

## 摘要

- 对照代码更新 `21-cron-development.md`、`21 cron.design.md`（RunCronTurn、1m 轮询、Wire 入口）。
- 修复 `retry_max_attempts` 语义：未设置默认 3 次重试（30s/2m/10m）；`0` 禁用重试。
- 新增 `POST /v1/cron-tasks/{id}/trigger` → `Runner.TriggerTask`（`output_json.trigger=manual`）。
- 前端：表单「失败重试次数」、列表「立即执行」按钮；dead 任务重置按钮已存在。

## 验证

```bash
make api && make wire && go test ./internal/cronrunner/... ./internal/service/...
cd web && pnpm test --run src/features/cron
```
