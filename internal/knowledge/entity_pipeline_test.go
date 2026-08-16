package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 自治理图谱 M2.1 实体共现轨契约：
//   - mock LLM 返回实体清单 → ReplaceDocEntities（归一化字典）→ 共现 → entity 出链；
//   - 出链 context=共享实体名、weight=共享数、link_type=entity；
//   - content_hash 一致且已抽过 → 跳过（零 LLM 调用）；正文变更 → 重抽重建；
//   - 空正文短路；LLM 失败上抛且不推进 state。

type stubEntityTrack struct {
	entities    []bizknowledge.DocEntity
	entityIDs   []int64
	replaceErr  error
	coocs       []bizknowledge.EntityCooccurrence
	coochErr    error
	links       []bizknowledge.Link
	linkErr     error
	maxDocFreq  int
	replaceCall int
}

func (s *stubEntityTrack) ReplaceDocEntities(_ context.Context, _, _ string, entities []bizknowledge.DocEntity) ([]int64, error) {
	s.replaceCall++
	s.entities = entities
	if s.replaceErr != nil {
		return nil, s.replaceErr
	}
	return s.entityIDs, nil
}

func (s *stubEntityTrack) FindEntityCooccurrences(_ context.Context, _ string, _ []int64, _ string, maxDocFreq int) ([]bizknowledge.EntityCooccurrence, error) {
	s.maxDocFreq = maxDocFreq
	return s.coocs, s.coochErr
}

func (s *stubEntityTrack) ReplaceEntityLinks(_ context.Context, _, _ string, links []bizknowledge.Link) error {
	s.links = links
	return s.linkErr
}

func newEntityPipeline(llm *stubRelationLLM, docs RelationDocReader, track *stubEntityTrack, state *stubRelationStateRepo) *EntityPipeline {
	return NewEntityPipeline(llm, stubRelationSys{}, nil, docs, track, state, loggateway.NewNoop())
}

func TestEntityPipeline_ProcessDoc_WritesEntityLinks(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		`[{"name":"PostgreSQL","type":"tech"},{"name":"流复制","type":"concept"}]`,
	}}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", RelPath: "entries/PostgreSQL 运维.md",
		ContentText: "PostgreSQL 的流复制是高可用的基石，PostgreSQL 运维必读。", ContentHash: "h1",
	}}
	track := &stubEntityTrack{
		entityIDs: []int64{11, 12},
		coocs: []bizknowledge.EntityCooccurrence{
			{DocID: "d2", SharedEntities: []string{"PostgreSQL", "流复制"}},
			{DocID: "d3", SharedEntities: []string{"PostgreSQL"}},
			{DocID: "d1", SharedEntities: []string{"PostgreSQL"}}, // 自环防御：应跳过
		},
	}
	state := &stubRelationStateRepo{}

	stats, err := newEntityPipeline(llm, docs, track, state).
		ProcessDoc(context.Background(), "c1", "d1")
	if err != nil {
		t.Fatalf("ProcessDoc: %v", err)
	}
	if stats.Skipped {
		t.Fatalf("unexpected skip: %s", stats.SkipReason)
	}
	if stats.Entities != 2 || stats.Cooccurrences != 3 || stats.Links != 2 {
		t.Errorf("stats = %+v, want entities=2 cooc=3 links=2", stats)
	}
	// 实体字典写入：PostgreSQL 正文出现 2 次。
	if track.replaceCall != 1 || len(track.entities) != 2 {
		t.Fatalf("ReplaceDocEntities calls=%d entities=%+v", track.replaceCall, track.entities)
	}
	if track.entities[0].Name != "PostgreSQL" || track.entities[0].Mentions != 2 {
		t.Errorf("entity[0] = %+v, want PostgreSQL mentions=2", track.entities[0])
	}
	if track.entities[1].Name != "流复制" || track.entities[1].Mentions != 1 {
		t.Errorf("entity[1] = %+v, want 流复制 mentions=1", track.entities[1])
	}
	// R-3 频次过滤参数透传。
	if track.maxDocFreq != entityMaxDocFreq {
		t.Errorf("maxDocFreq = %d, want %d", track.maxDocFreq, entityMaxDocFreq)
	}
	// entity 出链：context=共享实体名、weight=共享数、自环跳过。
	if len(track.links) != 2 {
		t.Fatalf("links = %+v, want 2", track.links)
	}
	if track.links[0].TargetDocID != "d2" || track.links[0].LinkType != bizknowledge.LinkTypeEntity ||
		track.links[0].Weight != 2 || !strings.Contains(track.links[0].Context, "流复制") {
		t.Errorf("link[0] = %+v, want d2 entity weight=2 ctx含流复制", track.links[0])
	}
	if track.links[1].TargetDocID != "d3" || track.links[1].Weight != 1 {
		t.Errorf("link[1] = %+v, want d3 weight=1", track.links[1])
	}
	// state 推进：entities_extracted_at 落时间。
	if len(state.upserts) != 1 || state.upserts[0].ContentHash != "h1" || state.upserts[0].EntitiesExtractedAt.IsZero() {
		t.Errorf("state upserts = %+v, want hash h1 + entities_extracted_at set", state.upserts)
	}
	if !state.upserts[0].RelationsExtractedAt.IsZero() {
		t.Errorf("entity track must not touch relations_extracted_at: %+v", state.upserts[0])
	}
}

func TestEntityPipeline_ProcessDoc_SkipsUnchanged(t *testing.T) {
	llm := &stubRelationLLM{}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	track := &stubEntityTrack{}
	state := &stubRelationStateRepo{byDoc: map[string]bizknowledge.RelationState{
		"d1": {DocID: "d1", CollectionID: "c1", ContentHash: "h1", EntitiesExtractedAt: time.Now()},
	}}
	stats, err := newEntityPipeline(llm, docs, track, state).
		ProcessDoc(context.Background(), "c1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Skipped || stats.SkipReason != "unchanged" {
		t.Errorf("stats = %+v, want skipped unchanged", stats)
	}
	if llm.calls != 0 || track.replaceCall != 0 {
		t.Errorf("llm=%d track=%d, want 0/0 (idempotent skip)", llm.calls, track.replaceCall)
	}
}

func TestEntityPipeline_ProcessDoc_ReextractsOnContentChange(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{`[]`}}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "new body", ContentHash: "h2",
	}}
	track := &stubEntityTrack{}
	state := &stubRelationStateRepo{byDoc: map[string]bizknowledge.RelationState{
		"d1": {DocID: "d1", CollectionID: "c1", ContentHash: "h1", EntitiesExtractedAt: time.Now()},
	}}
	stats, err := newEntityPipeline(llm, docs, track, state).
		ProcessDoc(context.Background(), "c1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped {
		t.Fatalf("content changed must re-extract, got skip %q", stats.SkipReason)
	}
	if llm.calls != 1 {
		t.Errorf("llm calls = %d, want 1", llm.calls)
	}
	// 空实体：实体记录清空 + entity 出链清空 + state 推进。
	if track.replaceCall != 1 || len(track.entities) != 0 {
		t.Errorf("track entities = %+v, want cleared", track.entities)
	}
	if track.links == nil || len(track.links) != 0 {
		t.Errorf("links = %+v, want empty (clear stale)", track.links)
	}
	if len(state.upserts) != 1 || state.upserts[0].ContentHash != "h2" {
		t.Errorf("state = %+v, want hash h2", state.upserts)
	}
}

func TestEntityPipeline_ProcessDoc_SkipsEmptyBody(t *testing.T) {
	llm := &stubRelationLLM{}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{ID: "d1", CollectionID: "c1"}}
	track := &stubEntityTrack{}
	stats, err := newEntityPipeline(llm, docs, track, &stubRelationStateRepo{}).
		ProcessDoc(context.Background(), "c1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Skipped || stats.SkipReason != "empty_body" {
		t.Errorf("stats = %+v, want skipped empty_body", stats)
	}
	if llm.calls != 0 {
		t.Errorf("empty body must not burn LLM: %d", llm.calls)
	}
}

func TestEntityPipeline_ProcessDoc_LLMErrorPropagates(t *testing.T) {
	llm := &stubRelationLLM{err: errors.New("llm down")}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	track := &stubEntityTrack{}
	state := &stubRelationStateRepo{}
	_, err := newEntityPipeline(llm, docs, track, state).
		ProcessDoc(context.Background(), "c1", "d1")
	if err == nil {
		t.Fatal("LLM failure must propagate (state not advanced, retry next index)")
	}
	if track.replaceCall != 0 || len(state.upserts) != 0 {
		t.Errorf("failure must not write entities/state: %d/%d", track.replaceCall, len(state.upserts))
	}
}

func TestEntityPipeline_ProcessDoc_NotWired(t *testing.T) {
	p := NewEntityPipeline(nil, nil, nil, nil, nil, nil, nil)
	if _, err := p.ProcessDoc(context.Background(), "c1", "d1"); err == nil {
		t.Fatal("unwired pipeline must error, not panic")
	}
}
