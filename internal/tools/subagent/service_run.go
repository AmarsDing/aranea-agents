package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func (s *Service) runChild(
	ctx context.Context,
	record *runRecord,
	started runningRun,
	result *replyAccumulator,
	usage *usageAccum,
	guard *runBudgetGuard,
) error {
	if record == nil {
		return apierror.Internal(apierror.DomainSubagent, "nil run record")
	}
	runtimeState := map[string]any{
		runtimeStateSubagentRun:      true,
		runtimeStateSubagentRunID:    record.ID,
		runtimeStateSubagentParentID: record.ParentSessionID,
	}

	runOpts := []trpcagent.RunOption{
		trpcagent.WithRequestID(started.requestID),
		trpcagent.WithRuntimeState(runtimeState),
		trpcagent.WithInjectedContextMessages([]trpcmodel.Message{
			trpcmodel.NewSystemMessage(kindSystemPrompt(record.Kind)),
		}),

		trpcagent.WithToolCallArgumentsJSONRepairEnabled(true),
		trpcagent.WithToolCallTextRepairEnabled(true),
	}

	evts, err := s.runner.Run(
		ctx,
		record.OwnerUserID,
		started.childSession,
		trpcmodel.NewUserMessage(record.Task),
		runOpts...,
	)
	if err != nil {
		return err
	}
	for evt := range evts {
		usage.consume(evt)
		result.consume(evt)
		guard.observe(usage)
	}
	return result.err
}

func (s *Service) markRunning(
	parent context.Context,
	runID string,
	timeoutSeconds int,
) (*runRecord, context.Context, runningRun, error) {
	s.mu.Lock()
	record := s.runs[strings.TrimSpace(runID)]
	if record == nil {
		s.mu.Unlock()
		return nil, nil, runningRun{}, trpcsubagent.ErrRunNotFound
	}
	if record.Status == trpcsubagent.StatusCanceled {
		s.mu.Unlock()
		return nil, nil, runningRun{}, apierror.BadRequest(apierror.DomainSubagent, "run canceled before start")
	}

	now := s.clock()
	started := runningRun{
		startedAt:    now,
		childSession: newChildSessionID(record.ID, now),
		requestID:    newRequestID(record.ID, now),
	}

	runCtx := parent
	if runCtx == nil {
		runCtx = context.Background()
	}
	if timeoutSeconds > 0 {
		timeoutCtx, cancel := context.WithTimeout(
			runCtx,
			time.Duration(timeoutSeconds)*time.Second,
		)
		runCtx = timeoutCtx
		started.cancel = cancel
	} else {
		nextCtx, cancel := context.WithCancel(runCtx)
		runCtx = nextCtx
		started.cancel = cancel
	}

	record.Status = trpcsubagent.StatusRunning
	record.ChildSessionID = started.childSession
	record.UpdatedAt = now
	record.StartedAt = cloneTime(now)
	record.FinishedAt = nil
	record.Error = ""
	record.Summary = ""
	record.Result = ""

	s.running[record.ID] = &runningRun{
		cancel:       started.cancel,
		childSession: started.childSession,
		requestID:    started.requestID,
		startedAt:    started.startedAt,
	}
	clone := record.clone()
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		if started.cancel != nil {
			started.cancel()
		}
		s.mu.Lock()
		delete(s.running, runID)
		if current := s.runs[runID]; current != nil {
			current.Status = trpcsubagent.StatusFailed
			current.Error = err.Error()
			current.Summary = summarizeResult(current.Error, 0)
			current.UpdatedAt = now
			current.FinishedAt = cloneTime(now)
		}
		s.mu.Unlock()
		return nil, nil, runningRun{}, apierror.Internal(apierror.DomainSubagent, "persist: "+err.Error())
	}
	s.emitModeBStarted(parent, ModeBStartedInfo{
		RunID:           clone.ID,
		ParentSessionID: clone.ParentSessionID,
		ChildSessionID:  started.childSession,
		Task:            clone.Task,
	})
	return clone, runCtx, started, nil
}

func (s *Service) emitModeBStarted(ctx context.Context, info ModeBStartedInfo) {
	if s == nil {
		return
	}
	s.mu.RLock()
	hook := s.onModeBStarted
	s.mu.RUnlock()
	if hook == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			s.lg.Warn("mode B started hook panic",
				loggateway.Str("run_id", info.RunID),
				loggateway.Str("panic", fmt.Sprint(r)),
			)
		}
	}()
	hook(ctx, info)
}

func (s *Service) emitModeBFinished(ctx context.Context, info ModeBFinishedInfo) {
	if s == nil {
		return
	}
	s.mu.RLock()
	hook := s.onModeBFinished
	s.mu.RUnlock()
	if hook == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			s.lg.Warn("mode B finished hook panic",
				loggateway.Str("run_id", info.RunID),
				loggateway.Str("panic", fmt.Sprint(r)),
			)
		}
	}()
	hook(ctx, info)
}

func (s *Service) finishRun(runID string, output string, runErr error, summaryRunes int) {
	s.mu.Lock()
	record := s.runs[runID]
	if record == nil {
		delete(s.running, runID)
		s.mu.Unlock()
		return
	}
	now := s.clock()
	running := s.running[runID]
	delete(s.running, runID)

	record.Result = output
	record.UpdatedAt = now
	record.FinishedAt = cloneTime(now)

	switch {
	case running != nil && running.cancelRequested:
		record.Status = trpcsubagent.StatusCanceled
		record.Error = ""

		reason := strings.TrimSpace(running.cancelReason)
		if reason == "" {
			reason = "canceled"
		}
		record.Summary = summarizeResult(reason, summaryRunes)
	case errors.Is(runErr, context.Canceled):
		record.Status = trpcsubagent.StatusCanceled
		record.Error = ""
		record.Summary = summarizeResult("canceled", summaryRunes)
	case runErr != nil:
		record.Status = trpcsubagent.StatusFailed
		record.Error = runErr.Error()
		record.Summary = summarizeResult(record.Error, summaryRunes)
	default:
		record.Status = trpcsubagent.StatusCompleted
		record.Error = ""
		record.Summary = summarizeResult(output, summaryRunes)
	}
	s.mu.Unlock()

	if err := s.persist(); err != nil {
		s.lg.Warn("subagent.finishRun",
			loggateway.StepID("tool.subagent_persist_fail"),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}

	finishedStatus := "completed"
	switch record.Status {
	case trpcsubagent.StatusFailed:
		finishedStatus = "failed"
	case trpcsubagent.StatusCanceled:
		finishedStatus = "cancelled"
	}
	s.emitModeBFinished(context.Background(), ModeBFinishedInfo{
		RunID:          record.ID,
		ChildSessionID: record.ChildSessionID,
		Status:         finishedStatus,
		Error:          record.Error,
	})

	s.mu.Lock()
	router := s.outboundRouter
	cloned := record.clone()
	s.mu.Unlock()
	if router != nil && cloned != nil && cloned.Status == trpcsubagent.StatusCompleted {
		s.doNotifyCompletion(router, cloned)
	}
}

func (s *Service) doNotifyCompletion(router *outbound.Router, record *runRecord) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeoutSec*time.Second)
	defer cancel()
	summary := record.Summary
	if summary == "" {
		summary = record.Result
	}
	if len(summary) > notifyMaxSummaryRunes {
		summary = truncateRunes(summary, notifyMaxSummaryRunes) + "..."
	}
	text := fmt.Sprintf("子 Agent 任务完成: %s\n摘要: %s", record.Task, summary)

	target := outbound.DeliveryTarget{}
	if dt, ok := outbound.ResolveTargetFromSessionID(record.ParentSessionID); ok {
		target = dt
	}
	if target.Channel == "" {
		return
	}
	if err := router.SendText(ctx, target, text); err != nil {
		s.lg.Warn("SubAgent completion notification failed",
			loggateway.StepID("subagent.notify.fail"),
			loggateway.Str("run_id", record.ID),
			loggateway.Err(err),
		)
	}
}
