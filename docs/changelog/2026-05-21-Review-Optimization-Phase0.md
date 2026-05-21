# 2026-05-21 Review 优化（Phase 0–5）

## Phase 0 — Memory runtime boundary

- 将 `biz.RuntimeSet` 移至 `internal/runtime.MemorySet`，消除 `internal/biz` 对 `trpc-agent-go/memory` 的 import
- `make runtime-boundary` 恢复通过

## CI

- `.github/workflows/ci.yml` lint job 增加 `make runtime-boundary`
- `execution-plan.md` §系统级验收同步

## Phase 1 — 平台

- **A2A Phase 4**：`internal/a2a/health/runner.go` 网关健康 Cron + `aranea_a2a_gateway_healthy`
- **Telemetry**：per-span `otel_id`（`TraceEmitter.SyncOtelSpanIDs` + `WrapFrameworkEventsWithOtel`）
- **Knowledge OCR**：`internal/knowledge/ocr.go` + `KNOWLEDGE_OCR=stub`
- **Team summary**：`team_runs.summary_json` 持久化
- **Memory 双轨**：`12-16 memory.design.md` §1.1
- **定价 UX**：`pricingWarning.ts` + ResourceManager 横幅

## Phase 2 — 前端

- `useMonitorPage` / `usePluginsPage` / `useSystemSettingsPage` / `useEcosystemPage`
- `useChatWorkspace` 接入 `useFollowUpQueue` + `useAwaitReply`
- Monitor 30s 自动刷新（audit/events/traces tab）

## Phase 3 — 测试

- `internal/server/ws_protocol_test.go`（ping/cancel/subscribe）
- `internal/biz/team_modes_test.go`（六模式校验）
- `internal/runtime/runner_lifecycle_test.go`、`pkg/auth/token_test.go`

## Phase 4 — P2 backlog（部分）

- `session_timeline_limits.go` 自 `session_usecase.go` 抽出
- Monitor Phase 4 自动刷新（composable 层）
- Graph HITL / Plugin 沙箱 / Evolution 可视化 — 仍排入 `execution-plan` P2，未在本轮产品化

## Phase 5 — P3 占位治理

- `README-development.md`：CLI / Ecosystem / TTS 标 **技术预览**
- `docs/需求/admin-auth.md` 需求三件套
- `SUMMARY.md` 过时条目同步

## 文档

- Review / execution-plan / SUMMARY 校正（FlowLogger ✅、PendingQueue ✅、Channel 验签 ✅、CheckQuota ✅）
