package biz

import (
	"context"
	"fmt"
)

// ── Meta Team process activity mounting (73-self-iteration-v3, T3.6) ────────
//
// The pipeline mounts its stage progression as a two-level activity tree:
//
//	si-run:<runID>                       (root, stage=run)
//	├── si-run:<runID>:diagnosing
//	├── si-run:<runID>:patching:a<N>     (attempt-scoped, retry → new node)
//	├── si-run:<runID>:verifying:a<N>
//	└── si-run:<runID>:governing
//
// IDs are deterministic so the service layer can upsert ordered Activity
// records; every stage node's ParentActivityID resolves to the run root
// (resolveParentActivityID 规范). The sink is a biz port — service 层将其
// 接到 Activity/EventBus 基础设施；nil sink 时流水线行为不变。

// SIActivityStage labels one pipeline stage node kind.
type SIActivityStage = string

const (
	SIStageRun        SIActivityStage = "run"
	SIStageDiagnosing SIActivityStage = "diagnosing"
	SIStagePatching   SIActivityStage = "patching"
	SIStageVerifying  SIActivityStage = "verifying"
	SIStageGoverning  SIActivityStage = "governing"
)

// SIRunActivityID returns the deterministic root activity ID of a run.
func SIRunActivityID(runID string) string { return "si-run:" + runID }

// SIStageActivityID returns the deterministic activity ID of a stage node.
// attempt > 0 appends an :a<N> suffix (patching/verifying retry tree);
// attempt == 0 (diagnosing/governing) carries no suffix.
func SIStageActivityID(runID, stage string, attempt int) string {
	if attempt > 0 {
		return fmt.Sprintf("si-run:%s:%s:a%d", runID, stage, attempt)
	}
	return fmt.Sprintf("si-run:%s:%s", runID, stage)
}

// SIActivityRecord is one node of the Meta Team process activity tree.
type SIActivityRecord struct {
	ID               string
	ParentActivityID string // root 为 ""，stage 节点恒为 SIRunActivityID(runID)
	RunID            string
	Stage            string // SIStage*
	Attempt          int    // run/diagnosing/governing 为 0；patching/verifying ≥1
	Status           ActivityStatus
	Summary          string
}

// SIActivitySink emits Meta Team process activity nodes. Emission errors are
// logged and swallowed by the pipeline (observability degradation must not
// break the loop).
// Stability:evolving
type SIActivitySink interface {
	EmitSIActivity(ctx context.Context, a SIActivityRecord) error
}
