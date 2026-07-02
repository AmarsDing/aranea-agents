package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// injectTeamRunLookupLimit bounds the look-back window when searching for the
// latest active or paused team run for inject. A team typically has at most
// one active run; 10 is a safety bound against runaway scans.
const injectTeamRunLookupLimit = 10

// PauseTeamRun pauses a running team run (running → paused).
//
// MVP implementation:
//  1. Validate team run is in 'running' status (CAS via state machine).
//  2. Cancel the active runner via RunRegistryPort.Cancel — this stops the
//     in-flight member step. (trpc-agent-go does not natively support
//     framework-level pause; cancel + state marker is the pragmatic MVP.)
//  3. Transition TeamRun status: running → paused (CAS, state-machine validated).
//  4. Publish run_status notice with status=paused so the frontend TeamCard
//     re-renders with the paused affordance (resume / inject).
//
// Note: true "resume from checkpoint" semantics require trpc-agent-go native
// pause support, which is not yet available. UnpauseTeamRun therefore relies
// on InjectTeamMessage to re-trigger execution with new input.
func (s *TeamService) PauseTeamRun(ctx context.Context, req *v1.PauseTeamRunRequest) (*v1.PauseTeamRunResponse, error) {
	runID := strings.TrimSpace(req.GetId())
	if runID == "" {
		return nil, apierror.BadRequest("TEAM", "id is required")
	}
	run, err := s.uc.GetRun(ctx, runID)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if run.Status != biz.TeamRunStatusRunning {
		return nil, apierror.Conflict("TEAM", "team run is not running (current status: %s); pause requires running state", run.Status)
	}

	// Cancel the active runner first so the in-flight member step stops.
	// We swallow the "no active runner" case because the team may have been
	// paused between steps (runner already finished).
	if s.runs != nil && strings.TrimSpace(run.SessionID) != "" {
		if cancelled, _ := s.runs.Cancel(run.SessionID, "team_pause"); cancelled {
			s.lg.Info("team run paused: active runner cancelled",
				loggateway.StepID("team.pause"),
				loggateway.Str("run_id", run.ID),
				loggateway.Str("session_id", run.SessionID),
			)
		}
	}

	// Transition DB status: running → paused (CAS, state-machine validated).
	updated, err := s.uc.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusPaused)
	if err != nil {
		return nil, mapTeamErr(err)
	}

	// Publish run_status notice (paused) so the frontend re-renders the
	// TeamCard tail with the resume / inject affordance.
	if s.eventBus != nil && strings.TrimSpace(updated.SessionID) != "" {
		PublishRunStatus(s.eventBus, updated.SessionID, updated.ID, "paused", "")
	}

	return &v1.PauseTeamRunResponse{
		RunId:  updated.ID,
		Status: updated.Status,
	}, nil
}

// UnpauseTeamRun resumes a paused team run (paused → running).
//
// MVP implementation:
//  1. Validate team run is in 'paused' status (CAS via state machine).
//  2. Transition TeamRun status: paused → running (CAS).
//  3. Publish run_status notice with status=running.
//
// Note: MVP does NOT automatically re-trigger execution of remaining steps.
// The user must call InjectTeamMessage to send a new instruction that
// resumes execution from the next step boundary. True "resume from
// checkpoint" requires trpc-agent-go native pause support (future work).
func (s *TeamService) UnpauseTeamRun(ctx context.Context, req *v1.UnpauseTeamRunRequest) (*v1.UnpauseTeamRunResponse, error) {
	runID := strings.TrimSpace(req.GetId())
	if runID == "" {
		return nil, apierror.BadRequest("TEAM", "id is required")
	}
	run, err := s.uc.GetRun(ctx, runID)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	if run.Status != biz.TeamRunStatusPaused {
		return nil, apierror.Conflict("TEAM", "team run is not paused (current status: %s); unpause requires paused state", run.Status)
	}

	// Transition DB status: paused → running (CAS, state-machine validated).
	updated, err := s.uc.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusRunning)
	if err != nil {
		return nil, mapTeamErr(err)
	}

	// Publish run_status notice (running) so the frontend re-renders the
	// TeamCard tail with the pause affordance.
	if s.eventBus != nil && strings.TrimSpace(updated.SessionID) != "" {
		PublishRunStatus(s.eventBus, updated.SessionID, updated.ID, "running", "")
	}

	return &v1.UnpauseTeamRunResponse{
		RunId:  updated.ID,
		Status: updated.Status,
	}, nil
}

// InjectTeamMessage injects a user message into the active or paused team
// run's pending queue. The message is processed at the next step boundary.
//
// Lookup flow:
//  1. Validate team_id and message.
//  2. List recent runs for the team (limit 10) and find the latest one in
//     'running' or 'paused' status.
//  3. Call RunRegistryPort.EnqueueUserMessage(run.SessionID, message).
//     Returns accepted=true if the runner accepted the message; returns
//     accepted=false (no error) if no active runner exists.
func (s *TeamService) InjectTeamMessage(ctx context.Context, req *v1.InjectTeamMessageRequest) (*v1.InjectTeamMessageResponse, error) {
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team_id is required")
	}
	message := strings.TrimSpace(req.GetMessage())
	if message == "" {
		return nil, apierror.BadRequest("TEAM", "message is required")
	}
	if s.runs == nil {
		return nil, apierror.Internal("TEAM", "run registry not configured")
	}

	// Verify team exists.
	if _, err := s.uc.Get(ctx, teamID); err != nil {
		return nil, mapTeamErr(err)
	}

	// Find the latest running or paused run for this team.
	runs, err := s.uc.ListRuns(ctx, teamID, injectTeamRunLookupLimit)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	var target *biz.TeamRunRecord
	for i := range runs {
		r := &runs[i]
		if r.Status != biz.TeamRunStatusRunning && r.Status != biz.TeamRunStatusPaused {
			continue
		}
		if target == nil || r.CreatedAt > target.CreatedAt {
			target = r
		}
	}
	if target == nil {
		return nil, apierror.Conflict("TEAM", "team %s has no active or paused run; start a run before injecting", teamID)
	}
	if strings.TrimSpace(target.SessionID) == "" {
		return nil, apierror.Internal("TEAM", "team run %s has no session_id; cannot enqueue", target.ID)
	}

	accepted, err := s.runs.EnqueueUserMessage(target.SessionID, message)
	if err != nil {
		return nil, apierror.Internal("TEAM", "enqueue inject failed").WithCause(err)
	}

	s.lg.Info("team run inject enqueued",
		loggateway.StepID("team.inject"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("run_id", target.ID),
		loggateway.Str("session_id", target.SessionID),
		loggateway.Bool("accepted", accepted),
	)

	return &v1.InjectTeamMessageResponse{
		Accepted: accepted,
		Queued:   accepted,
	}, nil
}
