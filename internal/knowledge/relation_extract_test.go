package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 自治理图谱 M2.2 两步 LLM 关系抽取契约：
//   - mock LLM 返回实体清单+三元组 → typed 边写入正确 relation/目标/置信度；
//   - 同义实体（"PostgreSQL"/"PG" 经 aliases）解析为同一节点；
//   - confidence<0.7 的边 Closed（留痕不进主图谱）；词表外谓词落 vocab candidate；
//   - content_hash 一致且已抽过 → 跳过（零 LLM 调用）；空实体短路 Step2；
//   - 歧义宾语（多文档同键）不产边；自环不产边。

// ── stubs ─────────────────────────────────────────────────────────────────

type stubRelationLLM struct {
	responses []string // 按调用顺序出队（Step1 → Step2）
	calls     int
	lastUser  string
	err       error
}

func (s *stubRelationLLM) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	s.calls++
	s.lastUser = req.User
	if s.err != nil {
		return "", 0, s.err
	}
	if s.calls > len(s.responses) {
		return "", 0, errors.New("unexpected LLM call")
	}
	return s.responses[s.calls-1], 0, nil
}

type stubRelationSys struct{}

func (stubRelationSys) Get(context.Context) (biz.SystemSetting, error) {
	return biz.SystemSetting{DefaultRefineLLM: biz.RefineLLMSetting{Provider: "p", Model: "m"}}, nil
}

type stubRelationDocReader struct {
	doc bizknowledge.Document
	err error
}

func (s *stubRelationDocReader) GetDocument(context.Context, string) (bizknowledge.Document, error) {
	return s.doc, s.err
}

type stubSemanticLinks struct {
	calls       int
	collection  string
	docID       string
	links       []bizknowledge.SemanticLink
	err         error
}

func (s *stubSemanticLinks) ReplaceSemanticLinks(_ context.Context, collectionID, docID string, links []bizknowledge.SemanticLink) error {
	s.calls++
	s.collection, s.docID, s.links = collectionID, docID, links
	return s.err
}

type stubRelationVocab struct {
	upserts []string
}

func (s *stubRelationVocab) UpsertCandidate(_ context.Context, relation, _ string) error {
	s.upserts = append(s.upserts, relation)
	return nil
}

type stubRelationStateRepo struct {
	byDoc   map[string]bizknowledge.RelationState
	upserts []bizknowledge.RelationState
}

func (s *stubRelationStateRepo) GetRelationState(_ context.Context, docID string) (bizknowledge.RelationState, bool, error) {
	st, ok := s.byDoc[docID]
	return st, ok, nil
}

func (s *stubRelationStateRepo) UpsertRelationState(_ context.Context, st bizknowledge.RelationState) error {
	if s.byDoc == nil {
		s.byDoc = map[string]bizknowledge.RelationState{}
	}
	s.byDoc[st.DocID] = st
	s.upserts = append(s.upserts, st)
	return nil
}

type stubRelationResolver struct {
	cands []bizknowledge.ResolveDocCandidate
	err   error
}

func (s *stubRelationResolver) ListResolveCandidates(context.Context, []string) ([]bizknowledge.ResolveDocCandidate, error) {
	return s.cands, s.err
}

// ── 用例 ──────────────────────────────────────────────────────────────────

func newRelationExtractor(llm *stubRelationLLM, docs RelationDocReader, links *stubSemanticLinks, vocab *stubRelationVocab, state *stubRelationStateRepo, resolver RelationObjectResolver) *RelationExtractor {
	return NewRelationExtractor(llm, stubRelationSys{}, nil, docs, links, vocab, state, resolver, loggateway.NewNoop())
}

func TestRelationExtractor_ExtractDoc_WritesTypedEdges(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		`[{"name":"PostgreSQL","type":"tech"},{"name":"流复制","type":"concept"}]`,
		`[
		  {"subject":"流复制","predicate":"part-of","object":"PG","confidence":0.9,"evidence":"流复制是 PostgreSQL 的内建能力"},
		  {"subject":"PostgreSQL","predicate":"depends-on","object":"流复制","confidence":0.6,"evidence":"高可用依赖流复制"},
		  {"subject":"PostgreSQL","predicate":"uses","object":"流复制","confidence":0.8,"evidence":"PostgreSQL 使用流复制做备库同步"},
		  {"subject":"PostgreSQL","predicate":"is-a","object":"不存在的东西","confidence":0.95,"evidence":"未登记实体应跳过"},
		  {"subject":"PostgreSQL","predicate":"is-a","object":"PostgreSQL 运维","confidence":0.95,"evidence":"自环应跳过"}
		]`,
	}}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", RelPath: "entries/PostgreSQL 运维.md",
		ContentText: "PostgreSQL 的流复制是高可用的基石。", ContentHash: "h1",
	}}
	links := &stubSemanticLinks{}
	vocab := &stubRelationVocab{}
	state := &stubRelationStateRepo{}
	resolver := &stubRelationResolver{cands: []bizknowledge.ResolveDocCandidate{
		{DocID: "d1", CollectionID: "c1", RelPath: "entries/PostgreSQL 运维.md", Title: "PostgreSQL 运维"},
		{DocID: "d2", CollectionID: "c1", RelPath: "entries/PostgreSQL.md", Aliases: []string{"PG"}},
		{DocID: "d3", CollectionID: "c1", RelPath: "entries/流复制.md"},
	}}

	stats, err := newRelationExtractor(llm, docs, links, vocab, state, resolver).
		ExtractDoc(context.Background(), "d1")
	if err != nil {
		t.Fatalf("ExtractDoc: %v", err)
	}
	if stats.Skipped {
		t.Fatalf("unexpected skip: %s", stats.SkipReason)
	}
	if stats.Entities != 2 || stats.Triples != 5 {
		t.Errorf("stats entities/triples = %d/%d, want 2/5", stats.Entities, stats.Triples)
	}
	// 同义实体：宾语 "PG" 经 alias 解析到 d2（与 "PostgreSQL" 同节点）。
	// 未知宾语/自环跳过；候选谓词 uses 落 vocab。
	if links.calls != 1 || links.collection != "c1" || links.docID != "d1" {
		t.Fatalf("ReplaceSemanticLinks calls=%d coll=%q doc=%q", links.calls, links.collection, links.docID)
	}
	if len(links.links) != 3 {
		t.Fatalf("links = %+v, want 3 edges", links.links)
	}
	byTargetRel := map[string]bizknowledge.SemanticLink{}
	for _, l := range links.links {
		byTargetRel[l.TargetDocID+"/"+l.Relation] = l
	}
	open, ok := byTargetRel["d2/part-of"]
	if !ok || open.Closed || open.Confidence != 0.9 {
		t.Errorf("d2/part-of = %+v ok=%v, want open edge conf 0.9", open, ok)
	}
	if !strings.Contains(open.Context, "流复制") {
		t.Errorf("edge context should carry subject evidence, got %q", open.Context)
	}
	closed, ok := byTargetRel["d3/depends-on"]
	if !ok || !closed.Closed {
		t.Errorf("d3/depends-on conf 0.6 < 0.7 must be Closed, got %+v ok=%v", closed, ok)
	}
	cand, ok := byTargetRel["d3/uses"]
	if !ok || cand.Closed {
		t.Errorf("d3/uses (candidate predicate) = %+v ok=%v, want open edge", cand, ok)
	}
	if len(vocab.upserts) != 1 || vocab.upserts[0] != "uses" {
		t.Errorf("vocab upserts = %v, want [uses]", vocab.upserts)
	}
	if stats.Candidates != 1 || stats.Links != 3 || stats.OpenLinks != 2 {
		t.Errorf("stats = %+v, want candidates=1 links=3 open=2", stats)
	}
	// state 推进（content_hash 登记 + 抽取时间）。
	if len(state.upserts) != 1 || state.upserts[0].ContentHash != "h1" || state.upserts[0].RelationsExtractedAt.IsZero() {
		t.Errorf("state upserts = %+v, want hash h1 + extracted_at set", state.upserts)
	}
	// 两步共用同一文档正文（Step2 user 含实体清单）。
	if !strings.Contains(llm.lastUser, "PostgreSQL") || !strings.Contains(llm.lastUser, "实体清单") {
		t.Errorf("step2 user prompt missing entities/body: %q", llm.lastUser)
	}
}

func TestRelationExtractor_ExtractDoc_SkipsUnchanged(t *testing.T) {
	llm := &stubRelationLLM{}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	links := &stubSemanticLinks{}
	state := &stubRelationStateRepo{byDoc: map[string]bizknowledge.RelationState{
		"d1": {DocID: "d1", CollectionID: "c1", ContentHash: "h1", RelationsExtractedAt: time.Now()},
	}}
	stats, err := newRelationExtractor(llm, docs, links, &stubRelationVocab{}, state, &stubRelationResolver{}).
		ExtractDoc(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Skipped || stats.SkipReason != "unchanged" {
		t.Errorf("stats = %+v, want skipped unchanged", stats)
	}
	if llm.calls != 0 || links.calls != 0 {
		t.Errorf("llm calls=%d link calls=%d, want 0/0 (idempotent skip)", llm.calls, links.calls)
	}
}

func TestRelationExtractor_ExtractDoc_ReextractsOnContentChange(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{`[]`}}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "new body", ContentHash: "h2",
	}}
	links := &stubSemanticLinks{}
	state := &stubRelationStateRepo{byDoc: map[string]bizknowledge.RelationState{
		"d1": {DocID: "d1", CollectionID: "c1", ContentHash: "h1", RelationsExtractedAt: time.Now()},
	}}
	stats, err := newRelationExtractor(llm, docs, links, &stubRelationVocab{}, state, &stubRelationResolver{}).
		ExtractDoc(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped {
		t.Fatalf("content changed must re-extract, got skip %q", stats.SkipReason)
	}
	// 空实体 → Step2 短路（仅 1 次 LLM 调用），出链清空，state 推进。
	if llm.calls != 1 {
		t.Errorf("llm calls = %d, want 1 (step2 short-circuited)", llm.calls)
	}
	if links.calls != 1 || len(links.links) != 0 {
		t.Errorf("links calls=%d n=%d, want 1/0 (clear stale edges)", links.calls, len(links.links))
	}
	if len(state.upserts) != 1 || state.upserts[0].ContentHash != "h2" {
		t.Errorf("state = %+v, want hash h2", state.upserts)
	}
}

func TestRelationExtractor_ExtractDoc_AmbiguousObjectSkipped(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		`["PostgreSQL"]`,
		`[{"subject":"x","predicate":"is-a","object":"PG","confidence":0.9}]`,
	}}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	links := &stubSemanticLinks{}
	resolver := &stubRelationResolver{cands: []bizknowledge.ResolveDocCandidate{
		{DocID: "d2", CollectionID: "c1", Aliases: []string{"PG"}},
		{DocID: "d3", CollectionID: "c1", Aliases: []string{"pg"}},
	}}
	stats, err := newRelationExtractor(llm, docs, links, &stubRelationVocab{}, &stubRelationStateRepo{}, resolver).
		ExtractDoc(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if links.calls != 1 || len(links.links) != 0 {
		t.Errorf("ambiguous object must yield no edges, got %+v", links.links)
	}
	if stats.Triples != 1 || stats.Links != 0 {
		t.Errorf("stats = %+v, want triples=1 links=0", stats)
	}
}

func TestRelationExtractor_ExtractDoc_SkipsWithoutResolver(t *testing.T) {
	llm := &stubRelationLLM{}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	links := &stubSemanticLinks{}
	stats, err := newRelationExtractor(llm, docs, links, &stubRelationVocab{}, &stubRelationStateRepo{}, nil).
		ExtractDoc(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Skipped || stats.SkipReason != "no_resolver" {
		t.Errorf("stats = %+v, want skipped no_resolver", stats)
	}
	if llm.calls != 0 || links.calls != 0 {
		t.Errorf("resolver missing must not burn LLM: calls %d/%d", llm.calls, links.calls)
	}
}

func TestRelationExtractor_ExtractDoc_LLMErrorPropagates(t *testing.T) {
	llm := &stubRelationLLM{err: errors.New("llm down")}
	docs := &stubRelationDocReader{doc: bizknowledge.Document{
		ID: "d1", CollectionID: "c1", ContentText: "body", ContentHash: "h1",
	}}
	links := &stubSemanticLinks{}
	state := &stubRelationStateRepo{}
	_, err := newRelationExtractor(llm, docs, links, &stubRelationVocab{}, state, &stubRelationResolver{}).
		ExtractDoc(context.Background(), "d1")
	if err == nil {
		t.Fatal("LLM failure must propagate (state not advanced, retry next cycle)")
	}
	if links.calls != 0 || len(state.upserts) != 0 {
		t.Errorf("failure must not write links/state: %d/%d", links.calls, len(state.upserts))
	}
}

func TestNormalizePredicate(t *testing.T) {
	for in, want := range map[string]string{
		"Is-A":         "is-a",
		"part_of":      "part-of",
		"depends on":   "depends-on",
		"  causes  ":   "causes",
		"evolves from": "evolves-from",
		"":             "",
	} {
		if got := normalizePredicate(in); got != want {
			t.Errorf("normalizePredicate(%q) = %q, want %q", in, got, want)
		}
	}
	if !isCoreRelation("is-a") || isCoreRelation("uses") {
		t.Error("core relation membership broken")
	}
}
