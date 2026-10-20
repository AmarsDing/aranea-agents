package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/agentbridge/v1"
	"aranea-agents/internal/biz/agentbridge"
)

// ---------- M1-10 管理 API（proto 处理器） ----------

func TestAgentBridgeAPI_UpsertAgentValidation(t *testing.T) {
	e := newABSvcEnv()
	if _, err := e.api.UpsertAgent(context.Background(), &v1.UpsertAgentRequest{Command: "x"}); err == nil {
		t.Fatal("missing agent_key must error")
	}
	if _, err := e.api.UpsertAgent(context.Background(), &v1.UpsertAgentRequest{AgentKey: "x"}); err == nil {
		t.Fatal("missing command must error for unknown agent_key")
	}
}

func TestAgentBridgeAPI_UpsertAgentDefaultLaunch(t *testing.T) {
	e := newABSvcEnv()
	agent, err := e.api.UpsertAgent(context.Background(), &v1.UpsertAgentRequest{
		AgentKey: "codebuddy",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("known key may omit command: %v", err)
	}
	if agent.Command != "codebuddy" || len(agent.Args) != 1 || agent.Args[0] != "--acp" {
		t.Fatalf("codebuddy default launch = %+v", agent)
	}
}

func TestAgentBridgeAPI_UpsertAndListAgents(t *testing.T) {
	e := newABSvcEnv()
	agent, err := e.api.UpsertAgent(context.Background(), &v1.UpsertAgentRequest{
		AgentKey:    "codebuddy",
		DisplayName: "CodeBuddy",
		Command:     "codebuddy",
		Args:        []string{"--acp"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if agent.AgentKey != "codebuddy" || !agent.Enabled {
		t.Fatalf("agent = %+v", agent)
	}

	res, err := e.api.ListAgents(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].AgentKey != "codebuddy" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[0].DisplayName != "CodeBuddy" || len(res.Items[0].Args) != 1 {
		t.Fatalf("item = %+v", res.Items[0])
	}
}

func TestAgentBridgeAPI_UpsertProjectValidation(t *testing.T) {
	e := newABSvcEnv()
	if _, err := e.api.UpsertProject(context.Background(), &v1.UpsertProjectRequest{Path: `F:\x`}); err == nil {
		t.Fatal("missing name must error")
	}
	if _, err := e.api.UpsertProject(context.Background(), &v1.UpsertProjectRequest{Name: "x"}); err == nil {
		t.Fatal("missing path must error")
	}
}

func TestAgentBridgeAPI_ProjectCRUD(t *testing.T) {
	e := newABSvcEnv()
	if _, err := e.api.UpsertProject(context.Background(), &v1.UpsertProjectRequest{
		Name: "aranea", Path: `F:\aranea`, Description: "主仓",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, err := e.api.ListProjects(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "aranea" || list.Items[0].Path != `F:\aranea` {
		t.Fatalf("items = %+v", list.Items)
	}
	if list.Items[0].Description != "主仓" {
		t.Fatalf("description = %q", list.Items[0].Description)
	}
	if _, err := e.api.DeleteProject(context.Background(), &v1.DeleteProjectRequest{Id: list.Items[0].Id}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = e.api.ListProjects(context.Background(), nil)
	if len(list.Items) != 0 {
		t.Fatalf("after delete items = %+v", list.Items)
	}
}

func TestAgentBridgeAPI_ListTasksResolvesNames(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(context.Context, string, string, agentbridge.EventHandler) (string, error) {
			return "done", nil
		},
	}}
	if _, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "x"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	res, err := e.api.ListTasks(context.Background(), &v1.ListTasksRequest{SessionId: "sess-1"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(res.Items))
	}
	item := res.Items[0]
	if item.AgentKey != "codebuddy" || item.ProjectName != "aranea" {
		t.Fatalf("resolved names = %q/%q", item.AgentKey, item.ProjectName)
	}
	if item.SessionId != "sess-1" || item.Prompt != "x" {
		t.Fatalf("item = %+v", item)
	}
}

// ---------- M4 P0-1 S1（AC-11）：API 响应 env 掩码 ----------

func TestAgentBridgeAPI_EnvMasking(t *testing.T) {
	e := newABSvcEnv()
	if _, err := e.api.UpsertAgent(context.Background(), &v1.UpsertAgentRequest{
		AgentKey: "deepseek",
		Command:  `bin\acp-shim-deepseek.exe`,
		Env: map[string]string{
			"DEEPSEEK_API_KEY":     "sk-plaintext-12345",
			"AUTH_TOKEN":           "tok-plaintext",
			"WILL_NOT_LEAK_SECRET": "sec-plaintext",
			"DB_PASSWORD":          "pw-plaintext",
			"APP_CREDENTIAL":       "cred-plaintext",
			"NO_PROXY":             "127.0.0.1", // 非敏感键保留
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	res, err := e.api.ListAgents(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(res.Items))
	}
	envMap := res.Items[0].Env
	for _, k := range []string{"DEEPSEEK_API_KEY", "AUTH_TOKEN", "WILL_NOT_LEAK_SECRET", "DB_PASSWORD", "APP_CREDENTIAL"} {
		if v := envMap[k]; v != "***" {
			t.Fatalf("sensitive key %s leaked: %q", k, v)
		}
	}
	if envMap["NO_PROXY"] != "127.0.0.1" {
		t.Fatalf("non-sensitive key must keep value, got %q", envMap["NO_PROXY"])
	}
}

func TestMaskAgentEnv_Idempotent(t *testing.T) {
	in := map[string]string{"X_KEY": "abc"}
	out := maskAgentEnv(in)
	if out["X_KEY"] != "***" {
		t.Fatalf("mask = %q", out["X_KEY"])
	}
	if in["X_KEY"] != "abc" {
		t.Fatal("input map must not be mutated")
	}
	if got := maskAgentEnv(nil); got != nil {
		t.Fatalf("nil env must stay nil, got %v", got)
	}
}

func TestAgentBridgeAPI_GetAndCancelTask(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codebuddy")
	e.seedProject("aranea", `F:\aranea`)
	started := make(chan struct{})
	release := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(context.Context, string, string, agentbridge.EventHandler) (string, error) {
			close(started)
			<-release // 挂起，直到测试取消后放行
			return "", nil
		},
	}}
	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codebuddy", "aranea", "x")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-started

	got, err := e.api.GetTask(context.Background(), &v1.GetTaskRequest{Id: res.Task.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "running" {
		t.Fatalf("status = %q, want running", got.Status)
	}

	if _, err := e.api.CancelTask(context.Background(), &v1.CancelTaskRequest{Id: res.Task.ID}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	close(release)
	e.bus.waitNoticeType(t, "coding_task_cancelled", 1)
}
