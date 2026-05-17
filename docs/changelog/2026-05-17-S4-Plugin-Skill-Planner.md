# 2026-05-17 Sprint 4 — Plugin / Skill / Planner / Memory Tools

## Overview

Sprint 4 (T29–T33) delivers Plugin runtime wiring, Skill DB repository, Planner multi-strategy support, Memory tool injection, and the AutoMemory background job, completing milestone items M-1 through M-5.

---

## T29 — Plugin Runtime接入 (M-4)

**New files**
- `internal/plugin/trpc/adapter.go` — converts `biz.Plugin` DB rows to `trpcplugin.Plugin` via the built-in key registry
- `internal/plugin/trpc/audit.go` — `AuditLogPlugin`: records every `after_tool` callback to the structured logger (slog); writes to audit_log DB table planned for S5
- `internal/plugin/trpc/runtime.go` — thread-safe `Runtime` struct that holds the current active plugin slice and supports hot-reload via `Apply(ctx, []biz.Plugin)`
- `internal/plugin/trpc/permissions.go` — `Check(plugin, action)` + `AdminPermissions()` helpers

**Modified files**
- `internal/agent/trpc_runtime.go` — `TRPCRunnerDeps` gains `Plugins []trpcplugin.Plugin`; `NewTRPCRunner` passes them via `trpcrunner.WithPlugins`
- `internal/agent/turn_helpers.go` — `NewRunnerDepsFromRuntime` accepts variadic `plugins ...trpcplugin.Plugin`
- `internal/service/plugin.go` — `PluginService` holds `*plugintrpc.Runtime`; `ToggleEnabled` and `UpdateConfig` call `reloadRuntime` after mutation for immediate hot-reload
- `internal/server/metrics.go` — added `PluginInvokeTotal`, `PluginBlockTotal`, `AutoMemoryJobTotal`, `AutoMemoryExtractionDuration` Prometheus metrics
- `cmd/admin/wire_gen.go` — wired `plugintrpc.NewRuntime()` into `PluginService` constructor

---

## T30 — Skill Repository 适配 (M-3)

**New files**
- `internal/skill/trpc/db_repository.go` — `DBRepositoryAdapter` implementing `trpcskill.Repository` backed by `biz.SkillUsecase`; lazy-loads skill body on first `Get`; in-process LRU-style TTL cache (default 2 min); `Invalidate()` for manual cache bust

**Modified files**
- `internal/biz/skill.go` — added `GetBySlug`, `ListEnabledPublishedCandidates` convenience methods
- `internal/skill/watch/runner.go` — `Runner` gains optional `eventBus event.Bus`; added `NewRunnerWithBus` constructor; after successful `syncSlug`, publishes `event.Envelope{Type: "skill.reload"}` to bus

---

## T31 — Planner 多策略 (M-5)

**New files**
- `internal/agent/planner/selector.go` — `Select(dialogMode, plannerKind string) trpcplanner.Planner`; routes `""` / `"builtin"` / `"react"` / `"a2ui"` to the appropriate trpc-agent-go planner; preserves legacy `dialogMode="plan"` behavior when `plannerKind` is empty

**Modified files**
- `internal/biz/agent_types.go` — `AgentRuntimeSettings.PlannerKind string` field added
- `api/kratos/agent/v1/agent.proto` — `planner_kind = 100` added to `AgentRuntimeSettings`; `make api` regenerated Go + TypeScript clients
- `internal/agent/trpc_build.go` — replaced inline builtin-planner guard with `agentplanner.Select(deps.DialogMode, plannerKind(ag))`; removed `trpcbuiltin` direct import
- `internal/service/agent.go` — proto ↔ biz mapping for `PlannerKind`

---

## T32 — Memory 五件套工具 (M-2)

**New files**
- `internal/tools/memory/tools.go` — `DefaultTools() []trpctool.Tool` returning add / update / load / search / delete (no clear; destructive op excluded from default set)

**Modified files**
- `internal/agent/trpc_build.go` — when `ag.Settings.MemoryEnabled`, appends `memorytool.DefaultTools()` via `trpcllmagent.WithTools`; tools resolve `MemoryService` from invocation context at runtime (runner must be created with `WithMemoryService`)

---

## T33 — AutoMemory Job 闭环 (M-1)

**New files**
- `internal/cronrunner/jobs/auto_memory.go` — `AutoMemoryWorker` polls the global queue every 10 s (configurable); drains ≤ 50 jobs/tick; exponential retry (30 s / 2 m / 10 m, 3 attempts max); dead jobs logged + metrics emitted; lightweight keyword extraction (LLM-based extractor planned once provider catalog is available to worker)

**Modified files**
- `internal/memory/trpc/sqlite_adapter.go` — `EnqueueAutoMemoryJob` now pushes to a process-wide `chan AutoMemoryJobRequest` (capacity 256, drop-on-full for resilience); `GlobalAutoMemoryQueue()` exported for the cron worker

---

## Infrastructure

- `internal/conf/conf.proto` → `conf.pb.go` regenerated to include `Server.WS` config (fixes pre-existing `c.GetWs()` build error in `internal/server/ws.go`)

---

## Verification

- `go run ./cmd/araneactl/lint --root .` → 0 violations
- `go test ./internal/testutil/... ./internal/event/... ./internal/workspace/... ./internal/agent/callbacks/... ./pkg/apierror/... ./pkg/safego/...` → all pass
- `go build` on all new/modified packages → success
