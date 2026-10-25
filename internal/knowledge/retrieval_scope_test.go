package knowledge

import (
	"context"
	"reflect"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

type scopedTestChunkLister struct {
	fakeChunkLister
	query biz.KnowledgeSearchQuery
}

func (s *scopedTestChunkLister) ListChunksByDocumentsForSearch(_ context.Context, q biz.KnowledgeSearchQuery, _ []string, _ int) ([]biz.KnowledgeChunk, error) {
	s.query = q
	return []biz.KnowledgeChunk{{ID: "scoped", DocID: "d2", Content: "within scope"}}, nil
}

func TestGraphExpansion_SearchScopePreservedAndFailClosed(t *testing.T) {
	q := biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "两者关系", TopK: 5,
		PathPrefix: "notes", ExcludePathPrefixes: []string{"inbox/"}, FilterJSON: `{"category":"approved"}`}
	links := fakeLinkReader{byDoc: map[string][]bizknowledge.Link{
		"d1": {{DocID: "d1", TargetDocID: "d2", LinkType: bizknowledge.LinkTypeExplicit}},
	}}
	seeds := []biz.KnowledgeChunk{{ID: "seed", DocID: "d1", Score: 1}}
	scoped := &scopedTestChunkLister{}
	got := NewGraphExpander(links, scoped, loggateway.NewNoop()).Expand(context.Background(), q, seeds)
	if len(got) != 2 || !reflect.DeepEqual(scoped.query, q) {
		t.Fatalf("scope must reach neighbor selection: got=%v q=%+v", got, scoped.query)
	}
	legacy := fakeChunkLister{byDoc: map[string][]biz.KnowledgeChunk{"d2": {{ID: "outside", DocID: "d2"}}}}
	got = NewGraphExpander(links, legacy, loggateway.NewNoop()).Expand(context.Background(), q, seeds)
	if len(got) != 1 || got[0].ID != "seed" {
		t.Fatalf("unscoped fallback must not broaden query: %v", got)
	}
}

type scopedTestCardHosts struct {
	stubCardHostDocs
	query biz.KnowledgeSearchQuery
}

func (s *scopedTestCardHosts) ListChunksByDocumentsForSearch(_ context.Context, q biz.KnowledgeSearchQuery, _ []string, _ int) ([]biz.KnowledgeChunk, error) {
	s.query = q
	return []biz.KnowledgeChunk{{DocID: "inside"}}, nil
}

func TestGraphCards_AsOfNeverReadsCurrentRelationCards(t *testing.T) {
	vector := &stubCardVectorSearcher{
		entityHits: []bizknowledge.GraphCardHit{{Kind: "entity", RefID: 1, Text: "current node"}},
		edgeHits:   []bizknowledge.GraphCardHit{{Kind: "edge", RefID: 2, Text: "future relation"}},
	}
	hosts := &stubCardHostDocs{byRef: map[int64][]string{1: {"d1"}, 2: {"d1", "d2"}}}
	s := newTestCardSearcher(vector, &stubCardLexicalSearcher{}, hosts)
	got, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "c1", Temporal: bizknowledge.TemporalSpan{ValidAt: time.Now().Add(-time.Hour)},
	}, []float32{1})
	if err != nil || len(got) != 1 || got[0].Content != "current node" {
		t.Fatalf("historical query included relation: got=%v err=%v", got, err)
	}
	if len(vector.kindSeen) != 1 || vector.kindSeen[0] != "entity" {
		t.Fatalf("as-of must not consult current edge projection: %v", vector.kindSeen)
	}
}

func TestGraphCards_SearchScopeAppliesToBothRelationEndpoints(t *testing.T) {
	vector := &stubCardVectorSearcher{
		entityHits: []bizknowledge.GraphCardHit{{Kind: "entity", RefID: 1}},
		edgeHits:   []bizknowledge.GraphCardHit{{Kind: "edge", RefID: 2}},
	}
	hosts := &scopedTestCardHosts{stubCardHostDocs: stubCardHostDocs{
		byRef: map[int64][]string{1: {"inside", "outside"}, 2: {"inside", "outside"}},
	}}
	s := NewGraphCardSearcher(vector, &stubCardLexicalSearcher{}, hosts, loggateway.NewNoop())
	q := biz.KnowledgeSearchQuery{CollectionID: "c1", PathPrefix: "notes", ExcludePathPrefixes: []string{"inbox/"}}
	got, err := s.SearchGraphCardChunks(context.Background(), q, []float32{1})
	if err != nil || len(got) != 1 || got[0].ID != "g33e/1/inside" || !reflect.DeepEqual(hosts.query, q) {
		t.Fatalf("card scope broadened: got=%v q=%+v err=%v", got, hosts.query, err)
	}
}

func TestGraphCards_ChunkMetadataFilterDoesNotBroadenToWholeHost(t *testing.T) {
	vector := &stubCardVectorSearcher{entityHits: []bizknowledge.GraphCardHit{{Kind: "entity", RefID: 1}}}
	hosts := &scopedTestCardHosts{stubCardHostDocs: stubCardHostDocs{byRef: map[int64][]string{1: {"inside"}}}}
	s := NewGraphCardSearcher(vector, &stubCardLexicalSearcher{}, hosts, loggateway.NewNoop())
	got, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "c1", FilterJSON: `{"approved":true}`,
	}, []float32{1})
	if err != nil || len(got) != 0 || len(vector.kindSeen) != 0 {
		t.Fatalf("card without exact source chunk must not satisfy chunk metadata: got=%v err=%v", got, err)
	}
}
