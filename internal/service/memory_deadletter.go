package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func (s *MemoryService) ListMemoryDeadLetters(ctx context.Context, req *v1.ListMemoryDeadLettersRequest) (*v1.ListMemoryDeadLettersResponse, error) {
	if s.deadLetterRepo == nil {
		return nil, apierror.Internal("MEMORY", "dead-letter repo not wired")
	}
	lim := int(req.GetLimit())
	if lim <= 0 {
		lim = 100
	}
	entries, err := s.deadLetterRepo.ListDeadLetters(ctx, req.GetState(), lim)
	if err != nil {
		return nil, err
	}
	out := &v1.ListMemoryDeadLettersResponse{}
	for _, e := range entries {
		out.Items = append(out.Items, bizDeadLetterEntryToProto(e))
	}
	return out, nil
}

func (s *MemoryService) ReplayMemoryDeadLetter(ctx context.Context, req *v1.ReplayMemoryDeadLetterRequest) (*v1.ReplayMemoryDeadLetterResponse, error) {
	if s.deadLetterRepo == nil {
		return nil, apierror.Internal("MEMORY", "dead-letter repo not wired")
	}
	id := req.GetId()
	if id <= 0 {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	if s.deadLetterEnqueue != nil {
		if err := s.deadLetterEnqueue(ctx, id); err != nil {
			return nil, err
		}
	} else {
		if err := s.deadLetterRepo.MarkDeadLetterReplayed(ctx, id); err != nil {
			return nil, err
		}
	}
	// Read back the persisted row; a replay/abandon that "succeeds" but
	// cannot be read back is an inconsistency the caller must see, not a
	// synthetic entry fabricated by the service.
	e, err := s.deadLetterRepo.GetDeadLetter(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.ReplayMemoryDeadLetterResponse{Entry: bizDeadLetterEntryToProto(e)}, nil
}

func (s *MemoryService) AbandonMemoryDeadLetter(ctx context.Context, req *v1.AbandonMemoryDeadLetterRequest) (*v1.AbandonMemoryDeadLetterResponse, error) {
	if s.deadLetterRepo == nil {
		return nil, apierror.Internal("MEMORY", "dead-letter repo not wired")
	}
	id := req.GetId()
	if id <= 0 {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	if err := s.deadLetterRepo.MarkDeadLetterAbandoned(ctx, id, req.GetReason()); err != nil {
		return nil, err
	}
	e, err := s.deadLetterRepo.GetDeadLetter(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.AbandonMemoryDeadLetterResponse{Entry: bizDeadLetterEntryToProto(e)}, nil
}

func bizDeadLetterEntryToProto(e biz.MemoryDeadLetterEntry) *v1.MemoryDeadLetterEntry {
	return &v1.MemoryDeadLetterEntry{
		Id:         e.ID,
		EnqueuedAt: e.EnqueuedAt.Format(time.RFC3339),
		FailedAt:   e.FailedAt.Format(time.RFC3339),
		SessionId:  e.SessionID,
		AppName:    e.AppName,
		DropReason: e.DropReason,
		Priority:   int32(e.Priority),
		Attempts:   int32(e.Attempts),
		State:      e.State,
		LastError:  e.LastError,
	}
}
