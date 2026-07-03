# LLM Activity Ordering Phase 3a: Dead Code Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove dead v1 Plan code chain (Legacy Plan model + state machine + repo + schema), and remove the no-op Activity backfill migration stub. Migrate the still-active `LegacyPlanStatus` enum to `TaskPlanStatus` (owned by `task_plan.go`) before deletion.

**Architecture:** The legacy `biz.Plan` / `biz.PlanRepository` / `biz.PlanUsecase` types are dead code — not bound in wire.go, not called at runtime. However, the `LegacyPlanStatus` enum (defined in `plan.go`) is actively used by `biz.TaskPlan.Status` (task_plan.go:55) and by `chat_plan_confirm.go:60`. We first migrate the enum to `task_plan.go` (renamed `TaskPlanStatus`), update all references, then delete the dead Plan chain. Separately, `activity_backfill_migrate.go` is a no-op stub (only logs) still called from `data.go:749` — remove the call and the file.

**Tech Stack:** Go, Ent ORM, SQLite, Wire DI, Kratos v2

**Spec:** `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` §Phase 3 (lines 1191-1204)

---

## File Structure

### Modified Files

| File | Change |
|------|-------|
| `internal/biz/task_plan.go` | Add `TaskPlanStatus` type + constants (migrated from `plan.go`); change `TaskPlan.Status` field type |
| `internal/biz/agent_allocator.go` | (Verify only — uses `AllocationPlanRepository`, not Plan) |
| `internal/service/chat_plan_confirm.go` | Replace `biz.LegacyPlanStatusDraft` → `biz.TaskPlanStatusDraft` |
| `internal/agent/task_planner_impl.go` | Replace `LegacyPlanStatus*` → `TaskPlanStatus*` references |
| `internal/data/task_plan_repo.go` | Replace `biz.LegacyPlanStatus*` → `biz.TaskPlanStatus*` references |
| `internal/data/data.go` | Remove `RunActivityBackfillMigration` call (line 749-752) |
| `internal/service/chat_plan_query_test.go` | Replace `LegacyPlanStatus*` → `TaskPlanStatus*` references (if any) |

### Deleted Files (7 files)

| File | Lines | Reason |
|------|-------|--------|
| `internal/biz/plan.go` | 137 | Dead v1 Plan model + usecase (not wired) |
| `internal/biz/plan_state_machine.go` | 108 | Only used by dead `plan.go` usecase |
| `internal/biz/plan_state_machine_test.go` | ~170 | Tests for dead state machine |
| `internal/data/plan.go` | 122 | Dead `planRepo` (not wired; `NewPlanRepo` never called) |
| `internal/data/plan_schema.go` | 15 | Dead `EnsurePlanSchema` (never called) |
| `internal/data/sql/plan.sql` | 16 | Embedded by dead `plan_schema.go` |
| `internal/data/activity_backfill_migrate.go` | 38 | No-op stub (only logs; Phase 1c-3 deleted messages table) |

---

## Tier 1: Migrate LegacyPlanStatus → TaskPlanStatus

### Task 1: Add TaskPlanStatus to task_plan.go

**Files:**
- Modify: `internal/biz/task_plan.go`

- [ ] **Step 1: Add TaskPlanStatus type + constants to task_plan.go**

Open `internal/biz/task_plan.go`. Add the following type definition block **before** the `TaskPlan` struct (after the `OrchestrationStrategy` constants block, around line 27):

```go
// TaskPlanStatus represents the lifecycle state of a TaskPlan.
// (Migrated from plan.go's LegacyPlanStatus — the values are unchanged
// to preserve DB compatibility with the task_plans.status column.)
type TaskPlanStatus string

const (
	TaskPlanStatusDraft     TaskPlanStatus = "draft"
	TaskPlanStatusApproved  TaskPlanStatus = "approved"
	TaskPlanStatusConfirmed TaskPlanStatus = "confirmed"
	TaskPlanStatusExecuting TaskPlanStatus = "executing"
	TaskPlanStatusCompleted TaskPlanStatus = "completed"
	TaskPlanStatusFailed    TaskPlanStatus = "failed"
)
```

- [ ] **Step 2: Change TaskPlan.Status field type**

In `internal/biz/task_plan.go`, find the `TaskPlan` struct's `Status` field (line 55):

```go
// Status (reuses LegacyPlanStatus from plan.go)
Status LegacyPlanStatus `json:"status"`
```

Replace with:

```go
// Status is the lifecycle state of the plan.
Status TaskPlanStatus `json:"status"`
```

- [ ] **Step 3: Verify the file compiles in isolation**

Run: `go build ./internal/biz/`

Expected: **FAIL** with errors about `LegacyPlanStatus` not defined (in other files that still reference it). This is expected — we will fix references in subsequent tasks. The `task_plan.go` file itself should not have internal errors.

- [ ] **Step 4: Commit (intermediate, will fix references next)**

Do NOT commit yet — proceed to Task 2 to fix all references first.

### Task 2: Update all LegacyPlanStatus references

**Files:**
- Modify: `internal/service/chat_plan_confirm.go`
- Modify: `internal/agent/task_planner_impl.go`
- Modify: `internal/data/task_plan_repo.go`
- Modify: `internal/service/chat_plan_query_test.go` (if it references LegacyPlanStatus)

- [ ] **Step 1: Find all remaining LegacyPlanStatus references**

Run: `grep -rn "LegacyPlanStatus" internal/ --include="*.go"`

Expected output: a list of files still referencing `LegacyPlanStatus`. Each must be updated to use `TaskPlanStatus` instead. The constant prefix also changes from `LegacyPlanStatus*` to `TaskPlanStatus*`.

- [ ] **Step 2: Update internal/service/chat_plan_confirm.go**

In `internal/service/chat_plan_confirm.go`, find line 60:

```go
if plan.Status != biz.LegacyPlanStatusDraft {
```

Replace with:

```go
if plan.Status != biz.TaskPlanStatusDraft {
```

- [ ] **Step 3: Update internal/agent/task_planner_impl.go**

Run a targeted grep to find exact lines:

```bash
grep -n "LegacyPlanStatus" internal/agent/task_planner_impl.go
```

For each match, replace `LegacyPlanStatus` with `TaskPlanStatus` (type) and `LegacyPlanStatusDraft`/`LegacyPlanStatusApproved`/etc. with `TaskPlanStatusDraft`/`TaskPlanStatusApproved`/etc. Use `replace_all` semantics: every occurrence of the substring `LegacyPlanStatus` becomes `TaskPlanStatus`.

- [ ] **Step 4: Update internal/data/task_plan_repo.go**

Run:

```bash
grep -n "LegacyPlanStatus" internal/data/task_plan_repo.go
```

Replace every `LegacyPlanStatus` substring with `TaskPlanStatus` (same rename for constants).

- [ ] **Step 5: Update internal/service/chat_plan_query_test.go (if needed)**

Run:

```bash
grep -n "LegacyPlanStatus" internal/service/chat_plan_query_test.go
```

If matches found, replace `LegacyPlanStatus` → `TaskPlanStatus` (and constants).

- [ ] **Step 6: Verify no remaining LegacyPlanStatus references**

Run: `grep -rn "LegacyPlanStatus" internal/ --include="*.go"`

Expected: **only** matches in `internal/biz/plan.go` and `internal/biz/plan_state_machine.go` and `internal/biz/plan_state_machine_test.go` (these will be deleted in Tier 2).

- [ ] **Step 7: Verify backend builds**

Run: `go build -tags=pgvector ./...`

Expected: **PASS** (no compile errors). The `plan.go` file still defines `LegacyPlanStatus` so it compiles, but `task_plan.go` now defines `TaskPlanStatus` independently.

- [ ] **Step 8: Run biz + data + service tests**

Run: `go test -tags=pgvector ./internal/biz/... ./internal/data/... ./internal/service/... -count=1`

Expected: **PASS** (all existing tests pass with the renamed type).

- [ ] **Step 9: Commit**

```bash
git add internal/biz/task_plan.go internal/service/chat_plan_confirm.go internal/agent/task_planner_impl.go internal/data/task_plan_repo.go internal/service/chat_plan_query_test.go
git commit -m "$(cat <<'EOF'
refactor(biz): migrate LegacyPlanStatus → TaskPlanStatus

Move the plan status enum from the dead plan.go to the active
task_plan.go, renaming it to reflect its actual ownership. All
references in service/agent/data layers updated.

The legacy plan.go and plan_state_machine.go still define
LegacyPlanStatus (unused outside their own files) — they will be
deleted in the next commit.
EOF
)"
```

---

## Tier 2: Delete Dead v1 Plan Chain

### Task 3: Delete dead Plan files

**Files:**
- Delete: `internal/biz/plan.go`
- Delete: `internal/biz/plan_state_machine.go`
- Delete: `internal/biz/plan_state_machine_test.go`
- Delete: `internal/data/plan.go`
- Delete: `internal/data/plan_schema.go`
- Delete: `internal/data/sql/plan.sql`

- [ ] **Step 1: Verify plan.go is dead (sanity check)**

Run:

```bash
grep -rn "biz.PlanRepository\|biz.PlanUsecase\|biz.Plan\b\|NewPlanRepo\|NewPlanUsecase\|biz.LegacyPlanStep" internal/ cmd/ --include="*.go" | grep -v "_test.go" | grep -v "plan.go\|plan_state_machine"
```

Expected: **empty output** (no references to the dead types outside the files being deleted). If any reference appears, STOP and fix it first.

Note: `biz.Plan` (without trailing word char) might match `biz.PlanBoard` or `biz.PlanStep` — those are v2 types and should NOT be deleted. The regex `\b` boundary handles this.

- [ ] **Step 2: Verify EnsurePlanSchema is dead**

Run: `grep -rn "EnsurePlanSchema" internal/ cmd/ --include="*.go"`

Expected: only the definition in `plan_schema.go:13`. No callers.

- [ ] **Step 3: Delete the 6 files**

Delete:
- `internal/biz/plan.go`
- `internal/biz/plan_state_machine.go`
- `internal/biz/plan_state_machine_test.go`
- `internal/data/plan.go`
- `internal/data/plan_schema.go`
- `internal/data/sql/plan.sql`

- [ ] **Step 4: Verify backend builds**

Run: `go build -tags=pgvector ./...`

Expected: **PASS**. No compilation errors — all deleted code was dead.

- [ ] **Step 5: Run biz + data tests**

Run: `go test -tags=pgvector ./internal/biz/... ./internal/data/... -count=1`

Expected: **PASS** (the deleted `plan_state_machine_test.go` is gone; no other test should reference the deleted types).

- [ ] **Step 6: Commit**

```bash
git add -A internal/biz/plan.go internal/biz/plan_state_machine.go internal/biz/plan_state_machine_test.go internal/data/plan.go internal/data/plan_schema.go internal/data/sql/plan.sql
git commit -m "$(cat <<'EOF'
refactor(biz): delete dead v1 Plan code chain

Remove the legacy Plan model, its state machine, repo, and schema
DDL — none of these were wired into the runtime (NewPlanRepo and
NewPlanUsecase have zero callers in wire.go). The active TaskPlan
model now owns the status enum (TaskPlanStatus, migrated in the
previous commit).

Deleted files:
- internal/biz/plan.go (137 lines)
- internal/biz/plan_state_machine.go (108 lines)
- internal/biz/plan_state_machine_test.go (~170 lines)
- internal/data/plan.go (122 lines)
- internal/data/plan_schema.go (15 lines)
- internal/data/sql/plan.sql (16 lines)

Note: the `plans` table is NOT dropped in this commit — the DDL
was never registered in ddl_migration_registry.go (EnsurePlanSchema
was never called), so the table only exists if created manually.
A future migration can DROP TABLE IF EXISTS plans safely.
EOF
)"
```

---

## Tier 3: Remove No-Op Activity Backfill Migration

### Task 4: Delete activity_backfill_migrate.go and remove its call

**Files:**
- Delete: `internal/data/activity_backfill_migrate.go`
- Modify: `internal/data/data.go` (remove lines 749-752)

- [ ] **Step 1: Read activity_backfill_migrate.go to confirm it's a no-op**

Read `internal/data/activity_backfill_migrate.go`. The function `RunActivityBackfillMigration` should only contain a log statement (no actual DB work) — the comment says "Phase 1c-3 deleted the messages table; backfill is now handled by SQL migration 20260902".

- [ ] **Step 2: Remove the call from data.go**

In `internal/data/data.go`, find the block (around lines 749-752):

```go
	if err := RunActivityBackfillMigration(ctx, entClient, d.Dialect(), d.lg); err != nil {
		d.lg.Error("migration step failed", loggateway.StepID("data.migration.activity_backfill"), loggateway.Err(err))
		return fmt.Errorf("activity backfill migration: %w", err)
	}
```

Delete this entire block (4 lines + the blank line if present).

- [ ] **Step 3: Delete activity_backfill_migrate.go**

Delete: `internal/data/activity_backfill_migrate.go`

- [ ] **Step 4: Verify no remaining references**

Run: `grep -rn "RunActivityBackfillMigration\|activity_backfill_migrate" internal/ cmd/ --include="*.go"`

Expected: **empty output**.

- [ ] **Step 5: Verify backend builds**

Run: `go build -tags=pgvector ./...`

Expected: **PASS**.

- [ ] **Step 6: Run data tests**

Run: `go test -tags=pgvector ./internal/data/... -count=1`

Expected: **PASS**.

- [ ] **Step 7: Commit**

```bash
git add -A internal/data/activity_backfill_migrate.go internal/data/data.go
git commit -m "$(cat <<'EOF'
refactor(data): remove no-op Activity backfill migration stub

The RunActivityBackfillMigration function was a no-op stub: the
messages table it once backfilled from was deleted in Phase 1c-3,
and the function only emitted a log line. Remove the function and
its call from runPendingDataMigrations.

Note: this does NOT remove the activities table itself — that
table is still actively written by the v1 ActivityProjector and
will be removed in Phase 3b once the v1 projector is decommissioned.
EOF
)"
```

---

## Tier 4: Final Verification

### Task 5: Full verification + regenerate wire

**Files:**
- Verify only

- [ ] **Step 1: Regenerate wire (in case any provider signatures changed)**

Run: `make wire`

Expected: **PASS**. `wire_gen.go` should be unchanged (we didn't modify any provider function signatures — `NewPlanRepo` was never in the wire set).

- [ ] **Step 2: Full backend build**

Run: `go build -tags=pgvector ./...`

Expected: **PASS**.

- [ ] **Step 3: Full backend test suite**

Run: `go test -tags=pgvector ./... -count=1`

Expected: **PASS** (all tests pass).

- [ ] **Step 4: Lint (if available)**

Run: `go vet -tags=pgvector ./...`

Expected: **PASS**.

- [ ] **Step 5: Verify no dangling references to deleted types**

Run:

```bash
grep -rn "LegacyPlanStatus\|biz.PlanRepository\|biz.PlanUsecase\|biz.LegacyPlanStep\|NewPlanRepo\|NewPlanUsecase\|EnsurePlanSchema\|RunActivityBackfillMigration" internal/ cmd/ --include="*.go"
```

Expected: **empty output** (all deleted types/functions have no remaining references).

- [ ] **Step 6: Run app end-to-end smoke test (manual)**

Start the backend:

```powershell
$env:KRATOS_HTTP_AUTH_DISABLED="1"
$env:DEPLOY_ENV="dev"
$env:DAO_VECTOR_PGVECTOR="1"
go run -tags=pgvector ./cmd/admin/ -conf configs/
```

Expected: server starts without migration errors. The `plans` table (if it exists from prior runs) is harmless — no code reads it.

Open `http://localhost:8000` in a browser, navigate to chat, send a test message. Verify:
- Chat responds normally
- TaskPlan flow works (if a complex query triggers planning, the plan confirmation should still work — it uses `TaskPlanStatusDraft`)

- [ ] **Step 7: Commit final verification (if any cleanup needed)**

If `make wire` produced changes to `wire_gen.go` (it shouldn't), commit them:

```bash
git add cmd/admin/wire_gen.go
git commit -m "chore(wire): regenerate wire_gen.go after Phase 3a cleanup"
```

Otherwise, no commit needed — Phase 3a is complete.

---

## Phase 3b Outline (Not Executed in This Plan)

Phase 3b removes the **active** v1 Activity chain. This is a much larger refactor (40+ files) and must be planned separately after Phase 3a is verified. High-level scope:

### Stage 1: Migrate service layer v1 event publishing (15+ files)
- `spirit_team.go` (~90 v1 event publishes)
- `chat_event_publisher.go`, `chat_orchestrator_turn_phases.go`, `run_status_publish.go`, etc.
- Each `biz.ActivityEventCreated`/`ActivityEventCompleted` publish must migrate to v2 `Event` interface or be removed if redundant.

### Stage 2: Migrate team layer v1 references (11 files)
- `runner_team_trpc.go`, `team_graph_run_coordinator.go`, etc.

### Stage 3: Migrate plugin/hook files (4 files)
- `cost_guard.go`, `model_router.go`, `tool_confirmation.go`, `runner_team_trpc.go` (uses `biz.ActivityEmitter`)

### Stage 4: Remove v1 path from stream_consumer.go
- Keep v2 `v2Projector.ProcessEvent` path
- Delete `opts.ActivityProjector.ProcessEvent` path + `v2ProjectMetaFromV1` converter

### Stage 5: Remove v1 wire bindings
- `activityevent.New` provider + `wire.Bind(new(biz.ActivityEventBus), ...)`
- Remove `activityBus biz.ActivityEventBus` parameter from 10+ providers

### Stage 6: Delete v1 Activity chain
- `internal/agent/activity_projector.go` (1841 lines)
- `internal/agent/activity_event_sequencer.go` (474 lines)
- `internal/event/activityevent/bus.go` (146 lines)
- `internal/biz/activity.go` (216 lines)
- `internal/biz/activity_event.go` (150 lines)
- `internal/biz/activity_state_machine.go` (133 lines)
- `internal/data/activity_repo.go` (413 lines)
- `internal/data/ent/schema/activity.go` (95 lines)

### Stage 7: DDL migration to DROP activities table
- Register in `ddl_migration_registry.go`:
  ```go
  {Version: 20260801, Name: "drop_activities_table", SQL: "sql/migrations/20260801_drop_activities.sql"},
  ```
- SQL: `DROP TABLE IF EXISTS activities;`

### Risk Notes
- Stage 1 is the highest-risk stage: `spirit_team.go` publishes v1 events that the frontend may still rely on for system-level notifications. Must verify v2 events cover all use cases.
- Stage 5 (wire bindings) may cascade — removing `activityBus` from one provider may require updating callers.
- `useChatWorkspace.ts` still has `handleActivityEvent` for system event routing — frontend v1 system event path must be migrated before backend can fully remove v1 ActivityEventBus.

**Phase 3b requires its own detailed plan with TDD tasks per file. Do NOT execute Phase 3b without a separate plan.**

---

## Self-Review

### Spec Coverage
- Design doc §Phase 3 says "delete `internal/biz/activity.go`, `internal/biz/activity_event.go`, `internal/biz/plan.go`". This plan covers `plan.go` deletion (Phase 3a). `activity.go` and `activity_event.go` are still actively used (25 service files) and are deferred to Phase 3b (outlined).
- Design doc says "delete `internal/data/ent/schema/activity.go`" — deferred to Phase 3b (active schema, still written to).
- Design doc says "delete old DDL migrations referencing activities table" — deferred to Phase 3b.
- Design doc mentions `task_plan.go` as a deletion target — **this is a design doc error**. `task_plan.go` is the active TaskPlan model (wired, used by TaskPlanner). Only the legacy `plan.go` is dead. This plan corrects the design doc error by deleting `plan.go` and preserving `task_plan.go`.

### Placeholder Scan
- No "TBD"/"TODO"/"implement later" placeholders.
- Every code step shows actual code or exact grep commands.
- Phase 3b outline is intentionally high-level (marked "Not Executed in This Plan") — it's a roadmap, not executable steps.

### Type Consistency
- `TaskPlanStatus` (with constants `TaskPlanStatusDraft`/`Approved`/`Confirmed`/`Executing`/`Completed`/`Failed`) is used consistently across all tasks.
- String values (`"draft"`, `"approved"`, etc.) are unchanged from `LegacyPlanStatus` — DB compatibility preserved.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-03-llm-activity-ordering-phase3a.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
