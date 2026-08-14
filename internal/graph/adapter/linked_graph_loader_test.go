package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestLinkedGraphBuildConfigLoader_NilAndEmpty(t *testing.T) {
	t.Parallel()
	var l *LinkedGraphBuildConfigLoader
	if _, err := l.LoadGraphBuildConfig(context.Background(), "g1"); err == nil {
		t.Fatal("nil loader must fail")
	}
	l = NewLinkedGraphBuildConfigLoader(nil)
	if _, err := l.LoadGraphBuildConfig(context.Background(), "g1"); err == nil {
		t.Fatal("nil usecase must fail")
	}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo:    &idorStyleGraphRepo{defs: map[string]*biz.GraphDefinition{}},
		RunRepo: &idorStyleRunRepo{execs: map[string]*biz.GraphExecution{}},
		Lg:      loggateway.NewNoop(),
	})
	l = NewLinkedGraphBuildConfigLoader(uc)
	if _, err := l.LoadGraphBuildConfig(context.Background(), "  "); err == nil {
		t.Fatal("empty graph id must fail")
	}
	if _, err := l.LoadGraphBuildConfig(context.Background(), "missing"); err == nil {
		t.Fatal("missing graph must fail")
	}
}

func TestLinkedGraphBuildConfigLoader_Hit(t *testing.T) {
	t.Parallel()
	repo := &idorStyleGraphRepo{defs: map[string]*biz.GraphDefinition{
		"g-1": {
			ID:          "g-1",
			Name:        "linked",
			EntryPoint:  "n1",
			FinishPoint: "n1",
			Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "a1"}},
		},
	}}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo:    repo,
		RunRepo: &idorStyleRunRepo{execs: map[string]*biz.GraphExecution{}},
		Lg:      loggateway.NewNoop(),
	})
	cfg, err := NewLinkedGraphBuildConfigLoader(uc).LoadGraphBuildConfig(context.Background(), "g-1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EntryPoint != "n1" || len(cfg.Nodes) != 1 {
		t.Fatalf("cfg=%+v", cfg)
	}
}

// Minimal GraphRepo/GraphRunRepo copies so loader tests do not import service stubs.
type idorStyleGraphRepo struct {
	defs map[string]*biz.GraphDefinition
}

func (r *idorStyleGraphRepo) GetDefinition(_ context.Context, id string) (*biz.GraphDefinition, error) {
	if d, ok := r.defs[id]; ok {
		return d, nil
	}
	return nil, biz.ErrNotFound
}
func (r *idorStyleGraphRepo) GetDefinitionByName(context.Context, string) (*biz.GraphDefinition, error) {
	return nil, biz.ErrNotFound
}
func (r *idorStyleGraphRepo) ListDefinitions(context.Context, int, string) ([]*biz.GraphDefinition, string, error) {
	return nil, "", nil
}
func (r *idorStyleGraphRepo) ListUserTemplateDefinitions(context.Context, int) ([]*biz.GraphDefinition, error) {
	return nil, nil
}
func (r *idorStyleGraphRepo) ListDefinitionsByWorkspace(context.Context, int, string, string) ([]*biz.GraphDefinition, string, error) {
	return nil, "", nil
}
func (r *idorStyleGraphRepo) ListUserTemplateDefinitionsByWorkspace(context.Context, int, string) ([]*biz.GraphDefinition, error) {
	return nil, nil
}
func (r *idorStyleGraphRepo) SaveDefinition(_ context.Context, d *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return d, nil
}
func (r *idorStyleGraphRepo) UpdateDefinition(_ context.Context, d *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return d, nil
}
func (r *idorStyleGraphRepo) DeleteDefinition(context.Context, string) error { return nil }
func (r *idorStyleGraphRepo) ReorderGraphs(context.Context, []string) error  { return nil }

type idorStyleRunRepo struct {
	execs map[string]*biz.GraphExecution
}

func (r *idorStyleRunRepo) SaveRun(context.Context, *biz.GraphExecution) error { return nil }
func (r *idorStyleRunRepo) GetRun(_ context.Context, id string) (*biz.GraphExecution, error) {
	if e, ok := r.execs[id]; ok {
		return e, nil
	}
	return nil, biz.ErrNotFound
}
func (r *idorStyleRunRepo) ListRunsByGraph(context.Context, string, int, string, ...biz.GraphRunListOption) ([]*biz.GraphExecution, string, error) {
	return nil, "", nil
}
func (r *idorStyleRunRepo) UpdateRun(context.Context, *biz.GraphExecution) error { return nil }
