package service

import (
	chatv1 "aranea-agents/api/kratos/chat/v1"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"context"
	"strings"
	"time"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// GetRunStatus returns the current run lifecycle state for a session.
func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.GetRunStatusRequest) (*chatv1.RunStatus, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if err := s.assertSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	resp := &chatv1.RunStatus{Status: "idle"}
	if runID, status, errMsg, updatedAt, ok := s.orch.GetRunStatus(ctx, sessionID); ok {
		resp.RunId = runID
		resp.Status = status
		resp.ErrorMessage = errMsg
		resp.UpdatedAt = updatedAt
	} else if snap, ok := s.orch.hydrateRunStatusFromSession(ctx, sessionID); ok {

		if strings.TrimSpace(snap.RunID) == "" {
			if terminalRunStatus(snap.Status) && runStatusSnapshotFresh(snap.UpdatedAt, rt.TerminalStatusTTL) {
				resp.Status = snap.Status
				resp.ErrorMessage = snap.ErrorMessage
				resp.UpdatedAt = snap.UpdatedAt
			} else {
				return nil, apierror.NotFound(apierror.DomainChat, "no run found for session")
			}
		} else {
			resp.RunId = snap.RunID
			resp.Status = snap.Status
			resp.ErrorMessage = snap.ErrorMessage
			resp.UpdatedAt = snap.UpdatedAt
		}

		if strings.TrimSpace(snap.Status) == "awaiting_user" {
			resp.AwaitKind = snap.AwaitKind
			resp.AwaitToolKey = snap.AwaitToolKey
			resp.AwaitToolCallId = snap.AwaitToolCallID
		}
	}
	if runnerIface, _, active := s.orch.ActiveRunner(sessionID); active {
		if runner, ok := runnerIface.(trpcrunner.Runner); ok {
			applyFrameworkRunStatus(resp, runner, sessionID)
		}
	}
	if meta := s.orch.resolveAwaitMeta(ctx, sessionID, resp.Status); strings.TrimSpace(resp.Status) == "awaiting_user" {
		resp.AwaitKind = meta.Kind
		resp.AwaitToolKey = meta.ToolKey
		resp.AwaitToolCallId = meta.ToolCallID
	}
	return resp, nil
}

func applyFrameworkRunStatus(resp *chatv1.RunStatus, runner trpcrunner.Runner, requestID string) {
	if resp == nil || runner == nil {
		return
	}
	st, ok := rt.FrameworkRunStatusFromRunner(runner, requestID)
	if !ok {
		return
	}
	resp.InvocationId = st.InvocationID
	resp.AgentName = st.AgentName
	if !st.StartedAt.IsZero() {
		resp.StartedAt = st.StartedAt.Format(time.RFC3339)
	}
	if !st.LastEventAt.IsZero() {
		resp.LastEventAt = st.LastEventAt.Format(time.RFC3339)
	}
	resp.EventCount = int32(st.EventCount)
}

// AwaitUserReply submits a human reply for a run that is paused in awaiting_user state.
func (s *ChatService) AwaitUserReply(ctx context.Context, req *chatv1.AwaitUserReplyRequest) (*chatv1.AwaitUserReplyResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	reply := strings.TrimSpace(req.GetReply())
	if reply == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "reply is required")
	}
	runID := ""
	if req.RunId != nil {
		runID = strings.TrimSpace(*req.RunId)
	}

	outcome, err := s.orch.submitAwaitReply(ctx, sessionID, awaitReply{
		runID:         runID,
		token:         reply,
		resumeContent: reply,
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.AwaitUserReplyResponse{Accepted: outcome != awaitReplyRejected}, nil
}
