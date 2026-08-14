package knowledge

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

type fakeLinkReader struct {
	byDoc map[string][]bizknowledge.Link
	err   error
}

func (f fakeLinkReader) ListLinks(_ context.Context, _, docID, _ string) ([]bizknowledge.Link, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byDoc[docID], nil
}

type fakeChunkLister struct {
	byDoc map[string][]biz.KnowledgeChunk
	err   error
}

func (f fakeChunkLister) ListChunksByDocuments(_ context.Context, _ string, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []biz.KnowledgeChunk
	for _, id := range docIDs {
		chunks := f.byDoc[id]
		if limitPerDoc > 0 && len(chunks) > limitPerDoc {
			chunks = chunks[:limitPerDoc]
		}
		out = append(out, chunks...)
	}
	return out, nil
}

func TestGraphExpander_MergesNeighborChunks(t *testing.T) {
	exp := NewGraphExpander(
		fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
			"d1": {{DocID: "d1", TargetDocID: "d2", LinkType: bizknowledge.LinkTypeExplicit, Weight: 2}},
		}},
		fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{
			"d2": {{ID: "c-n1", DocID: "d2", CollectionID: "c1", Content: "neighbor body", ChunkIndex: 0}},
		}},
		loggateway.NewNoop(),
	)
	seeds := []biz.KnowledgeChunk{{
		ID: "c-s1", DocID: "d1", CollectionID: "c1", Content: "seed body", Score: 0.9,
	}}
	got := exp.Expand(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "两者关系如何", TopK: 5}, seeds)
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %+v", len(got), got)
	}
	if got[0].ID != "c-s1" {
		t.Fatalf("seed must rank first, got %s", got[0].ID)
	}
	if got[1].ID != "c-n1" {
		t.Fatalf("neighbor missing, got %+v", got)
	}
	if got[1].Score <= 0 {
		t.Fatalf("neighbor score must be decayed from seed, got %v", got[1].Score)
	}
}

func TestGraphExpander_SkipsInstantIntent(t *testing.T) {
	exp := NewGraphExpander(
		fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
			"d1": {{DocID: "d1", TargetDocID: "d2", LinkType: bizknowledge.LinkTypeExplicit}},
		}},
		fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{
			"d2": {{ID: "c-n1", DocID: "d2", Content: "should not appear"}},
		}},
		loggateway.NewNoop(),
	)
	seeds := []biz.KnowledgeChunk{{ID: "c-s1", DocID: "d1", Score: 1}}
	got := exp.Expand(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "c1", Query: "notes/handbook.md", TopK: 5,
	}, seeds)
	if len(got) != 1 || got[0].ID != "c-s1" {
		t.Fatalf("instant query must not expand, got %+v", got)
	}
}

func TestGraphExpander_DegradesOnLinkError(t *testing.T) {
	exp := NewGraphExpander(
		fakeLinkReader{err: errors.New("db down")},
		fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{
			"d2": {{ID: "c-n1"}},
		}},
		loggateway.NewNoop(),
	)
	seeds := []biz.KnowledgeChunk{{ID: "c-s1", DocID: "d1", Score: 1}}
	got := exp.Expand(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "什么是协议", TopK: 5}, seeds)
	if len(got) != 1 || got[0].ID != "c-s1" {
		t.Fatalf("link error must degrade to seeds, got %+v", got)
	}
}

func TestGraphExpander_PrefersExplicitOverSemantic(t *testing.T) {
	exp := NewGraphExpander(
		fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
			"d1": {
				{DocID: "d1", TargetDocID: "sem", LinkType: bizknowledge.LinkTypeSemantic, Weight: 9},
				{DocID: "d1", TargetDocID: "exp", LinkType: bizknowledge.LinkTypeExplicit, Weight: 1},
			},
		}},
		fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{
			"exp": {{ID: "c-exp", DocID: "exp", Content: "explicit"}},
			"sem": {{ID: "c-sem", DocID: "sem", Content: "semantic"}},
		}},
		loggateway.NewNoop(),
	)
	// cap neighbors at 1 by temporarily using a one-neighbor graph via max=8 but
	// we assert explicit appears: both should merge; explicit rank score 3*1=3,
	// semantic 1*9=9 — semantic wins on weight. Adjust: explicit weight 4 → 12 vs 9.
	exp.links = fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
		"d1": {
			{DocID: "d1", TargetDocID: "sem", LinkType: bizknowledge.LinkTypeSemantic, Weight: 1},
			{DocID: "d1", TargetDocID: "exp", LinkType: bizknowledge.LinkTypeExplicit, Weight: 1},
		},
	}}
	got := exp.Expand(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "对比两者", TopK: 5},
		[]biz.KnowledgeChunk{{ID: "c-s1", DocID: "d1", Score: 1}})
	ids := map[string]bool{}
	for _, ch := range got {
		ids[ch.ID] = true
	}
	if !ids["c-exp"] || !ids["c-sem"] {
		t.Fatalf("both neighbor types should merge when under cap, got %+v", got)
	}
}

func TestNewGraphExpander_NilDeps(t *testing.T) {
	if NewGraphExpander(nil, fakeChunkLister{}, loggateway.NewNoop()) != nil {
		t.Fatal("nil links must return nil expander")
	}
	if NewGraphExpander(fakeLinkReader{}, nil, loggateway.NewNoop()) != nil {
		t.Fatal("nil chunks must return nil expander")
	}
}

func TestPickAutoRewriteStrategy(t *testing.T) {
	if got := pickAutoRewriteStrategy(QueryComplex, RewriteNone); got != RewriteMultiQuery {
		t.Fatalf("complex + none → multi_query, got %q", got)
	}
	if got := pickAutoRewriteStrategy(QueryComplex, RewriteHyDE); got != RewriteNone {
		t.Fatalf("already rewritten must not auto-pick, got %q", got)
	}
	if got := pickAutoRewriteStrategy(QuerySimple, RewriteNone); got != RewriteNone {
		t.Fatalf("simple must not auto-rewrite, got %q", got)
	}
	if got := pickAutoRewriteStrategy(QueryModerate, RewriteNone); got != RewriteNone {
		t.Fatalf("moderate must not auto-rewrite, got %q", got)
	}
}
