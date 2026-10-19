package subagent

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"testing"
	"time"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func TestNewService_EmptyStateDir(t *testing.T) {
	_, err := NewService("", nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty state dir")
	}
}

func TestNewService_NilRunner(t *testing.T) {

	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	svc.Start(context.Background())
	_, spawnErr := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "s1",
		Task:            "do something",
	})
	if spawnErr == nil {
		t.Fatal("expected error when spawning with nil runner")
	}
}

func TestService_ListForUser_NilService(t *testing.T) {
	var s *Service
	if s.ListForUser("user", trpcsubagent.ListFilter{}) != nil {
		t.Fatal("nil service should return nil")
	}
}

func TestService_GetForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.GetForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_CancelForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, _, err = svc.CancelForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_Spawn_NilService(t *testing.T) {
	var s *Service
	_, err := s.Spawn(context.Background(), SpawnRequest{})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestService_Spawn_NotStarted(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for not started")
	}
}

func TestService_Spawn_EmptyOwner(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty owner")
	}
}

func TestService_Spawn_EmptyParentSession(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty parent session")
	}
}

func TestService_Spawn_EmptyTask(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: ""})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestService_Close_NilService(t *testing.T) {
	var s *Service
	if err := s.Close(); err != nil {
		t.Fatalf("nil service Close should not error: %v", err)
	}
}

func TestService_Start_NilService(t *testing.T) {
	var s *Service
	s.Start(context.Background())
}

func TestRunRecord_PublicView_Nil(t *testing.T) {
	var r *runRecord
	if v := r.publicView(); v.ID != "" {
		t.Fatal("nil record should return zero Run")
	}
}

func TestRunRecord_Clone_Nil(t *testing.T) {
	var r *runRecord
	if c := r.clone(); c != nil {
		t.Fatal("nil clone should return nil")
	}
}

func TestRunRecord_Clone_DeepCopy(t *testing.T) {
	now := time.Now()
	r := &runRecord{
		Run: trpcsubagent.Run{
			ID:         "r1",
			StartedAt:  &now,
			FinishedAt: &now,
		},
	}
	c := r.clone()
	if c == r {
		t.Fatal("clone should be different pointer")
	}
	if c.StartedAt == r.StartedAt {
		t.Fatal("StartedAt should be deep copied")
	}
}

func TestReplyAccumulator_NilEvent(t *testing.T) {
	a := replyAccumulator{}
	a.consume(nil)
	if a.text != "" {
		t.Fatal("nil event should not produce text")
	}
}

func TestDecodeRunIDArgs_InvalidJSON(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`not json`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestDecodeRunIDArgs_EmptyID(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`{"id":""}`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}
