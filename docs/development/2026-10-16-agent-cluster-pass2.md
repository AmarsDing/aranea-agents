# Agent cluster pass2 — 2026-10-16

Branch: `feat/agent-cluster-pass2` (from `dev` @ `f9dc637da` after #96).

## Fixes

| ID | Change |
|----|--------|
| **DAG-NIL-2** | `cascadeSkip` + `updateGraphNode` nil-guard `depStep`/`step` (same class as #95 `checkDownstream`) |
| **CAP-HARD** | Redis dual-write `AgentLoadTracker` + `CapacityHardGate` when Redis up; Allocate clears assignment with `capacity_hard_blocked` instead of soft keep |
| **CAP-EXPLICIT** | `AllocateExplicit` annotates `capacity_*_explicit` without reassigning caller intent |
| **SES-03** | `SessionRunUsecase.ListByPhase` takes `SessionRunPhase` (repo still string for DB) |

## Tests
- `go test ./internal/agent/ -run 'Capacity|SoftGate|LoadTracker|HardBlocked|SnapshotFor'`
- `go test ./internal/service/ -run 'CascadeSkip_Nil|UpdateGraphNode_Nil|TrackRelease'`
- `go test ./internal/biz/ -run 'SessionRun'`

## Remaining debt
- Publish Package A / connectors / review queue / computeruse-default (media MVP)
- Repo-layer SessionRun params still `string` (full SES-03)
- Hard gate requires Redis; without Redis behavior stays soft (documented)
