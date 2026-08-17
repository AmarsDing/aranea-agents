package data

// ── G4-B8：ListCollectionLinks 库级关联读取（3D 图谱数据源） ────────────────

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
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

func TestKnowledgeRepo_ListChunksByDocuments(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedCollectionLinks(t, repo)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.InsertChunks(ctx, []bizknowledge.Chunk{
		{ID: "k1a", DocID: "d1", CollectionID: "c1", Content: "d1 first", ChunkIndex: 0},
		{ID: "k1b", DocID: "d1", CollectionID: "c1", Content: "d1 second", ChunkIndex: 1},
		{ID: "k1c", DocID: "d1", CollectionID: "c1", Content: "d1 third", ChunkIndex: 2},
		{ID: "k2a", DocID: "d2", CollectionID: "c1", Content: "d2 first", ChunkIndex: 0},
		{ID: "k2b", DocID: "d2", CollectionID: "c1", Content: "d2 second", ChunkIndex: 1},
	}))

	got, err := repo.ListChunksByDocuments(ctx, "c1", []string{"d1", "d2"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("chunks = %d, want 4: %+v", len(got), got)
	}
	// limitPerDoc=2 必须丢掉 d1 的第三块。
	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
	}
	if ids["k1c"] {
		t.Fatalf("chunk_index 2 must be truncated: %+v", got)
	}
	if !ids["k1a"] || !ids["k1b"] || !ids["k2a"] || !ids["k2b"] {
		t.Fatalf("expected first two chunks per doc, got %+v", got)
	}

	got, err = repo.ListChunksByDocuments(ctx, "c1", nil, 2)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty docIDs = (%v, %v), want empty", got, err)
	}
}

func TestKnowledgeRepo_GraphExpandHidesPrivateNeighbors(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedCollectionLinks(t, repo)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.InsertChunks(ctx, []bizknowledge.Chunk{
		{ID: "k1", DocID: "d1", CollectionID: "c1", Content: "public seed", ChunkIndex: 0},
		{ID: "k2", DocID: "d2", CollectionID: "c1", Content: "private neighbor", ChunkIndex: 0},
	}))
	must(repo.UpdateDocumentVisibility(ctx, "d2", "private", "7"))

	other := auth.NewContext(ctx, &auth.Auth{UserID: 8})
	chunks, err := repo.ListChunksByDocuments(other, "c1", []string{"d1", "d2"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].DocID != "d1" {
		t.Fatalf("other user chunks = %+v, want only d1", chunks)
	}
	links, err := repo.ListLinks(other, "c1", "d1", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.DocID == "d2" || l.TargetDocID == "d2" {
			t.Fatalf("other user must not see link to private d2: %+v", l)
		}
	}
	active, err := repo.ListActiveLinks(other, "c1", []string{"d1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range active {
		if l.DocID == "d2" || l.TargetDocID == "d2" {
			t.Fatalf("other user must not see active link to private d2: %+v", l)
		}
	}

	owner := auth.NewContext(ctx, &auth.Auth{UserID: 7})
	chunks, err = repo.ListChunksByDocuments(owner, "c1", []string{"d1", "d2"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("owner chunks = %+v, want d1+d2", chunks)
	}

	sys := workspace.WithSystemWorkspace(ctx)
	chunks, err = repo.ListChunksByDocuments(sys, "c1", []string{"d1", "d2"}, 2)
	if err != nil || len(chunks) != 2 {
		t.Fatalf("system chunks = (%+v, %v), want 2", chunks, err)
	}
}
