package data

// ── G4-B8：ListCollectionLinks 库级关联读取（3D 图谱数据源） ────────────────

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

func seedCollectionLinks(t *testing.T, repo *knowledgeRepo) {
	t.Helper()
	ctx := context.Background()
	for _, c := range []struct{ id, name, root string }{{"c1", "vault-a", "/vault-a"}, {"c2", "vault-b", "/vault-b"}} {
		if _, err := repo.CreateCollection(ctx, bizknowledge.Collection{ID: c.id, Name: c.name, RootPath: c.root}); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// knowledge_links.doc_id/target_doc_id 有 FK：先造文档。
	for _, d := range []struct{ id, col, rel string }{
		{"d1", "c1", "a.md"}, {"d2", "c1", "b.md"}, {"d3", "c1", "q1.md"},
		{"x1", "c2", "x1.md"}, {"x2", "c2", "x2.md"},
	} {
		_, err := repo.CreateDocument(ctx, bizknowledge.Document{
			ID: d.id, CollectionID: d.col, RelPath: d.rel, Source: d.rel, Status: "indexed",
		})
		must(err)
	}
	must(repo.ReplaceLinks(ctx, "c1", "d1", bizknowledge.LinkTypeExplicit, []bizknowledge.Link{
		{TargetDocID: "d2", Context: "[[b]]"},
		{TargetDocID: "d3", Context: "[[q1]]"},
	}))
	must(repo.ReplaceLinks(ctx, "c1", "d2", bizknowledge.LinkTypeEntity, []bizknowledge.Link{
		{TargetDocID: "d3", Context: "营收"},
	}))
	// c2 的关联不得混入 c1（collection 隔离）。
	must(repo.ReplaceLinks(ctx, "c2", "x1", bizknowledge.LinkTypeExplicit, []bizknowledge.Link{
		{TargetDocID: "x2"},
	}))
}

func TestKnowledgeRepo_ListCollectionLinks(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedCollectionLinks(t, repo)
	ctx := context.Background()

	// 全类型：c1 三条（2 explicit + 1 entity），按 id 有序。
	got, err := repo.ListCollectionLinks(ctx, "c1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("links = %d, want 3: %+v", len(got), got)
	}
	if got[0].DocID != "d1" || got[0].TargetDocID != "d2" || got[0].LinkType != bizknowledge.LinkTypeExplicit || got[0].Context != "[[b]]" {
		t.Fatalf("first link = %+v", got[0])
	}
	if got[2].LinkType != bizknowledge.LinkTypeEntity || got[2].Context != "营收" {
		t.Fatalf("entity link = %+v", got[2])
	}

	// 类型过滤：单类型。
	got, err = repo.ListCollectionLinks(ctx, "c1", []string{bizknowledge.LinkTypeExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("explicit links = %d, want 2", len(got))
	}

	// 类型过滤：多类型 ANY。
	got, err = repo.ListCollectionLinks(ctx, "c1", []string{bizknowledge.LinkTypeExplicit, bizknowledge.LinkTypeEntity})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("explicit+entity links = %d, want 3", len(got))
	}

	// 未命中类型 → 空。
	got, err = repo.ListCollectionLinks(ctx, "c1", []string{bizknowledge.LinkTypeSemantic})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("semantic links = %d, want 0", len(got))
	}

	// collection 隔离：c2 仅自己的一条。
	got, err = repo.ListCollectionLinks(ctx, "c2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DocID != "x1" {
		t.Fatalf("c2 links = %+v", got)
	}
}
