package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ResumeDurableSessionRun continues an agent turn from a durable checkpoint (CC-R-03).
func (s *ChatService) ResumeDurableSessionRun(ctx context.Context, sessionRunID string) error {
	if s == nil || s.orch == nil || s.orch.chJobs().SessionRuns == nil {
		return nil
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	if sessionRunID == "" {
		return nil
	}
	run, err := s.orch.chJobs().SessionRuns.Get(ctx, sessionRunID)
	if err != nil || run.ID == "" {
		return err
	}
	if run.Phase != biz.SessionRunPhaseDurable {
		return nil
	}
	if strings.TrimSpace(run.CheckpointID) == "" {
		return nil
	}
	if s.orch.HasActiveRun(run.SessionID) {
		return nil
	}
	claimed, err := s.orch.chJobs().SessionRuns.TryClaimDurableResume(ctx, sessionRunID)
	if err != nil || !claimed {
		return err
	}
	cp, err := s.orch.chJobs().SessionRuns.GetCheckpoint(ctx, sessionRunID)
	if err != nil || cp.ID == "" {
		if err := s.orch.chJobs().SessionRuns.ClearResumeClaim(ctx, sessionRunID); err != nil {
			s.lg.Warn("durable resume: clear claim failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
		}
		return err
	}
	payload, err := biz.ParseDurableCheckpointPayload(cp.PayloadJSON)
	if err != nil {
		if err := s.orch.chJobs().SessionRuns.ClearResumeClaim(ctx, sessionRunID); err != nil {
			s.lg.Warn("durable resume: clear claim failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
		}
		return err
	}
	deadline := time.Duration(biz.DefaultDurableDeadlineSec()) * time.Second
	runCtx, cancel := context.WithTimeout(context.Background(), deadline)
	safego.Go(runCtx, "session-run-durable-resume", func() {
		defer cancel()
		// 防卡死：goroutine panic 时必须把 run 落为 failed 并清除 resume claim
		// （Fail 内部已含 ClearResumeClaim），否则 run 永远卡在 claimed 状态无法
		// 再被 resume。随后 re-panic 交由 safego 记录堆栈并触发 PanicHook。
		defer func() {
			if r := recover(); r != nil {
				persistCtx := context.WithoutCancel(runCtx)
				if err := s.orch.chJobs().SessionRuns.Fail(persistCtx, sessionRunID, fmt.Sprintf("durable resume panic: %v", r)); err != nil {
					s.lg.Warn("durable resume: fail session run after panic failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
				}
				panic(r)
			}
		}()
		req := biz.TurnInput{
			SessionID: run.SessionID,
			Content:   biz.DurableResumePrompt(),
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint: biz.EntryPointDurable,
				AllowQueue: false,
			},
		}
		bgCtx := event.WithSessionRunID(event.WithEnvelopeSource(runCtx, run.Source), sessionRunID)
		bgCtx = event.WithDurableResume(bgCtx, event.DurableResumeSpec{
			SessionRunID:     sessionRunID,
			TurnID:           payload.TurnID,
			UserContent:      payload.UserContent,
			AgentID:          payload.AgentID,
			RuntimeRunID:     payload.RuntimeRunID,
			TrpcInvocationID: payload.TrpcInvocationID,
			SessionRevision:  payload.SessionRevision,
			DialogMode:       payload.DialogMode,
			Provider:         payload.Provider,
			Model:            payload.Model,
		})
		_, asst, turnErr := s.RunNativeTurn(bgCtx, req)
		persistCtx := context.WithoutCancel(runCtx)
		if turnErr != nil {
			if err := s.orch.chJobs().SessionRuns.Fail(persistCtx, sessionRunID, turnErr.Error()); err != nil {
				s.lg.Warn("durable resume: fail session run failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
			}
			if s.orch.chNotify().RunEscalation != nil {
				if failed, gerr := s.orch.chJobs().SessionRuns.Get(persistCtx, sessionRunID); gerr == nil && failed.ID != "" {
					if err := s.orch.chNotify().RunEscalation.NotifyRunFailed(persistCtx, failed, turnErr.Error()); err != nil {
						s.lg.Warn("durable resume: notify run failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
					}
				} else {
					if err := s.orch.chNotify().RunEscalation.NotifyRunFailed(persistCtx, biz.SessionRun{ID: sessionRunID, SessionID: run.SessionID}, turnErr.Error()); err != nil {
						s.lg.Warn("durable resume: notify run failed (fallback)", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
					}
				}
			}
			return
		}
		if err := s.orch.chJobs().SessionRuns.Complete(persistCtx, sessionRunID); err != nil {
			s.lg.Warn("durable resume: complete session run failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
		}
		if s.orch.chNotify().RunEscalation != nil {
			if completed, gerr := s.orch.chJobs().SessionRuns.Get(persistCtx, sessionRunID); gerr == nil && completed.ID != "" {
				if err := s.orch.chNotify().RunEscalation.NotifyRunCompleted(persistCtx, completed, asst.ContentMarkdown); err != nil {
					s.lg.Warn("durable resume: notify run completed failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
				}
			}
		}
	})
	return nil
}

func (s *ChatService) GetSessionRunUsecase() *biz.SessionRunUsecase {
	if s == nil || s.orch == nil {
		return nil
	}
	return s.orch.chJobs().SessionRuns
}
