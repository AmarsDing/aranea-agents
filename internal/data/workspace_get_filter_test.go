package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

func TestChannelRepo_Get_WrongWorkspaceNotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ownerCtx := workspace.WithContext(context.Background(), "ws-owner")
	otherCtx := workspace.WithContext(context.Background(), "ws-other")

	created, err := repo.Create(ownerCtx, biz.Channel{
		ID: "ch-ws-1", Key: "key-ws-1", Name: "owned", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-owner",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.WorkspaceID != "ws-owner" {
		t.Fatalf("WorkspaceID = %q, want ws-owner", created.WorkspaceID)
	}

	if _, err := repo.Get(ownerCtx, "ch-ws-1"); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := repo.Get(otherCtx, "ch-ws-1"); err == nil {
		t.Fatal("expected not found for wrong workspace")
	}

	// Shared (empty workspace_id) visible to any tenant.
	if _, err := repo.Create(workspace.WithSystemWorkspace(context.Background()), biz.Channel{
		ID: "ch-shared", Key: "key-shared", Name: "shared", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "",
	}); err != nil {
		t.Fatalf("Create shared: %v", err)
	}
	if _, err := repo.Get(otherCtx, "ch-shared"); err != nil {
		t.Fatalf("tenant should see shared channel: %v", err)
	}
}

func TestChannelRepo_Get_SystemSeesAllAndShared(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ownerCtx := workspace.WithContext(context.Background(), "ws-sys-a")
	sysCtx := workspace.WithSystemWorkspace(context.Background())

	if _, err := repo.Create(ownerCtx, biz.Channel{
		ID: "ch-sys-owned", Key: "key-sys-owned", Name: "owned", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-sys-a",
	}); err != nil {
		t.Fatalf("Create owned: %v", err)
	}
	if _, err := repo.Create(sysCtx, biz.Channel{
		ID: "ch-sys-shared", Key: "key-sys-shared", Name: "shared", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "",
	}); err != nil {
		t.Fatalf("Create shared: %v", err)
	}

	if _, err := repo.Get(sysCtx, "ch-sys-owned"); err != nil {
		t.Fatalf("system Get owned: %v", err)
	}
	if _, err := repo.Get(sysCtx, "ch-sys-shared"); err != nil {
		t.Fatalf("system Get shared: %v", err)
	}
	tenantCtx := workspace.WithContext(context.Background(), "ws-other")
	if _, err := repo.Get(tenantCtx, "ch-sys-shared"); err != nil {
		t.Fatalf("tenant Get shared empty workspace: %v", err)
	}
}

func TestChannelRepo_List_WorkspaceFilter(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewChannelRepo(d)
	ownerCtx := workspace.WithContext(context.Background(), "ws-list-a")
	otherCtx := workspace.WithContext(context.Background(), "ws-list-b")

	if _, err := repo.Create(ownerCtx, biz.Channel{
		ID: "ch-list-a", Key: "key-list-a", Name: "a", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-list-a",
	}); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if _, err := repo.Create(otherCtx, biz.Channel{
		ID: "ch-list-b", Key: "key-list-b", Name: "b", Status: "active", Enabled: true,
		ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-list-b",
	}); err != nil {
		t.Fatalf("Create b: %v", err)
	}

	listA, err := repo.List(ownerCtx)
	if err != nil {
		t.Fatalf("List a: %v", err)
	}
	for _, c := range listA {
		if c.WorkspaceID != "" && c.WorkspaceID != "ws-list-a" {
			t.Fatalf("List leaked workspace %q", c.WorkspaceID)
		}
	}
}

func TestTaskV2Repo_GetTask_WrongWorkspaceNotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTaskV2Repo(d, loggateway.NewNoop())
	now := time.Now().UTC().Truncate(time.Second)
	ownerCtx := workspace.WithContext(context.Background(), "ws-task-a")
	otherCtx := workspace.WithContext(context.Background(), "ws-task-b")

	if _, err := repo.CreateTask(ownerCtx, biz.Task{
		ID: "t-ws-1", SessionID: "s-ws", UserMessage: "hi",
		Status: biz.TaskStatusPending, Seq: 1, Version: 1,
		WorkspaceID: "ws-task-a", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if _, err := repo.GetTask(ownerCtx, "t-ws-1"); err != nil {
		t.Fatalf("owner GetTask: %v", err)
	}
	if _, err := repo.GetTask(otherCtx, "t-ws-1"); err == nil {
		t.Fatal("expected not found for wrong workspace")
	}
}

func TestCronRepo_GetCronTask_WrongWorkspaceNotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewCronRepo(d)
	ownerCtx := workspace.WithContext(context.Background(), "ws-cron-a")
	otherCtx := workspace.WithContext(context.Background(), "ws-cron-b")

	created, err := repo.CreateCronTask(ownerCtx, biz.CronTask{
		ID: "cron-ws-1", TaskKey: "cron-key-1", Name: "cron", Status: "idle",
		Enabled: true, ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-cron-a",
	})
	if err != nil {
		t.Fatalf("CreateCronTask: %v", err)
	}
	if created.ID != "cron-ws-1" {
		t.Fatalf("unexpected id %q", created.ID)
	}

	if _, err := repo.GetCronTask(ownerCtx, "cron-ws-1"); err != nil {
		t.Fatalf("owner GetCronTask: %v", err)
	}
	if _, err := repo.GetCronTask(otherCtx, "cron-ws-1"); err == nil {
		t.Fatal("expected not found for wrong workspace")
	}
}

func TestMCPServerRepo_Get_WrongWorkspaceNotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMCPServerRepo(d)
	ownerCtx := workspace.WithContext(context.Background(), "ws-mcp-a")
	otherCtx := workspace.WithContext(context.Background(), "ws-mcp-b")

	created, err := repo.CreateMCPServer(ownerCtx, biz.MCPServer{
		ID: "mcp-ws-1", Key: "mcp-key-1", Name: "mcp", Status: "active",
		Enabled: true, ConfigJSON: "{}", MetadataJSON: "{}", WorkspaceID: "ws-mcp-a",
	})
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	_ = created

	if _, err := repo.GetMCPServer(ownerCtx, "mcp-ws-1"); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := repo.GetMCPServer(otherCtx, "mcp-ws-1"); err == nil {
		t.Fatal("expected not found for wrong workspace")
	}
}
