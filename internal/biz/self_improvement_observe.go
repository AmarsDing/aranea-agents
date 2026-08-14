package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

// SIPlatformScanTargetID is the pseudo target ID passed to the orchestrator
// for platform-wide signal scans. Platform suggestions carry per-signature
// TargetIDs ("<trigger_source>/<signature>"), so this constant never matches
// a stored suggestion — CheckAndCreate's whole-target pending short-circuit
// stays inert and dedup falls to the per-signature cooldown + the DB
// pattern_hash unique index (design D2).
const SIPlatformScanTargetID = "platform"

// siObserveBatchSize pages pending platform suggestions per scan.
const siObserveBatchSize = 100

// SelfImprovementObserveUsecase drives the Observe stage of the platform
// self-improvement loop (73-self-iteration-v3, design §5 self_improve_observe):
//
//  1. Scan: call the unified orchestrator CheckAndCreate(platform) so the four
//     registered triggers turn live signals into pending suggestions.
//  2. Materialize: for every pending platform suggestion without a run, create
//     a SelfImprovementRun in status=detected. Idempotent via
//     GetBySuggestionID; later stages (diagnose/patch/...) pick the run up.
//
// The usecase tolerates a scan failure (existing pendings still materialize)
// and individual run-create failures (remaining suggestions still processed).
type SelfImprovementObserveUsecase struct {
	orch        *SkillEvolutionOrchestrator
	suggestions UnifiedEvolutionQueryReader
	runReader   SelfImprovementRunReader
	runWriter   SelfImprovementRunWriter
	lg          loggateway.Logger
}

// NewSelfImprovementObserveUsecase creates the usecase. All deps are nil-safe:
// a nil orchestrator skips the scan, nil readers/writers skip materialization.
func NewSelfImprovementObserveUsecase(
	orch *SkillEvolutionOrchestrator,
	suggestions UnifiedEvolutionQueryReader,
	runReader SelfImprovementRunReader,
	runWriter SelfImprovementRunWriter,
	lg loggateway.Logger,
) *SelfImprovementObserveUsecase {
	return &SelfImprovementObserveUsecase{
		orch:        orch,
		suggestions: suggestions,
		runReader:   runReader,
		runWriter:   runWriter,
		lg:          lg,
	}
}

// ScanOnce executes one observe cycle and returns the number of runs created.
// The first run-create error (if any) is returned after the batch finishes.
func (uc *SelfImprovementObserveUsecase) ScanOnce(ctx context.Context) (int, error) {
	if uc == nil || uc.suggestions == nil || uc.runReader == nil || uc.runWriter == nil {
		return 0, nil
	}

	// 1. Trigger scan. Failure is non-fatal: pre-existing pendings still
	//    materialize below.
	if uc.orch != nil {
		if _, err := uc.orch.CheckAndCreate(ctx, EvolutionTargetPlatform, SIPlatformScanTargetID); err != nil {
			uc.lg.Warn("self-improve observe: trigger scan failed",
				loggateway.StepID("si_observe.scan"),
				loggateway.Err(err))
		}
	}

	// 2. Materialize runs for pending platform suggestions.
	created := 0
	var firstErr error
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return created, ctx.Err()
		default:
		}
		batch, err := uc.suggestions.ListByTarget(ctx, string(EvolutionTargetPlatform), "", "", string(UnifiedEvolutionStatePending), siObserveBatchSize, offset)
		if err != nil {
			return created, err
		}
		for i := range batch {
			s := &batch[i]
			if s.TargetType != EvolutionTargetPlatform {
				continue // 双保险：查询层已按 platform 过滤
			}
			existing, err := uc.runReader.GetBySuggestionID(ctx, s.ID)
			if err != nil {
				uc.lg.Warn("self-improve observe: run lookup failed, skipping suggestion",
					loggateway.StepID("si_observe.lookup"),
					loggateway.Str("suggestion_id", s.ID),
					loggateway.Err(err))
				continue
			}
			if existing != nil {
				continue
			}
			now := time.Now().UTC()
			run := &SelfImprovementRun{
				ID:            newBizID(),
				SuggestionID:  s.ID,
				Status:        RunStatusDetected,
				TriggerSource: s.TriggerSource,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if err := uc.runWriter.Create(ctx, run); err != nil {
				uc.lg.Warn("self-improve observe: run create failed",
					loggateway.StepID("si_observe.create"),
					loggateway.Str("suggestion_id", s.ID),
					loggateway.Err(err))
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			created++
			uc.lg.Info("self-improve run detected",
				loggateway.StepID("si_observe.create"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Str("trigger_source", s.TriggerSource),
				loggateway.Str("run_id", run.ID))
		}
		if len(batch) < siObserveBatchSize {
			return created, firstErr
		}
		offset += siObserveBatchSize
	}
}
