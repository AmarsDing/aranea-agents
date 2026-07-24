package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func newTestCompiledTeamRepo(t *testing.T) (biz.CompiledTeamRepo, *sql.DB) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureCompiledTeamSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewCompiledTeamRepo(&Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}, nil)
	return repo, db
}

func TestCompiledTeamRepo_SaveAndLoad(t *testing.T) {
	repo, _ := newTestCompiledTeamRepo(t)
	ctx := context.Background()

	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "a1"}},
		EntryPoint:  "n1",
		FinishPoint: "n1",
	}, nil, nil, nil)

	if err := repo.Save(ctx, "team-1", "graph-1", "sess-1", ct); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := repo.Load(ctx, "team-1", "graph-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.EntryPoint != "n1" {
		t.Errorf("EntryPoint=%q want n1", loaded.EntryPoint)
	}
	if len(loaded.Nodes) != 1 {
		t.Errorf("Nodes=%d want 1", len(loaded.Nodes))
	}
	if loaded.Nodes[0].AgentName != "a1" {
		t.Errorf("AgentName=%q want a1", loaded.Nodes[0].AgentName)
	}
}

func TestCompiledTeamRepo_LoadNotFound(t *testing.T) {
	repo, _ := newTestCompiledTeamRepo(t)
	ctx := context.Background()

	_, err := repo.Load(ctx, "nonexistent", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing record")
	}
}

func TestCompiledTeamRepo_Delete(t *testing.T) {
	repo, _ := newTestCompiledTeamRepo(t)
	ctx := context.Background()

	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent}},
		EntryPoint:  "n1",
		FinishPoint: "n1",
	}, nil, nil, nil)

	if err := repo.Save(ctx, "team-1", "graph-1", "sess-1", ct); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := repo.Delete(ctx, "team-1", "graph-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := repo.Load(ctx, "team-1", "graph-1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCompiledTeamRepo_SaveUpsert(t *testing.T) {
	repo, _ := newTestCompiledTeamRepo(t)
	ctx := context.Background()

	ct1 := biz.NewCompiledTeam(biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "v1"}},
		EntryPoint:  "n1",
		FinishPoint: "n1",
	}, nil, nil, nil)

	if err := repo.Save(ctx, "team-1", "graph-1", "sess-1", ct1); err != nil {
		t.Fatalf("Save 1 failed: %v", err)
	}

	ct2 := biz.NewCompiledTeam(biz.GraphBuildConfig{
		Nodes:       []biz.NodeDef{{ID: "n1", Type: biz.NodeTypeAgent, AgentName: "v2"}},
		EntryPoint:  "n1",
		FinishPoint: "n1",
	}, nil, nil, nil)

	if err := repo.Save(ctx, "team-1", "graph-1", "sess-2", ct2); err != nil {
		t.Fatalf("Save 2 (upsert) failed: %v", err)
	}

	loaded, err := repo.Load(ctx, "team-1", "graph-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Nodes[0].AgentName != "v2" {
		t.Errorf("AgentName=%q want v2 (upserted value)", loaded.Nodes[0].AgentName)
	}
}
