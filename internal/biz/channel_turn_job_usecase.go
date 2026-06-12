package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

var errChannelTurnJobNotInit = apierror.Internal("CHANNEL_TURN_JOB", "usecase not initialized")

type ChannelTurnJobUsecase struct {
	channels *ChannelUsecase
	jobs     ChannelTurnJobRepo
}

func NewChannelTurnJobUsecase(channels *ChannelUsecase, jobs ChannelTurnJobRepo) *ChannelTurnJobUsecase {
	return &ChannelTurnJobUsecase{channels: channels, jobs: jobs}
}

func (u *ChannelTurnJobUsecase) ListByChannel(ctx context.Context, channelID string, limit int) ([]ChannelTurnJob, error) {
	if u == nil || u.jobs == nil {
		return nil, errChannelTurnJobNotInit
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, apierror.BadRequest("CHANNEL_TURN_JOB", "channel_id is required")
	}
	if u.channels != nil {
		if _, err := u.channels.Get(ctx, channelID); err != nil {
			return nil, err
		}
	}
	return u.jobs.ListByChannel(ctx, channelID, NormalizeChannelTurnJobListLimit(limit))
}

func (u *ChannelTurnJobUsecase) ListFiltered(ctx context.Context, q ChannelTurnJobListQuery) ([]ChannelTurnJob, error) {
	if u == nil || u.jobs == nil {
		return nil, errChannelTurnJobNotInit
	}
	q.SessionID = strings.TrimSpace(q.SessionID)
	q.AgentID = strings.TrimSpace(q.AgentID)
	q.Status = strings.TrimSpace(q.Status)
	return u.jobs.ListFiltered(ctx, q)
}

func (u *ChannelTurnJobUsecase) CreateAccepted(ctx context.Context, job ChannelTurnJob) (string, error) {
	if u == nil || u.jobs == nil {
		return "", errChannelTurnJobNotInit
	}
	job.Status = ChannelTurnJobStatusAccepted
	return u.jobs.Create(ctx, job)
}

// TransitionByEvent validates the state transition via the state machine and updates the job.
// This is the preferred way to change job status — callers specify the event, not the target status.
func (u *ChannelTurnJobUsecase) TransitionByEvent(ctx context.Context, id, event, errMsg, previewMsgID, contentPreview string) error {
	if u == nil || u.jobs == nil {
		return errChannelTurnJobNotInit
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	job, err := u.jobs.GetByID(ctx, id)
	if err != nil {
		return apierror.NotFound("CHANNEL_TURN_JOB", "job not found: "+id)
	}
	newStatus, err := TransitionChannelTurnJob(job.Status, event)
	if err != nil {
		return err
	}
	return u.jobs.UpdateStatus(ctx, id, newStatus, errMsg, previewMsgID, contentPreview)
}

// updateStatus updates the job status directly without state machine validation.
// Internal/admin path only — production code should use TransitionByEvent.
// Unexported to prevent callers from bypassing the state machine.
func (u *ChannelTurnJobUsecase) updateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	if u == nil || u.jobs == nil {
		return errChannelTurnJobNotInit
	}
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return u.jobs.UpdateStatus(ctx, id, status, errMsg, previewMsgID, contentPreview)
}

func (u *ChannelTurnJobUsecase) UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error {
	if u == nil || u.jobs == nil {
		return errChannelTurnJobNotInit
	}
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return u.jobs.UpdateAsyncTarget(ctx, id, targetType, targetID)
}

// CancelRunningForSession cancels all active (non-terminal) jobs for a given session.
// Uses TransitionByEvent to ensure atomic state machine validation + update.
func (u *ChannelTurnJobUsecase) CancelRunningForSession(ctx context.Context, channelID, sessionID string) error {
	if u == nil || u.jobs == nil {
		return errChannelTurnJobNotInit
	}
	channelID = strings.TrimSpace(channelID)
	sessionID = strings.TrimSpace(sessionID)
	if channelID == "" {
		return nil
	}
	jobs, err := u.jobs.ListActiveBySession(ctx, channelID, sessionID)
	if err != nil {
		return err
	}
	var firstErr error
	for _, job := range jobs {
		if err := u.TransitionByEvent(ctx, job.ID, JobEventCancel, "cancelled by session cleanup", "", ""); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Cancel cancels a specific job by ID using the state machine.
func (u *ChannelTurnJobUsecase) Cancel(ctx context.Context, id string) error {
	if u == nil || u.jobs == nil {
		return errChannelTurnJobNotInit
	}
	return u.TransitionByEvent(ctx, id, JobEventCancel, "cancelled by user", "", "")
}
