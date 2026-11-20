package biz

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

// 2026-11-19 Agent 管理审查：创建入口的标识 / 治理岗校验、A2A 类型落库、复制归属、治理岗身份不可改。

func TestAgentCreate_RejectsBadKeyAndGovernanceVariant(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]Agent{
		"underscore_key":       {AgentKey: "my_agent", DisplayName: "x"},
		"spoofed_dept_lead":    {AgentKey: "__dept_lead_sales__", DisplayName: "x"},
		"dept_lead_variant":    {AgentKey: "fake-lead", DisplayName: "x", AgentVariant: "dept_lead"},
		"company_lead_variant": {AgentKey: "fake-ceo", DisplayName: "x", AgentVariant: "company_lead"},
	} {
		in := in
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{}
			_, err := newAgentUsecaseWithExt(repo).Create(context.Background(), in)
			if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeBadRequest {
				t.Fatalf("want BadRequest, got %v", err)
			}
			if repo.createCalls != 0 {
				t.Fatalf("CreateAgent must not be called")
			}
		})
	}
}

func TestAgentCreate_PersistsA2AProxyKind(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{}
	_, _ = newAgentUsecaseWithExt(repo).Create(context.Background(), Agent{
		ID:          "a2a-1",
		AgentKey:    "remote-helper",
		DisplayName: "Remote",
		AgentKind:   AgentKindA2AProxy,
		A2AProxy:    &A2AProxyConfig{RemoteURL: "https://peer.example/a2a"},
	})
	if repo.createCalls != 1 {
		t.Fatalf("want 1 create call, got %d", repo.createCalls)
	}
	stored := repo.createdAgent.ConfigJSON
	if agentKindFromConfigJSON(stored) != AgentKindA2AProxy {
		t.Fatalf("agent_kind lost from persisted config_json: %s", stored)
	}
	if p := a2aProxyFromConfigJSON(stored); p == nil || p.RemoteURL != "https://peer.example/a2a" {
		t.Fatalf("a2a_proxy lost from persisted config_json: %s", stored)
	}
}

func TestAgentDuplicate_CopyBelongsToCaller(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{stubAgentRepo: stubAgentRepo{agent: Agent{
		ID: "src", AgentKey: "__memory__", DisplayName: "记忆管家", Kind: "system_builtin", Source: "system", Readonly: true,
	}}}
	_, _ = newAgentUsecaseWithExt(repo).Duplicate(context.Background(), "src")
	got := repo.createdAgent
	if got.Kind != "user" || got.Source != "user" || got.Readonly {
		t.Fatalf("copy must be a user agent, got kind=%q source=%q readonly=%v", got.Kind, got.Source, got.Readonly)
	}
	if !strings.HasPrefix(got.AgentKey, "memory-copy-") {
		t.Fatalf("unexpected copy key %q", got.AgentKey)
	}
}

func TestAgentUpdate_GovernanceVariantIsImmutable(t *testing.T) {
	t.Parallel()
	lead := Agent{ID: "lead", AgentKey: "__dept_lead_sales__", DisplayName: "主管", AgentVariant: "dept_lead", Status: "active"}
	spec := Agent{ID: "spec", AgentKey: "seller", DisplayName: "销售", AgentVariant: "general", Status: "active"}
	for name, tc := range map[string]struct{ cur, patch Agent }{
		"promote_to_lead": {spec, Agent{AgentKey: "seller", DisplayName: "销售", AgentVariant: "dept_lead"}},
		"demote_lead":     {lead, Agent{AgentKey: "__dept_lead_sales__", DisplayName: "主管", AgentVariant: "general"}},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{stubAgentRepo: stubAgentRepo{agent: tc.cur}}
			_, err := newAgentUsecaseWithExt(repo).Update(context.Background(), tc.cur.ID, tc.patch)
			if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeBadRequest {
				t.Fatalf("want BadRequest, got %v", err)
			}
			if repo.updateCalls != 0 {
				t.Fatalf("UpdateAgent must not be called")
			}
		})
	}
}
