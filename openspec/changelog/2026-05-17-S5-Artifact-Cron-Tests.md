# S5 Changelog — Artifact / Cron / Settings Split / Tests 60%

> Merged: 2026-05-17 | Sprint: S5 (weeks 9–10) | Tasks: T34–T37

---

## T34 — Artifact Minimal Implementation

**Backend**

- New proto: `api/kratos/artifact/v1/artifact.proto`  
  RPCs: `UploadArtifact`, `GetArtifact`, `ListArtifacts`, `DeleteArtifact`.  
  Payload transferred as standard base64 (no streaming); 50 MB size cap enforced.
- `internal/biz/artifact.go` — `ArtifactRepo` interface + `ArtifactUsecase`.
- `internal/data/artifactfs/repo.go` — filesystem-backed implementation; versioned storage (`v1.bin`, `v2.bin`, …) with `meta.json` sidecar; isolated sub-package to avoid transitive Ent imports.
- `internal/artifact/trpc/service.go` — adapter implementing `trpc-agent-go/artifact.Service` for agent-runtime access.
- `internal/service/artifact.go` — Kratos HTTP handler.
- `docs/guides/artifact.md` — usage & config guide.

**Metrics added**  
`aranea_artifact_upload_bytes_total`, `aranea_artifact_download_bytes_total`, `aranea_artifact_storage_bytes` (declared in `internal/server/metrics.go`).

---

## T35 — Cron Retry + Metrics + Dead-Letter

- `internal/cronrunner/runner.go`:
  - `dispatchWithRetry` — 3-attempt exponential backoff (30 s → 2 m → 10 m).
  - `dispatchSafe` — panic recovery wrapper via `pkg/safego`.
  - Dead-letter: after `maxDeadFailures = 3` consecutive run-level failures the job is set `status=dead, enabled=false` and a `cron.dead_letter` event is emitted.
  - Prometheus metrics declared locally (removed dependency on `internal/server` to prevent transitive Ent import in tests).
- `docs/guides/cron.md` — retry policy & metrics guide.

**Metrics added**  
`aranea_cron_job_runs_total{job_id,status}`, `aranea_cron_job_duration_seconds{job_id}`, `aranea_cron_job_dead_total{job_id}`.

---

## T36 — AgentRuntimeSettings Domain Split

- `internal/biz/agent_settings.go` — eight new `*Cfg` sub-structs grouping the 100+ flat fields into logical domains:
  `IdentityCfg`, `ReasoningCfg`, `MemoryCfg`, `ToolsCfg`, `SkillsCfg`, `PluginsCfg`, `EvolutionCfg`, `ContextCfg`.
- `internal/biz/agent_types.go` — `Get*()` accessor methods (`GetIdentity()`, `GetMemory()`, …) return structured domain views without altering the flat DB layout (zero migration needed).
- `internal/service/agent.go` — removed phantom fields `L0CompressProvider`/`L0CompressModel` from proto mapping; updated `fromProtoRuntime`/`toProtoRuntime`.
- `internal/agent/trpc_build.go`, `session_compress.go`, `intent/pass.go` — updated to use direct field access after accessor-naming conflict resolution.

---

## T37 — Test Coverage 60% + CI

**Root cause fix**  
`internal/data/ent/schema/session.go` — `Default(0)` on `Float` fields changed to `Default(0.0)`. This eliminated the `interface conversion: interface {} is nil, not float64` init panic that blocked testing of all packages importing `data/ent` transitively.

**New test files**

| File | Coverage |
|------|----------|
| `internal/data/artifactfs/repo_test.go` | 78.7% |
| `internal/artifact/trpc/service_test.go` | 66.0% |
| `internal/biz/agent_settings_test.go` | — |
| `internal/biz/biz_coverage_test.go` (Team, Cron, Plugin, Artifact usecases) | — |
| `internal/cronrunner/retry_test.go` | — |
| `internal/service/artifact_test.go` | — |
| `internal/service/team_test.go` | — |
| `internal/service/cron_test.go` | — |
| `internal/service/plugin_test.go` | — |
| `internal/service/skill_test.go` | — |

**Frontend test scaffold**

- `web/vitest.config.ts` — Vitest + happy-dom + v8 coverage config.
- `web/src/stores/__tests__/app.store.spec.ts` — 9 tests for `useAppStore` (agents, sessions, messages).
- `web/src/features/cron/__tests__/cron-types.spec.ts` — type-shape and retry-logic purity tests.
- `web/src/features/agents/__tests__/wireNormalize.spec.ts` — 14 tests for `normalizeRuntimeSettingsFromWire`, `normalizeAgentFromService`, `normalizePromptFileFromWire`.
- `web/package.json` — added `vitest`, `@vitest/coverage-v8`, `@pinia/testing`, `happy-dom` dev deps; added `test`, `test:watch`, `test:coverage` scripts.

**CI updates (`ci.yml`)**

- `test-go` job: expanded testable packages list; threshold raised from **30% → 60%**.
- Nightly `e2e-nightly` job added: runs on `schedule` (02:00 UTC) and `workflow_dispatch`; starts the admin binary against an SQLite dev DB; runs Cypress headlessly; failures are `continue-on-error` (non-blocking for PRs).

---

## Ent Schema Fix

`internal/data/ent/schema/session.go`

```go
// Before (caused init() panic in all test binaries importing data/ent):
field.Float("context_used_ratio").Default(0),
field.Float("max_context_used_ratio").Default(0),
field.Float("avg_latency_ms").Default(0),

// After:
field.Float("context_used_ratio").Default(0.0),
field.Float("max_context_used_ratio").Default(0.0),
field.Float("avg_latency_ms").Default(0.0),
```

`go generate aranea-agents/internal/data/ent` was re-run to update the generated `runtime.go`.

---

## Files Changed

```
api/kratos/artifact/v1/artifact.proto       [new]
docs/changelog/2026-05-17-S5-Artifact-Cron-Tests.md  [new]
docs/guides/artifact.md                     [new]
docs/guides/cron.md                         [new]
internal/artifact/trpc/service.go           [new]
internal/artifact/trpc/service_test.go      [new]
internal/biz/agent_settings.go              [new]
internal/biz/agent_settings_test.go         [new]
internal/biz/agent_types.go                 [modified]
internal/biz/artifact.go                    [new]
internal/biz/biz_coverage_test.go           [new]
internal/cronrunner/retry_test.go           [new]
internal/cronrunner/runner.go               [modified]
internal/data/artifactfs/repo.go            [new]
internal/data/artifactfs/repo_test.go       [new]
internal/data/ent/runtime.go                [regenerated]
internal/data/ent/schema/session.go         [modified]
internal/server/metrics.go                  [modified]
internal/service/agent.go                   [modified]
internal/service/artifact.go                [new]
internal/service/artifact_test.go           [new]
internal/service/cron_test.go               [new]
internal/service/plugin_test.go             [new]
internal/service/skill_test.go              [new]
internal/service/team_test.go               [new]
web/package.json                            [modified]
web/vitest.config.ts                        [new]
web/src/features/agents/__tests__/wireNormalize.spec.ts  [new]
web/src/features/cron/__tests__/cron-types.spec.ts       [new]
web/src/stores/__tests__/app.store.spec.ts               [new]
.github/workflows/ci.yml                    [modified]
```
