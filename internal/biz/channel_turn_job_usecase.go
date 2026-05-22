package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// ChannelTurnJobUsecase owns channel turn job persistence for ingress and admin API.
type ChannelTurnJobUsecase struct {
	channels *ChannelUsecase
	jobs     ChannelTurnJobRepo
}

func NewChannelTurnJobUsecase(channels *ChannelUsecase, jobs ChannelTurnJobRepo) *ChannelTurnJobUsecase {
	return &ChannelTurnJobUsecase{channels: channels, jobs: jobs}
}

func (u *ChannelTurnJobUsecase) ListByChannel(ctx context.Context, channelID string, limit int) ([]ChannelTurnJob, error) {
	if u == nil || u.jobs == nil {
		return nil, nil
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.BadRequest("CHANNEL_TURN_JOB", "channel_id is required")
	}
	if u.channels != nil {
		if _, err := u.channels.Get(ctx, channelID); err != nil {
			return nil, err
		}
	}
	return u.jobs.ListByChannel(ctx, channelID, NormalizeChannelTurnJobListLimit(limit))
}

// CreateAccepted inserts or resolves an accepted job and returns the persisted row id.
func (u *ChannelTurnJobUsecase) CreateAccepted(ctx context.Context, job ChannelTurnJob) (string, error) {
	if u == nil || u.jobs == nil {
		return "", nil
	}
	job.Status = ChannelTurnJobStatusAccepted
	return u.jobs.Create(ctx, job)
}

func (u *ChannelTurnJobUsecase) UpdateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	if u == nil || u.jobs == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	return u.jobs.UpdateStatus(ctx, id, status, errMsg, previewMsgID, contentPreview)
}

func (u *ChannelTurnJobUsecase) UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error {
	if u == nil || u.jobs == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	return u.jobs.UpdateAsyncTarget(ctx, id, targetType, targetID)
}

// CancelRunningForSession marks the newest running/accepted job for a session as cancelled.
func (u *ChannelTurnJobUsecase) CancelRunningForSession(ctx context.Context, channelID, sessionID string) error {
	if u == nil || u.jobs == nil {
		return nil
	}
	channelID = strings.TrimSpace(channelID)
	sessionID = strings.TrimSpace(sessionID)
	if channelID == "" {
		return nil
	}
	jobs, err := u.jobs.ListByChannel(ctx, channelID, 20)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if sessionID != "" && strings.TrimSpace(job.SessionID) != sessionID {
			continue
		}
		switch NormalizeChannelTurnJobStatus(job.Status) {
		case ChannelTurnJobStatusRunning, ChannelTurnJobStatusAccepted:
			return u.jobs.UpdateStatus(ctx, job.ID, ChannelTurnJobStatusCancelled, "", "", "")
		}
	}
	return nil
}
