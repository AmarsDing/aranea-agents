package knowledge

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestEnsureEntityWikiBody(t *testing.T) {
	t.Parallel()
	body, changed := ensureEntityWikiBody("", "PostgreSQL", "tech", "notes/runbook.md")
	if !changed || !strings.Contains(body, "[[notes/runbook]]") || !strings.Contains(body, "title: PostgreSQL") {
		t.Fatalf("new page: changed=%v body=%q", changed, body)
	}
	again, changed := ensureEntityWikiBody(body, "PostgreSQL", "tech", "notes/runbook.md")
	if changed || again != body {
		t.Fatalf("idempotent mention must not rewrite")
	}
	next, changed := ensureEntityWikiBody(body, "PostgreSQL", "tech", "ops/ha.md")
	if !changed || !strings.Contains(next, "[[ops/ha]]") || !strings.Contains(next, "[[notes/runbook]]") {
		t.Fatalf("second source: %q", next)
	}
}

func TestEnsureEntityWikiPages_TeamOnly(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"src": {ID: "src", CollectionID: "c1", RelPath: "notes/a.md", Source: "notes/a.md", ContentText: "PostgreSQL"},
	}
	created := 0
	repo := &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, VaultBackend: VaultBackendLocal}, nil
		},
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
		docCreateFn: func(_ context.Context, d Document) (Document, error) {
			created++
			d.ID = "new"
			return d, nil
		},
	}
	u := NewUsecaseFromRepo(repo)
	if err := u.EnsureEntityWikiPages(context.Background(), "c1", "src", []DocEntity{{Name: "PostgreSQL"}}); err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("local vault must not write wiki pages, created=%d", created)
	}
}

func TestEnsureEntityWikiPages_CreatesTeamEntry(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"src": {ID: "src", CollectionID: "c1", RelPath: "notes/a.md", ContentText: "x"},
	}
	repo := &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, VaultBackend: VaultBackendTeam}, nil
		},
		docGetFn: func(_ context.Context, id string) (Document, error) {
			d, ok := docs[id]
			if !ok {
				return Document{}, apierror.NotFound("KNOWLEDGE", "missing")
			}
			return d, nil
		},
		docGetByRelFn: func(_ context.Context, _, rel string) (Document, error) {
			for _, d := range docs {
				if d.RelPath == rel {
					return d, nil
				}
			}
			return Document{}, apierror.NotFound("KNOWLEDGE", "missing")
		},
		docCreateFn: func(_ context.Context, d Document) (Document, error) {
			if d.ID == "" {
				d.ID = "wiki-1"
			}
			docs[d.ID] = d
			return d, nil
		},
		docContentFn: func(_ context.Context, id, contentText string, _ bool) error {
			d := docs[id]
			d.ContentText = contentText
			docs[id] = d
			return nil
		},
	}
	u := NewUsecaseFromRepo(repo)
	if err := u.EnsureEntityWikiPages(context.Background(), "c1", "src", []DocEntity{{Name: "PostgreSQL", EntityType: "tech"}}); err != nil {
		t.Fatal(err)
	}
	var wiki Document
	for _, d := range docs {
		if d.RelPath == "entries/postgresql.md" {
			wiki = d
		}
	}
	if wiki.ID == "" {
		t.Fatalf("wiki page not created: %+v", docs)
	}
	if !strings.Contains(wiki.ContentText, "[[notes/a]]") {
		t.Fatalf("missing source link: %s", wiki.ContentText)
	}
}
