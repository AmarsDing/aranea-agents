package biz

import (
	"context"
	"testing"
)

type turnJobRepoStub struct {
	jobs map[string]ChannelTurnJob
}

func (s *turnJobRepoStub) Create(_ context.Context, job ChannelTurnJob) (string, error) {
	key := job.ChannelID + ":" + job.IdempotencyKey
	if existing, ok := s.jobs[key]; ok {
		return existing.ID, nil
	}
	if job.ID == "" {
		job.ID = NewChannelTurnJobID()
	}
	s.jobs[key] = job
	return job.ID, nil
}

func (s *turnJobRepoStub) UpdateStatus(_ context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	for k, job := range s.jobs {
		if job.ID == id {
			job.Status = NormalizeChannelTurnJobStatus(status)
			if errMsg != "" {
				job.ErrorMessage = errMsg
			}
			s.jobs[k] = job
			return nil
		}
	}
	return nil
}

func (s *turnJobRepoStub) UpdateAsyncTarget(_ context.Context, id, targetType, targetID string) error {
	for k, job := range s.jobs {
		if job.ID == id {
			job.AsyncTargetType = targetType
			job.AsyncTargetID = targetID
			s.jobs[k] = job
			return nil
		}
	}
	return nil
}

func (s *turnJobRepoStub) TransitionIfStale(_ context.Context, id, fromStatus, toStatus, errMsg, previewMsgID, contentPreview, _ string) (bool, error) {
	for k, job := range s.jobs {
		if job.ID == id {
			if NormalizeChannelTurnJobStatus(job.Status) != NormalizeChannelTurnJobStatus(fromStatus) {
				return false, nil
			}
			job.Status = NormalizeChannelTurnJobStatus(toStatus)
			if errMsg != "" {
				job.ErrorMessage = errMsg
			}
			if previewMsgID != "" {
				job.PreviewMessageID = previewMsgID
			}
			if contentPreview != "" {
				job.ContentPreview = contentPreview
			}
			s.jobs[k] = job
			return true, nil
		}
	}
	return false, nil
}

func (s *turnJobRepoStub) GetByIdempotency(_ context.Context, channelID, idempotencyKey string) (ChannelTurnJob, error) {
	return s.jobs[channelID+":"+idempotencyKey], nil
}
func (s *turnJobRepoStub) GetByID(_ context.Context, id string) (ChannelTurnJob, error) {
	for _, job := range s.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return ChannelTurnJob{}, nil
}

func (s *turnJobRepoStub) ListByChannel(_ context.Context, channelID string, limit int) ([]ChannelTurnJob, error) {
	limit = NormalizeChannelTurnJobListLimit(limit)
	var out []ChannelTurnJob
	for _, job := range s.jobs {
		if job.ChannelID == channelID {
			out = append(out, job)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *turnJobRepoStub) ListFiltered(_ context.Context, q ChannelTurnJobListQuery) ([]ChannelTurnJob, error) {
	limit := NormalizeChannelTurnJobListLimit(q.Limit)
	var out []ChannelTurnJob
	for _, job := range s.jobs {
		if q.SessionID != "" && job.SessionID != q.SessionID {
			continue
		}
		if q.Status != "" && NormalizeChannelTurnJobStatus(job.Status) != NormalizeChannelTurnJobStatus(q.Status) {
			continue
		}
		out = append(out, job)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *turnJobRepoStub) ListActiveBySession(_ context.Context, channelID, sessionID string) ([]ChannelTurnJob, error) {
	var out []ChannelTurnJob
	for _, job := range s.jobs {
		if job.ChannelID != channelID {
			continue
		}
		if sessionID != "" && job.SessionID != sessionID {
			continue
		}
		if IsChannelTurnJobTerminalStatus(job.Status) {
			continue
		}
		out = append(out, job)
	}
	return out, nil
}

func (s *turnJobRepoStub) ListStaleByStatus(_ context.Context, status, beforeUpdatedAt string, limit int) ([]ChannelTurnJob, error) {
	return nil, nil
}

func TestChannelTurnJobUsecaseCreateAcceptedReturnsStableID(t *testing.T) {
	repo := &turnJobRepoStub{jobs: map[string]ChannelTurnJob{}}
	uc := NewChannelTurnJobUsecase(nil, repo)

	id1, err := uc.CreateAccepted(context.Background(), ChannelTurnJob{
		ID:             "a",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem",
	})
	if err != nil || id1 != "a" {
		t.Fatalf("first create: id=%q err=%v", id1, err)
	}
	id2, err := uc.CreateAccepted(context.Background(), ChannelTurnJob{
		ID:             "b",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem",
	})
	if err != nil || id2 != "a" {
		t.Fatalf("second create: id=%q err=%v", id2, err)
	}
}

func TestChannelTurnJobUsecaseCancelRunningForSession(t *testing.T) {
	repo := &turnJobRepoStub{jobs: map[string]ChannelTurnJob{
		"ch-1:k1": {ID: "j1", ChannelID: "ch-1", SessionID: "sess-1", Status: ChannelTurnJobStatusRunning, IdempotencyKey: "k1"},
	}}
	uc := NewChannelTurnJobUsecase(nil, repo)
	if err := uc.CancelRunningForSession(context.Background(), "ch-1", "sess-1"); err != nil {
		t.Fatal(err)
	}
	if repo.jobs["ch-1:k1"].Status != ChannelTurnJobStatusCancelled {
		t.Fatalf("status=%q", repo.jobs["ch-1:k1"].Status)
	}
}

func TestNormalizeChannelTurnJobListLimit(t *testing.T) {
	if got := NormalizeChannelTurnJobListLimit(0); got != 50 {
		t.Fatalf("default limit = %d", got)
	}
	if got := NormalizeChannelTurnJobListLimit(999); got != MaxChannelTurnJobListLimit {
		t.Fatalf("cap limit = %d", got)
	}
}
