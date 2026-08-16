package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// P1-2 变更分类守卫：AgentToolPolicyChange 是 service 层失效策略的唯一依据——
// StructureChanged → invalidate 重建；仅 PolicyChanged → Set resolver 零重建。
// 分类必须基于「与持久化前值的比较」而非「请求字段存在性」（前端全量提交表单）。

type policyAgentReader struct {
	AgentReader
}

func (r *policyAgentReader) GetAgentByID(_ context.Context, id string) (Agent, error) {
	if id == "missing" {
		return Agent{}, apierror.NotFound("AGENT", "not found")
	}
	return Agent{ID: id, AgentKey: "k-" + id, Status: "active"}, nil
}

type policySettingsRepo struct {
	byAgent map[string]AgentRuntimeSettings
	saved   *AgentRuntimeSettings
}

func (r *policySettingsRepo) GetAgentRuntimeSettings(_ context.Context, id string) (AgentRuntimeSettings, error) {
	if s, ok := r.byAgent[id]; ok {
		return s, nil
	}
	return AgentRuntimeSettings{}, ErrNotFound
}

func (r *policySettingsRepo) ListAgentRuntimeSettings(context.Context) (map[string]AgentRuntimeSettings, error) {
	return r.byAgent, nil
}

func (r *policySettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	cp := v
	r.saved = &cp
	return v, nil
}

type policyToolReader struct {
	ToolRegistryReader
}

func (r *policyToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	return ToolListResult{}, nil
}

func (r *policyToolReader) ListToolAgentOverridesByAgent(context.Context, string) ([]ToolAgentOverride, error) {
	return nil, nil
}

func newPolicyUsecase(settings *policySettingsRepo) *AgentUsecase {
	return NewAgentUsecase(AgentUsecaseDeps{
		Reader:   &policyAgentReader{},
		Settings: settings,
		Tools:    &policyToolReader{},
		Lg:       loggateway.NewNoop(),
	})
}

func int32p(v int32) *int32 { return &v }

func TestUpdateAgentToolPolicy_ChangeClassification(t *testing.T) {
	base := func() *policySettingsRepo {
		return &policySettingsRepo{byAgent: map[string]AgentRuntimeSettings{
			"agent-a": {
				AgentID:                  "agent-a",
				ToolsEnabled:             true,
				ToolsProfile:             "coding",
				ToolsAllowJSON:           `["read_file"]`,
				ToolsDenyJSON:            `[]`,
				ToolsExecutionTimeoutSec: 60,
			},
		}}
	}

	t.Run("仅 timeout 变化 → 仅 PolicyChanged", func(t *testing.T) {
		uc := newPolicyUsecase(base())
		_, change, err := uc.UpdateAgentToolPolicy(context.Background(), "agent-a", AgentToolPolicyInput{
			ToolsEnabled:        true,
			Profile:             "coding",
			Allow:               []string{"read_file"},
			Deny:                []string{},
			ExecutionTimeoutSec: int32p(300),
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if change.StructureChanged {
			t.Fatal("timeout-only update must not mark StructureChanged")
		}
		if !change.PolicyChanged || change.NewExecutionTimeoutSec != 300 {
			t.Fatalf("want PolicyChanged with 300, got %+v", change)
		}
	})

	t.Run("仅 allow 变化 → 仅 StructureChanged", func(t *testing.T) {
		uc := newPolicyUsecase(base())
		_, change, err := uc.UpdateAgentToolPolicy(context.Background(), "agent-a", AgentToolPolicyInput{
			ToolsEnabled:        true,
			Profile:             "coding",
			Allow:               []string{"read_file", "write_file"},
			Deny:                []string{},
			ExecutionTimeoutSec: int32p(60), // 同值全量提交，不算变化
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if !change.StructureChanged {
			t.Fatal("allow-list update must mark StructureChanged")
		}
		if change.PolicyChanged {
			t.Fatal("same-value timeout resubmit must not mark PolicyChanged")
		}
	})

	t.Run("全量同值提交 → 双 false（幂等不失效）", func(t *testing.T) {
		uc := newPolicyUsecase(base())
		_, change, err := uc.UpdateAgentToolPolicy(context.Background(), "agent-a", AgentToolPolicyInput{
			ToolsEnabled:        true,
			Profile:             "coding",
			Allow:               []string{"read_file"},
			Deny:                []string{},
			ExecutionTimeoutSec: int32p(60),
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if change.StructureChanged || change.PolicyChanged {
			t.Fatalf("idempotent resubmit must be no-op, got %+v", change)
		}
	})

	t.Run("timeout + profile 同时变化 → 双 true", func(t *testing.T) {
		uc := newPolicyUsecase(base())
		_, change, err := uc.UpdateAgentToolPolicy(context.Background(), "agent-a", AgentToolPolicyInput{
			ToolsEnabled:        true,
			Profile:             "full",
			Allow:               []string{"read_file"},
			Deny:                []string{},
			ExecutionTimeoutSec: int32p(120),
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if !change.StructureChanged || !change.PolicyChanged {
			t.Fatalf("want both changed, got %+v", change)
		}
		if change.NewExecutionTimeoutSec != 120 {
			t.Fatalf("want NewExecutionTimeoutSec=120, got %d", change.NewExecutionTimeoutSec)
		}
	})

	t.Run("ExecutionTimeoutSec 未提交（nil）→ 值保持不变", func(t *testing.T) {
		repo := base()
		uc := newPolicyUsecase(repo)
		_, change, err := uc.UpdateAgentToolPolicy(context.Background(), "agent-a", AgentToolPolicyInput{
			ToolsEnabled: true,
			Profile:      "coding",
			Allow:        []string{"read_file"},
			Deny:         []string{},
			// ExecutionTimeoutSec 为 nil：老客户端/未提交该字段
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if change.PolicyChanged {
			t.Fatal("nil ExecutionTimeoutSec must not mark PolicyChanged")
		}
		if repo.saved == nil || repo.saved.ToolsExecutionTimeoutSec != 60 {
			t.Fatalf("persisted timeout must stay 60, got %+v", repo.saved)
		}
	})
}
