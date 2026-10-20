package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
)

// G-13 金标快照门禁的确定性检索器（fixture searcher）。
//
// FixtureCorpus 是一个自洽的检索世界：文档语料 + 每条查询的种子锚点。
// 种子锚点复刻 internal/knowledge/gold_recall_test.go 的最坏假设——混合检索
// 只命中种子文档，其余相关文档必须靠一跳 GraphExpander 补回。因此门禁覆盖
// 的是进程内检索组件的回归：
//
//	编译期成链（AutolinkWikiMentions）→ Chunker 切分 → 意图分流（Instant 跳过
//	扩展）→ GraphExpander 一跳扩展 → MergeSearchResults 合并截断
//
// 真实生产链的 BM25/向量召回在 data 层（PG tsvector/pgvector），CI 无 PG/pgvector
// 依赖不可复现，故这里以锚点替代；检索层自身（成链/切分/意图/扩展/合并）的
// 任何回归都会被快照 diff 的 recall 跌幅捕获。

// FixtureDoc 语料文档：body 为成链前原文，fixture 化时注入 wiki 链接。
type FixtureDoc struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// FixtureCorpus 金标语料：docs + seeds（查询原文 → 种子文档 id）。
// seeds 与 EvalSet 的查询一一对应；缺锚点的查询在 Search 时显式报错，
// 让快照门禁把「语料与评测集漂移」作为失败而非静默 vacuous success。
type FixtureCorpus struct {
	Docs  []FixtureDoc      `json:"docs"`
	Seeds map[string]string `json:"seeds"`
}

// fixtureCollection 是 fixture 世界内固定的 collection id。
const fixtureCollection = "gold-fixture"

// LoadFixtureCorpus 读取并校验金标语料。
func LoadFixtureCorpus(path string) (FixtureCorpus, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return FixtureCorpus{}, fmt.Errorf("eval: read corpus %s: %w", path, err)
	}
	var c FixtureCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		return FixtureCorpus{}, fmt.Errorf("eval: parse corpus %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return FixtureCorpus{}, fmt.Errorf("eval: validate corpus %s: %w", path, err)
	}
	return c, nil
}

func (c FixtureCorpus) validate() error {
	if len(c.Docs) == 0 {
		return errors.New("docs is required")
	}
	ids := make(map[string]struct{}, len(c.Docs))
	for i, d := range c.Docs {
		if strings.TrimSpace(d.ID) == "" {
			return fmt.Errorf("doc %d: id is required", i)
		}
		if _, dup := ids[d.ID]; dup {
			return fmt.Errorf("doc %d: duplicate id %q", i, d.ID)
		}
		ids[d.ID] = struct{}{}
		if strings.TrimSpace(d.Title) == "" {
			return fmt.Errorf("doc %q: title is required (autolink 锚定)", d.ID)
		}
		if strings.TrimSpace(d.Body) == "" {
			return fmt.Errorf("doc %q: body is required", d.ID)
		}
	}
	for q, id := range c.Seeds {
		if strings.TrimSpace(q) == "" {
			return errors.New("seeds: query key is required")
		}
		if _, ok := ids[id]; !ok {
			return fmt.Errorf("seeds: query %q anchors unknown doc %q", q, id)
		}
	}
	return nil
}

// fixtureSearcher 以种子锚点为起点的确定性 Searcher。无词法打分——种子即
// 最坏假设下的混合检索命中，后续全部交给生产扩展/合并组件。
type fixtureSearcher struct {
	chunks   map[string][]biz.KnowledgeChunk // docID → chunks（chunk 序固定）
	links    map[string][]bizknowledge.Link  // docID → outbound explicit links
	seeds    map[string]string
	expander *knowledge.GraphExpander
}

// NewFixtureSearcher 把语料编译为 Searcher：成链 → 切分 → 提取 wiki 边 →
// 接生产 GraphExpander（无 active 边，维持 1 跳旧行为，与 gold_recall_test 同口径）。
func NewFixtureSearcher(c FixtureCorpus) (Searcher, error) {
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("eval: validate corpus: %w", err)
	}
	titles := make([]string, 0, len(c.Docs))
	for _, d := range c.Docs {
		titles = append(titles, d.Title)
	}
	s := &fixtureSearcher{
		chunks: make(map[string][]biz.KnowledgeChunk, len(c.Docs)),
		links:  make(map[string][]bizknowledge.Link, len(c.Docs)),
		seeds:  c.Seeds,
	}
	chunker := knowledge.NewChunker(0, 0, "")
	for _, d := range c.Docs {
		linked, _ := bizknowledge.AutolinkWikiMentions(d.Body, d.Title, titles)
		for i, ch := range chunker.Split(linked) {
			s.chunks[d.ID] = append(s.chunks[d.ID], biz.KnowledgeChunk{
				ID:           fmt.Sprintf("%s-c%d", d.ID, i),
				DocID:        d.ID,
				CollectionID: fixtureCollection,
				Content:      ch.Content,
				Score:        0.9, // 与 gold_recall_test.go 种子分数同口径
			})
		}
		for _, t := range parseWikiTitles(linked) {
			tid := resolveFixtureTitle(c.Docs, t)
			if tid == "" || tid == d.ID {
				continue
			}
			s.links[d.ID] = append(s.links[d.ID], bizknowledge.Link{
				DocID:       d.ID,
				TargetDocID: tid,
				LinkType:    bizknowledge.LinkTypeExplicit,
				Weight:      1,
			})
		}
	}
	s.expander = knowledge.NewGraphExpander(
		fixtureLinkReader{byDoc: s.links},
		fixtureChunkLister{byDoc: s.chunks},
		nil, // GraphExpander 对 nil logger 自降级为 Noop
	)
	return s, nil
}

// Search 以锚点种子 + 一跳扩展回答查询。未登记的查询报错（Runner 记入
// CaseResult.Err 并继续），由快照测试汇总成失败。
func (s *fixtureSearcher) Search(ctx context.Context, query string, topK int) ([]Hit, error) {
	docID, ok := s.seeds[query]
	if !ok {
		return nil, fmt.Errorf("eval: fixture corpus has no seed anchor for query %q", query)
	}
	seeds := s.chunks[docID]
	if len(seeds) == 0 {
		return nil, fmt.Errorf("eval: fixture seed doc %q produced no chunks", docID)
	}
	out := s.expander.Expand(ctx, biz.KnowledgeSearchQuery{
		CollectionID: fixtureCollection,
		Query:        query,
		TopK:         topK,
	}, seeds)
	hits := make([]Hit, 0, len(out))
	for _, ch := range out {
		hits = append(hits, Hit{ChunkID: ch.ID, DocID: ch.DocID, Score: float64(ch.Score)})
	}
	return hits, nil
}

// fixtureLinkReader / fixtureChunkLister 复刻 internal/knowledge/graph_expander_test.go
// 的端口应答（保持两侧口径一致；GraphExpander 依赖失败时自身降级为种子）。
type fixtureLinkReader struct {
	byDoc map[string][]bizknowledge.Link
}

func (f fixtureLinkReader) ListLinks(_ context.Context, _, docID, _ string) ([]bizknowledge.Link, error) {
	return f.byDoc[docID], nil
}

type fixtureChunkLister struct {
	byDoc map[string][]biz.KnowledgeChunk
}

func (f fixtureChunkLister) ListChunksByDocuments(_ context.Context, _ string, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error) {
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

// parseWikiTitles 与 gold_recall_test.go 同规则：提取 [[title]] 提及。
func parseWikiTitles(body string) []string {
	var out []string
	for {
		i := strings.Index(body, "[[")
		if i < 0 {
			break
		}
		j := strings.Index(body[i+2:], "]]")
		if j < 0 {
			break
		}
		out = append(out, body[i+2:i+2+j])
		body = body[i+2+j+2:]
	}
	return out
}

// resolveFixtureTitle 大小写不敏感地把提及定名回文档 id。
func resolveFixtureTitle(docs []FixtureDoc, title string) string {
	for _, d := range docs {
		if strings.EqualFold(d.Title, title) {
			return d.ID
		}
	}
	return ""
}
