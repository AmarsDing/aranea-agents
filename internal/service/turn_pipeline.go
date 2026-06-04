package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TurnIngress normalizes a transport-specific request into a canonical intent.
type TurnIngress interface {
	BuildTurnIntent(ctx context.Context) (biz.TurnIntent, error)
}

// TurnService owns turn admission and persistent lifecycle state.
type TurnService interface {
	AdmitTurn(ctx context.Context, intent biz.TurnIntent) (biz.Turn, error)
	CompleteTurn(ctx context.Context, turn biz.Turn, result biz.NativeTurnResult) (biz.Turn, error)
	FailTurn(ctx context.Context, turn biz.Turn, err error) (biz.Turn, error)
}

// TurnExecutor runs an admitted turn against the selected runtime.
type TurnExecutor interface {
	ExecuteTurn(ctx context.Context, turn biz.Turn, input biz.TurnInput) (biz.NativeTurnResult, error)
}

// TurnProjector projects canonical turn events into chat WS, channel outbound, monitor, and session views.
type TurnProjector interface {
	ProjectTurnEvent(ctx context.Context, event biz.TurnEvent) error
}

// TurnPipeline is the explicit Ingress -> TurnService -> Executor -> Projector boundary.
type TurnPipeline struct {
	Service   TurnService
	Executor  TurnExecutor
	Projector TurnProjector
	Now       func() time.Time
}

// Run executes the canonical pipeline without knowing whether the request came from Web, WS, or Channel.
func (p TurnPipeline) Run(ctx context.Context, intent biz.TurnIntent) (biz.Turn, biz.NativeTurnResult, error) {
	intent = intent.Canonicalize()
	lg := loggateway.Global()
	start := time.Now()

	turn, err := p.Service.AdmitTurn(ctx, intent)
	if err != nil {
		if lg != nil {
			lg.With(loggateway.SessionID(intent.SessionID)).Info("TurnPipeline.Run: AdmitTurn 失败",
				loggateway.StepID("pipeline.admit_fail"), loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()), loggateway.Err(err))
		}
		return biz.Turn{}, biz.NativeTurnResult{}, err
	}
	if lg != nil {
		lg.With(loggateway.SessionID(intent.SessionID)).Info("TurnPipeline.Run: AdmitTurn 完成",
			loggateway.StepID("pipeline.admit_done"), loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()), loggateway.Any("turn_id", turn.ID))
	}
	p.project(ctx, biz.TurnEvent{
		TurnID:     turn.ID,
		SessionID:  turn.SessionID,
		Type:       biz.TurnEventQueued,
		Source:     turn.Source,
		Status:     turn.Status,
		OccurredAt: p.now(),
	})

	result, execErr := p.Executor.ExecuteTurn(ctx, turn, intent.TurnInput())
	if lg != nil {
		lg.With(loggateway.SessionID(intent.SessionID)).Info("TurnPipeline.Run: ExecuteTurn 完成",
			loggateway.StepID("pipeline.execute_done"),
			loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
			loggateway.Any("has_error", execErr != nil),
			loggateway.Any("outcome", string(result.Outcome)))
	}
	if execErr != nil {
		failed, failErr := p.Service.FailTurn(ctx, turn, execErr)
		if failErr == nil {
			turn = failed
		}
		p.project(ctx, biz.TurnEvent{
			TurnID:     turn.ID,
			SessionID:  turn.SessionID,
			Type:       biz.TurnEventFailed,
			Source:     turn.Source,
			Status:     biz.TurnStatusFailed,
			OccurredAt: p.now(),
			Metadata:   map[string]string{"error": execErr.Error()},
		})
		return turn, result, execErr
	}

	if result.Outcome == biz.NativeTurnOutcomeQueued {
		p.project(ctx, biz.TurnEvent{
			TurnID:     turn.ID,
			SessionID:  turn.SessionID,
			Type:       biz.TurnEventQueued,
			Source:     turn.Source,
			Status:     biz.TurnStatusQueued,
			OccurredAt: p.now(),
		})
		return turn, result, nil
	}

	if result.Outcome == biz.NativeTurnOutcomeFailed {
		failed, failErr := p.Service.FailTurn(ctx, turn, nil)
		if failErr == nil {
			turn = failed
		}
		p.project(ctx, biz.TurnEvent{
			TurnID:     turn.ID,
			SessionID:  turn.SessionID,
			Type:       biz.TurnEventFailed,
			Source:     turn.Source,
			Status:     biz.TurnStatusFailed,
			OccurredAt: p.now(),
		})
		return turn, result, nil
	}

	completed, err := p.Service.CompleteTurn(ctx, turn, result)
	if err != nil {
		return turn, result, err
	}
	turn = completed
	p.project(ctx, biz.TurnEvent{
		TurnID:     turn.ID,
		SessionID:  turn.SessionID,
		Type:       biz.TurnEventCompleted,
		Source:     turn.Source,
		Status:     biz.TurnStatusFromNativeOutcome(result.Outcome),
		OccurredAt: p.now(),
	})
	return turn, result, nil
}

func (p TurnPipeline) project(ctx context.Context, ev biz.TurnEvent) {
	if p.Projector == nil {
		return
	}
	_ = p.Projector.ProjectTurnEvent(ctx, ev)
}

func (p TurnPipeline) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}
