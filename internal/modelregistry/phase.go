package modelregistry

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

type PhaseStatus string

const (
	PhaseSucceeded PhaseStatus = "succeeded"
	PhaseFailed    PhaseStatus = "failed"
	PhaseSkipped   PhaseStatus = "skipped"
)

type Phase interface {
	Name() string
	Timeout() time.Duration
	Run(pc *PhaseContext) PhaseResult
}

type PhaseResult struct {
	PhaseName  string
	Status     PhaseStatus
	Duration   time.Duration
	Stats      map[string]int
	Errors     []string
	Checkpoint *MigrationCheckpoint
}

type PhaseContext struct {
	Ctx        context.Context
	Store      *Store
	Backend    ApplyBackend
	Reader     ApplyReader
	Writer     ApplyWriter
	Migrator   MigrationWriter
	Directory  Directory
	Policy     Policy
	Checkpoint *MigrationCheckpoint
	Lg         loggateway.Logger
}

type phaseCtxKey struct{}

func WithPhaseCtx(ctx context.Context, pc *PhaseContext) context.Context {
	return context.WithValue(ctx, phaseCtxKey{}, pc)
}

func PhaseFromCtx(ctx context.Context) *PhaseContext {
	pc, _ := ctx.Value(phaseCtxKey{}).(*PhaseContext)
	return pc
}
