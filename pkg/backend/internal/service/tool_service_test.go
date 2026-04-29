// tool_service_test.go 覆盖第三阶段（§5.10）接入的 agent-evolution → 工具策略：`ToolService.EffectiveForAgent` 须将黑名单工具降为
// denied 且 reason="evolution_blacklist"，
// 并按智能体 strategy.tool_preference 重排允许项。
package service

import (
	"context"
	"path/filepath"
	"testing"

	"arenea/backend/internal/repository"
	toolstorage "arenea/backend/internal/capability/storage"
)

func newTestToolService(t *testing.T) (*ToolService, *AgentEvolutionService, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tools.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tools := NewToolService(toolstorage.NewSQLiteStore(repo.DB()))
	tools.SetRuntimeSettingsStore(repo)
	evo := NewAgentEvolutionService(repo)
	tools.SetEvolutionPolicySource(evo)
	return tools, evo, repo
}

// §13 – 进化黑名单须在 EffectiveForAgent 中体现为
// state=denied / reason="evolution_blacklist"，且不影响目录其余项。
func TestToolEffectiveAppliesEvolutionBlacklist(t *testing.T) {
	tools, evo, _ := newTestToolService(t)
	bl := []string{"shell_exec"}
	if _, err := evo.UpdateStrategy(context.Background(), "agent-bl", StrategyPatch{
		ToolBlacklist: &bl,
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	view, err := tools.EffectiveForAgent("agent-bl")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	var found bool
	for _, item := range view.Items {
		if item.ToolKey == "shell_exec" {
			found = true
			if item.EffectiveState != "denied" || item.Reason != "evolution_blacklist" || item.Enabled {
				t.Fatalf("expected shell_exec denied via evolution_blacklist, got %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("expected shell_exec in seeded tool catalog")
	}
}

// §13 – tool_preference 须重排允许项，使最高分工具在提示渲染视图中排最前。
func TestToolEffectiveSortsByEvolutionPreference(t *testing.T) {
	tools, evo, _ := newTestToolService(t)
	if _, err := evo.UpdateStrategy(context.Background(), "agent-pref", StrategyPatch{
		ToolPreference: map[string]float64{
			"datetime":   0.95,
			"web_search": 0.10,
		},
	}); err != nil {
		t.Fatalf("update strategy: %v", err)
	}
	view, err := tools.EffectiveForAgent("agent-pref")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	var dtIdx, wsIdx = -1, -1
	for i, item := range view.Items {
		if item.EffectiveState != "allowed" {
			continue
		}
		if item.ToolKey == "datetime" {
			dtIdx = i
		}
		if item.ToolKey == "web_search" {
			wsIdx = i
		}
	}
	if dtIdx < 0 || wsIdx < 0 {
		t.Fatalf("expected both datetime and web_search to be allowed, got dt=%d ws=%d", dtIdx, wsIdx)
	}
	if dtIdx >= wsIdx {
		t.Fatalf("expected datetime before web_search after preference reorder, got dt=%d ws=%d", dtIdx, wsIdx)
	}
}

// 未接入 evolution 源时 ToolService 仍须产生一致视图 —— 防止意外空指针解引用。
func TestToolEffectiveWithoutEvolutionSourceWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tools.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tools := NewToolService(toolstorage.NewSQLiteStore(repo.DB()))
	tools.SetRuntimeSettingsStore(repo)
	view, err := tools.EffectiveForAgent("agent-none")
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if len(view.Items) == 0 {
		t.Fatalf("expected seeded tool catalog, got 0 items")
	}
}
