package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestBackfillOutgoingAutolinks_PersistsWikilinks(t *testing.T) {
	m := noOpMockRepo()
	var updated string
	docs := []Document{
		{ID: "a", CollectionID: "c1", RelPath: "手册.md", Source: "手册.md", MimeType: "text/markdown", ContentText: "请遵守值班制度。"},
		{ID: "b", CollectionID: "c1", RelPath: "值班制度.md", Source: "值班制度.md", MimeType: "text/markdown", ContentText: "夜班必须双人值守。"},
	}
	m.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendTeam}, nil
	}
	m.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		out := make([]Document, len(docs))
		for i, d := range docs {
			out[i] = Document{ID: d.ID, CollectionID: d.CollectionID, RelPath: d.RelPath, Source: d.Source, MimeType: d.MimeType}
		}
		return out, len(out), nil
	}
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		for _, d := range docs {
			if d.ID == id {
				return d, nil
			}
		}
		return Document{ID: id}, nil
	}
	m.docContentFn = func(_ context.Context, id, contentText string, _ bool) error {
		if id == "a" {
			updated = contentText
		}
		for i := range docs {
			if docs[i].ID == id {
				docs[i].ContentText = contentText
			}
		}
		return nil
	}
	u := NewUsecaseFromRepo(m)
	res, err := u.BackfillOutgoingAutolinks(context.Background(), "c1", nil)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.Changed < 1 || res.Replacements < 1 {
		t.Fatalf("res=%+v updated=%q", res, updated)
	}
	if !strings.Contains(updated, "[[值班制度]]") {
		t.Fatalf("expected wikilink in %q", updated)
	}
}

func TestApplyOutgoingAutolinks_Idempotent(t *testing.T) {
	m := noOpMockRepo()
	body := "见[[值班制度]]即可。"
	m.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendTeam}, nil
	}
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", RelPath: "手册.md", Source: "手册.md", ContentText: body, MimeType: "text/markdown"}, nil
	}
	m.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return []Document{
			{ID: "doc-1", RelPath: "手册.md", Source: "手册.md"},
			{ID: "b", RelPath: "值班制度.md", Source: "值班制度.md"},
		}, 2, nil
	}
	u := NewUsecaseFromRepo(m)
	got, err := u.ApplyOutgoingAutolinks(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.Replacements != 0 {
		t.Fatalf("already linked, replacements=%d", got.Replacements)
	}
}

func TestPreviewOutgoingAutolinks_Counts(t *testing.T) {
	m := noOpMockRepo()
	m.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendTeam}, nil
	}
	m.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", RelPath: "手册.md", Source: "手册.md", ContentText: "请遵守值班制度。", MimeType: "text/markdown"}, nil
	}
	m.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return []Document{
			{ID: "doc-1", RelPath: "手册.md", Source: "手册.md"},
			{ID: "b", RelPath: "值班制度.md", Source: "值班制度.md"},
		}, 2, nil
	}
	u := NewUsecaseFromRepo(m)
	got, err := u.PreviewOutgoingAutolinks(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Replacements != 1 || !strings.Contains(got.Preview, "[[值班制度]]") {
		t.Fatalf("got %+v", got)
	}
}
