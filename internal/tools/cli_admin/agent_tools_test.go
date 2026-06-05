package cli_admin

import (
	"context"
	"fmt"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockAgentRepo struct {
	listFn           func(ctx context.Context, keyword string, limit, offset int32) ([]AgentItem, int32, error)
	getFn            func(ctx context.Context, id string) (*AgentItem, error)
	getByAgentKeyFn  func(ctx context.Context, agentKey string) (*AgentItem, error)
}

func (m *mockAgentRepo) ListAgents(ctx context.Context, keyword string, limit, offset int32) ([]AgentItem, int32, error) {
	return m.listFn(ctx, keyword, limit, offset)
}

func (m *mockAgentRepo) GetAgent(ctx context.Context, id string) (*AgentItem, error) {
	return m.getFn(ctx, id)
}

func (m *mockAgentRepo) GetAgentByAgentKey(ctx context.Context, agentKey string) (*AgentItem, error) {
	if m.getByAgentKeyFn != nil {
		return m.getByAgentKeyFn(ctx, agentKey)
	}
	return nil, fmt.Errorf("not found")
}

type mockSkillRepo struct {
	listFn func(ctx context.Context, keyword string, limit, offset int32) ([]SkillItem, int32, error)
	getFn  func(ctx context.Context, id string) (*SkillItem, error)
}

func (m *mockSkillRepo) ListSkills(ctx context.Context, keyword string, limit, offset int32) ([]SkillItem, int32, error) {
	return m.listFn(ctx, keyword, limit, offset)
}

func (m *mockSkillRepo) GetSkill(ctx context.Context, id string) (*SkillItem, error) {
	return m.getFn(ctx, id)
}

func TestAgentListTool(t *testing.T) {
	t.Run("default limit", func(t *testing.T) {
		var capturedLimit int32
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				listFn: func(_ context.Context, _ string, limit, _ int32) ([]AgentItem, int32, error) {
					capturedLimit = limit
					return []AgentItem{}, int32(0), nil
				},
			},
		}
		tool := newAgentListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedLimit != 20 {
			t.Fatalf("expected default limit 20, got %d", capturedLimit)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		var capturedLimit int32
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				listFn: func(_ context.Context, _ string, limit, _ int32) ([]AgentItem, int32, error) {
					capturedLimit = limit
					return []AgentItem{}, int32(0), nil
				},
			},
		}
		tool := newAgentListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{"limit":5}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedLimit != 5 {
			t.Fatalf("expected limit 5, got %d", capturedLimit)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]AgentItem, int32, error) {
					return nil, int32(0), context.DeadlineExceeded
				},
			},
		}
		tool := newAgentListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty result", func(t *testing.T) {
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]AgentItem, int32, error) {
					return []AgentItem{}, int32(0), nil
				},
			},
		}
		tool := newAgentListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		out, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, ok := out.(agentListOutput)
		if !ok {
			t.Fatalf("expected agentListOutput, got %T", out)
		}
		if len(result.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(result.Items))
		}
		if result.Total != 0 {
			t.Fatalf("expected total 0, got %d", result.Total)
		}
	})

	t.Run("with results", func(t *testing.T) {
		items := []AgentItem{
			{ID: "a1", AgentKey: "agent-1", DisplayName: "Agent 1", Provider: "openai", Model: "gpt-4", Status: "active"},
			{ID: "a2", AgentKey: "agent-2", DisplayName: "Agent 2", Provider: "anthropic", Model: "claude-3", Status: "active"},
		}
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]AgentItem, int32, error) {
					return items, int32(2), nil
				},
			},
		}
		tool := newAgentListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		out, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, ok := out.(agentListOutput)
		if !ok {
			t.Fatalf("expected agentListOutput, got %T", out)
		}
		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result.Items))
		}
		if result.Total != 2 {
			t.Fatalf("expected total 2, got %d", result.Total)
		}
	})
}

func TestAgentGetTool(t *testing.T) {
	t.Run("empty ID", func(t *testing.T) {
		deps := Deps{
			AgentRepo: &mockAgentRepo{},
		}
		tool := newAgentGetTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{"id":""}`))
		if err == nil || !strings.Contains(err.Error(), "id is required") {
			t.Fatalf("expected 'id is required' error, got %v", err)
		}
	})

	t.Run("valid ID", func(t *testing.T) {
		expected := &AgentItem{ID: "a1", AgentKey: "agent-1", DisplayName: "Agent 1", Provider: "openai", Model: "gpt-4", Status: "active"}
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				getFn: func(_ context.Context, id string) (*AgentItem, error) {
					if id != "a1" {
						t.Fatalf("expected id 'a1', got %q", id)
					}
					return expected, nil
				},
			},
		}
		tool := newAgentGetTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		out, err := callable.Call(context.Background(), []byte(`{"id":"a1"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, ok := out.(*AgentItem)
		if !ok {
			t.Fatalf("expected *AgentItem, got %T", out)
		}
		if result.ID != "a1" {
			t.Fatalf("expected ID 'a1', got %q", result.ID)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		deps := Deps{
			AgentRepo: &mockAgentRepo{
				getFn: func(_ context.Context, _ string) (*AgentItem, error) {
					return nil, context.DeadlineExceeded
				},
			},
		}
		tool := newAgentGetTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{"id":"a1"}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSkillListTool(t *testing.T) {
	t.Run("default limit", func(t *testing.T) {
		var capturedLimit int32
		deps := Deps{
			SkillRepo: &mockSkillRepo{
				listFn: func(_ context.Context, _ string, limit, _ int32) ([]SkillItem, int32, error) {
					capturedLimit = limit
					return []SkillItem{}, int32(0), nil
				},
			},
		}
		tool := newSkillListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedLimit != 20 {
			t.Fatalf("expected default limit 20, got %d", capturedLimit)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		var capturedLimit int32
		deps := Deps{
			SkillRepo: &mockSkillRepo{
				listFn: func(_ context.Context, _ string, limit, _ int32) ([]SkillItem, int32, error) {
					capturedLimit = limit
					return []SkillItem{}, int32(0), nil
				},
			},
		}
		tool := newSkillListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{"limit":10}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedLimit != 10 {
			t.Fatalf("expected limit 10, got %d", capturedLimit)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		deps := Deps{
			SkillRepo: &mockSkillRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]SkillItem, int32, error) {
					return nil, int32(0), context.DeadlineExceeded
				},
			},
		}
		tool := newSkillListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		_, err := callable.Call(context.Background(), []byte(`{}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty result", func(t *testing.T) {
		deps := Deps{
			SkillRepo: &mockSkillRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]SkillItem, int32, error) {
					return []SkillItem{}, int32(0), nil
				},
			},
		}
		tool := newSkillListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		out, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, ok := out.(skillListOutput)
		if !ok {
			t.Fatalf("expected skillListOutput, got %T", out)
		}
		if len(result.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(result.Items))
		}
		if result.Total != 0 {
			t.Fatalf("expected total 0, got %d", result.Total)
		}
	})

	t.Run("with results", func(t *testing.T) {
		items := []SkillItem{
			{ID: "s1", SkillKey: "skill-1", DisplayName: "Skill 1", Status: "active", Version: "1.0"},
			{ID: "s2", SkillKey: "skill-2", DisplayName: "Skill 2", Status: "active", Version: "2.0"},
		}
		deps := Deps{
			SkillRepo: &mockSkillRepo{
				listFn: func(_ context.Context, _ string, _, _ int32) ([]SkillItem, int32, error) {
					return items, int32(2), nil
				},
			},
		}
		tool := newSkillListTool(deps)
		callable, ok := tool.(trpctool.CallableTool)
		if !ok {
			t.Fatal("tool is not callable")
		}
		out, err := callable.Call(context.Background(), []byte(`{}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result, ok := out.(skillListOutput)
		if !ok {
			t.Fatalf("expected skillListOutput, got %T", out)
		}
		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result.Items))
		}
		if result.Total != 2 {
			t.Fatalf("expected total 2, got %d", result.Total)
		}
	})
}
