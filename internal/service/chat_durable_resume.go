package service

import (
	"context"
	"errors"
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
	// P0 FR-4：claim 与 resume turn 完成进程内注册之间存在窗口，期间同会话
	// 的新 turn 会误判空闲并抢跑（绕过 durable claim 的先到语义）。登记占位
	// 使窗口期对外 busy；本 resume turn 携 DurableResume 标记在锁内采用占位。
	// 若槽位与 :34 检查之间恰好被并发 turn 抢占，按 busy 瞬态交还 claim，
	// 待会话空闲由 worker 下一 tick 重试——绝不裸 Store 覆盖真实条目（会
	// 冲掉其 cancel 函数，破坏 M3 取消语义）。
	if !s.orch.runs.StorePlaceholderIfVacant(run.SessionID) {
		s.lg.Info("durable resume deferred: session slot raced",
			loggateway.Str("session_run_id", sessionRunID), loggateway.Str("session_id", run.SessionID))
		if cerr := s.orch.chJobs().SessionRuns.ClearResumeClaim(ctx, sessionRunID); cerr != nil {
			s.lg.Warn("durable resume: clear claim after placeholder race failed", loggateway.Err(cerr), loggateway.Str("session_run_id", sessionRunID))
		}
		return nil
	}
	cp, err := s.orch.chJobs().SessionRuns.GetCheckpoint(ctx, sessionRunID)
	if err != nil || cp.ID == "" {
		s.orch.runs.FinishPlaceholder(run.SessionID)
		if err := s.orch.chJobs().SessionRuns.ClearResumeClaim(ctx, sessionRunID); err != nil {
			s.lg.Warn("durable resume: clear claim failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
		}
		return err
	}
	payload, err := biz.ParseDurableCheckpointPayload(cp.PayloadJSON)
	if err != nil {
		s.orch.runs.FinishPlaceholder(run.SessionID)
		if err := s.orch.chJobs().SessionRuns.ClearResumeClaim(ctx, sessionRunID); err != nil {
			s.lg.Warn("durable resume: clear claim failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
		}
		return err
	}
	deadline := time.Duration(biz.DefaultDurableDeadlineSec()) * time.Second
	runCtx, cancel := context.WithTimeout(context.Background(), deadline)
	safego.Go(runCtx, "session-run-durable-resume", func() {
		defer cancel()
		// P0 FR-4：清理 claim 占位——若本 goroutine 的 resume turn 已采用占位
		// 并升级为真实条目，FinishPlaceholder CAS 失配不触碰（真实条目由其
		// 自身收尾 defer Finish(runID) 负责）；若 turn 未到达注册点（早退），
		// 占位在此清除，槽位恢复空闲供 worker 下一 tick 重试。
		defer s.orch.runs.FinishPlaceholder(run.SessionID)
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
			Content:   biz.DurableResumePromptFor(payload),
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint: biz.EntryPointDurable,
				AllowQueue: false,
			},
		}
		bgCtx := event.WithSessionRunID(event.WithEnvelopeSource(runCtx, run.Source), sessionRunID)
		bgCtx = event.WithDurableResume(bgCtx, event.DurableResumeSpec{
			SessionRunID:          sessionRunID,
			TurnID:                payload.TurnID,
			UserContent:           payload.UserContent,
			AgentID:               payload.AgentID,
			RuntimeRunID:          payload.RuntimeRunID,
			TrpcInvocationID:      payload.TrpcInvocationID,
			SessionRevision:       payload.SessionRevision,
			DialogMode:            payload.DialogMode,
			Provider:              payload.Provider,
			Model:                 payload.Model,
			Gear:                  payload.Gear,
			PlaybookID:            payload.PlaybookID,
			AuthorizedStageIDs:    append([]string(nil), payload.AuthorizedStageIDs...),
			ConstraintFingerprint: payload.ConstraintFingerprint,
		})
		_, asst, turnErr := s.RunNativeTurn(bgCtx, req)
		persistCtx := context.WithoutCancel(runCtx)
		if turnErr != nil {
			// P0 FR-4/H-2：busy/queued 为瞬态冲突而非失败——此刻会话槽被并发
			// turn（用户新消息、团队轮次等）占用。清 claim 交还 durable 运行，
			// 不落 failed、不发失败通知、不转会话状态；worker 下一 tick 待会话
			// 空闲自动重试。
			if isTurnBusyError(turnErr) || errors.Is(turnErr, ErrTurnMessageQueued) {
				s.lg.Info("durable resume deferred: session busy",
					loggateway.Str("session_run_id", sessionRunID), loggateway.Str("session_id", run.SessionID))
				if err := s.orch.chJobs().SessionRuns.ClearResumeClaim(persistCtx, sessionRunID); err != nil {
					s.lg.Warn("durable resume: clear claim after busy failed", loggateway.Err(err), loggateway.Str("session_run_id", sessionRunID))
				}
				return
			}
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
