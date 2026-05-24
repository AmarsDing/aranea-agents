package service_test

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type chatJobsRepoStub struct {
	jobs []biz.ChannelTurnJob
}

func (s *chatJobsRepoStub) Create(context.Context, biz.ChannelTurnJob) (string, error) { return "", nil }
func (s *chatJobsRepoStub) UpdateStatus(context.Context, string, string, string, string, string) error {
	return nil
}
func (s *chatJobsRepoStub) UpdateAsyncTarget(context.Context, string, string, string) error { return nil }
func (s *chatJobsRepoStub) GetByIdempotency(context.Context, string, string) (biz.ChannelTurnJob, error) {
	return biz.ChannelTurnJob{}, nil
}
func (s *chatJobsRepoStub) ListByChannel(context.Context, string, int) ([]biz.ChannelTurnJob, error) {
	return nil, nil
}
func (s *chatJobsRepoStub) ListFiltered(_ context.Context, q biz.ChannelTurnJobListQuery) ([]biz.ChannelTurnJob, error) {
	var out []biz.ChannelTurnJob
	for _, j := range s.jobs {
		if q.SessionID != "" && j.SessionID != q.SessionID {
			continue
		}
		if q.AgentID != "" && j.AgentID != q.AgentID {
			continue
		}
		if q.Status != "" && biz.NormalizeChannelTurnJobStatus(j.Status) != biz.NormalizeChannelTurnJobStatus(q.Status) {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func TestChatService_ListChatBackgroundJobs_validation(t *testing.T) {
	svc := service.NewChatService(service.ChatOrchestratorDeps{
		ChTurn: service.ChannelTurnDeps{
			TurnJobs: biz.NewChannelTurnJobUsecase(nil, &chatJobsRepoStub{}),
		},
	})
	_, err := svc.ListChatBackgroundJobs(context.Background(), &chatv1.ListChatBackgroundJobsRequest{})
	if err == nil || !kerrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestChatService_ListChatBackgroundJobs_bySession(t *testing.T) {
	repo := &chatJobsRepoStub{jobs: []biz.ChannelTurnJob{
		{
			ID:              "job-1",
			SessionID:       "sess-1",
			AgentID:         "agent-1",
			Status:          biz.ChannelTurnJobStatusRunning,
			AsyncTargetType: "graph",
			AsyncTargetID:   "exec-1",
			GraphID:         "graph-1",
			ContentPreview:  "running analysis",
		},
	}}
	svc := service.NewChatService(service.ChatOrchestratorDeps{
		ChTurn: service.ChannelTurnDeps{
			TurnJobs: biz.NewChannelTurnJobUsecase(nil, repo),
		},
	})
	sessionID := "sess-1"
	resp, err := svc.ListChatBackgroundJobs(context.Background(), &chatv1.ListChatBackgroundJobsRequest{
		SessionId: &sessionID,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items: got %d want 1", len(resp.GetItems()))
	}
	item := resp.GetItems()[0]
	if item.GetAgentId() != "agent-1" {
		t.Fatalf("agent_id: got %q", item.GetAgentId())
	}
	if item.GetGraphId() != "graph-1" {
		t.Fatalf("graph_id: got %q", item.GetGraphId())
	}
	if item.GetTargetId() != "exec-1" {
		t.Fatalf("target_id: got %q", item.GetTargetId())
	}
}

func TestChatService_ListChatBackgroundJobs_sanitizesInvalidUTF8Summary(t *testing.T) {
	repo := &chatJobsRepoStub{jobs: []biz.ChannelTurnJob{
		{
			ID:             "job-bad",
			SessionID:      "sess-1",
			Status:         biz.ChannelTurnJobStatusCompleted,
			ContentPreview: "ok\xffpreview",
		},
	}}
	svc := service.NewChatService(service.ChatOrchestratorDeps{
		ChTurn: service.ChannelTurnDeps{
			TurnJobs: biz.NewChannelTurnJobUsecase(nil, repo),
		},
	})
	sessionID := "sess-1"
	resp, err := svc.ListChatBackgroundJobs(context.Background(), &chatv1.ListChatBackgroundJobsRequest{
		SessionId: &sessionID,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items: got %d want 1", len(resp.GetItems()))
	}
	if got := resp.GetItems()[0].GetSummary(); got != "okpreview" {
		t.Fatalf("summary: got %q want okpreview", got)
	}
}
