package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// SubmitClarification handles user submission of clarification answers.
// It loads the clarify step, validates kind=clarify + status=awaiting_input,
// updates the step with answers, publishes system.notice, and resumes the turn.
func (s *ChatService) SubmitClarification(ctx context.Context, req *chatv1.SubmitClarificationRequest) (*chatv1.SubmitClarificationResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "service unavailable")
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	stepID := strings.TrimSpace(req.GetStepId())
	if sessionID == "" || stepID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id and step_id are required")
	}

	stepReader := s.orch.stepReader()
	stepWriter := s.orch.stepWriter()
	if stepReader == nil || stepWriter == nil {
		return nil, apierror.Internal(apierror.DomainChat, "step store unavailable")
	}

	step, err := stepReader.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}

	if step.Kind != biz.StepKindClarify {
		return nil, apierror.BadRequest(apierror.DomainChat, "expected clarify kind, got %s", step.Kind)
	}
	if step.Status != biz.StepStatusAwaitingInput {
		return nil, apierror.Conflict(apierror.DomainChat, "clarification already submitted (current status: %s)", step.Status)
	}
	if step.SessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "step does not belong to session %s", sessionID)
	}

	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for clarification")
	}

	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound(apierror.DomainChat, "session not found")
		}
		if session.UserID != userID {
			s.lg.Warn("submit clarification ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", stepID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can submit clarification")
		}
	} else {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}

	// Parse the clarification envelope from step content.
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		return nil, apierror.Internal(apierror.DomainChat, "failed to parse clarification envelope: %v", err)
	}

	// Convert proto answers to biz answers.
	answers := make([]biz.ClarificationAnswer, len(req.GetAnswers()))
	for i, a := range req.GetAnswers() {
		answers[i] = biz.ClarificationAnswer{
			Selected: a.GetSelected(),
			Other:    strings.TrimSpace(a.GetOther()),
		}
	}
	envelope.Answers = answers

	// Build clarified context for turn resumption.
	clarifiedContext := envelope.BuildClarifiedContext()

	// Update step: awaiting_input → completed, write back answers.
	now := time.Now().UTC()
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	updatedContent, err := json.Marshal(envelope)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainChat, "failed to marshal clarification envelope: %v", err)
	}
	step.Content = string(updatedContent)

	if _, err := stepWriter.UpdateStep(ctx, step); err != nil {
		return nil, err
	}

	// Publish step updated event.
	if s.orch.v2Seq != nil {
		s.orch.v2Seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
	}

	// Publish system.notice for clarification submission.
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		meta := map[string]any{
			"step_id":  step.ID,
			"task_id":  step.TaskID,
			"status":   string(step.Status),
			"kind":     string(step.Kind),
			"answered": len(answers),
			"total":    len(envelope.Questions),
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(step.SessionID, "clarification_submitted", "", meta))
	}

	// Transition session back to running.
	s.orch.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")

	// Resume the turn with clarified context.
	// The clarified context is injected as a user-perspective message.
	resumeErr := s.orch.resumeTurnWithClarification(ctx, sessionID, step.TaskID, clarifiedContext)
	if resumeErr != nil {
		s.lg.Warn("failed to resume turn with clarification",
			loggateway.Str("session_id", sessionID),
			loggateway.Str("step_id", stepID),
			loggateway.Err(resumeErr),
		)
		// Non-fatal: the step is already marked completed, so the user can
		// continue by sending a new message.
	}

	return &chatv1.SubmitClarificationResponse{
		Accepted:         true,
		Status:           string(step.Status),
		ClarifiedContext: clarifiedContext,
	}, nil
}

// resumeTurnWithClarification resumes a paused turn after the user submits
// clarification answers. It loads the pending clarification state, injects the
// clarified context into the original input, and executes the turn.
func (o *ChatOrchestrator) resumeTurnWithClarification(ctx context.Context, sessionID, taskID, clarifiedContext string) error {
	// Load the pending clarification state.
	v, ok := o.pendingClarifications.Load(sessionID)
	if !ok {
		return apierror.NotFound(apierror.DomainChat, "no pending clarification for session %s", sessionID)
	}
	pc := v.(pendingClarification)

	// Verify task ID matches.
	if pc.TaskID != taskID {
		return apierror.BadRequest(apierror.DomainChat, "task ID mismatch: expected %s, got %s", pc.TaskID, taskID)
	}

	// Clean up the pending state.
	o.pendingClarifications.Delete(sessionID)

	// Inject the clarified context into the original input.
	// The clarified context is prepended to the original content as a user-perspective message.
	input := pc.Input
	input.Content = clarifiedContext + "\n\n原始需求：" + input.Content

	o.lg().Info("resuming turn with clarification",
		loggateway.SessionID(sessionID),
		loggateway.Str("task_id", taskID),
		loggateway.Str("step_id", pc.StepID),
		loggateway.Int("clarified_context_len", len(clarifiedContext)),
	)

	// Execute the turn with the clarified input.
	// Use a background context to avoid cancellation from the HTTP request.
	turnCtx := context.Background()
	_, err := o.Execute(turnCtx, input)
	return err
}
