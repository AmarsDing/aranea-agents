package chatactivity

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubToolRepo struct {
	biz.ToolRepo
	display map[string]string
}

func (s *stubToolRepo) GetTool(_ context.Context, idOrKey string) (biz.Tool, error) {
	if name, ok := s.display[idOrKey]; ok {
		return biz.Tool{Key: idOrKey, DisplayName: name}, nil
	}
	return biz.Tool{}, biz.ErrNotFound
}

type stubAgentRepo struct {
	biz.AgentRepository
	names map[string]string
	ids   map[string]string
}

func (s *stubAgentRepo) GetAgentByAgentKey(_ context.Context, agentKey string) (biz.Agent, error) {
	name, nameOK := s.names[agentKey]
	id, idOK := s.ids[agentKey]
	if !nameOK && !idOK {
		return biz.Agent{}, biz.ErrNotFound
	}
	return biz.Agent{ID: id, AgentKey: agentKey, DisplayName: name}, nil
}
func (s *stubAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) { return 0, nil }

func TestCatalogActivityMetaResolver(t *testing.T) {
	resolver := newCatalogActivityMetaResolver(
		biz.NewToolUsecase(&stubToolRepo{display: map[string]string{
			"save_file": "保存文件",
		}}, nil, loggateway.NewNoop()),
		&stubAgentRepo{
			names: map[string]string{"worker-a": "Worker A"},
			ids:   map[string]string{"worker-a": "ag-worker-a"},
		},
	)
	if got := resolver.ResolveDisplayLabel(context.Background(), "write_file"); got != "保存文件" {
		t.Fatalf("display label: %q", got)
	}
	if got := resolver.ResolveAgentDisplayName(context.Background(), "worker-a"); got != "Worker A" {
		t.Fatalf("agent name: %q", got)
	}
	if got := resolver.ResolveAgentID(context.Background(), "worker-a"); got != "ag-worker-a" {
		t.Fatalf("agent id: %q", got)
	}
}
