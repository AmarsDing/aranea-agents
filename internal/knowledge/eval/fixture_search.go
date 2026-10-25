package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

// FixtureCardDecl 语料卡片声明（G-33，ADR-KN-20261020-H）：一条图谱卡片命中
// 经宿主投影产出的候选。声明即接线（NewFixtureSearcher 自动装配桩）；关腿
// 消融走 NewFixtureSearcherAblated。
type FixtureCardDecl struct {
	Kind string `json:"kind"` // entity | edge（bizknowledge.GraphCardKind*）
	Ref  int64  `json:"ref"`
	Text string `json:"text"`
	Host string `json:"host"` // 宿主文档 id（必须指向 docs）
}

// FixtureEdgeDecl 语料时态边声明（G-05-asof）：一条 knowledge_links 式双时态
// 关联。声明即接线——NewFixtureSearcher 在声明了 edges 时自动装配 active（当前
// 快照）+ temporal（四象限谓词）双腿进生产 GraphExpander，与装配层同款断言。
type FixtureEdgeDecl struct {
	ID         string `json:"id"`
	Doc        string `json:"doc"`
	Target     string `json:"target"`
	LinkType   string `json:"link_type"` // explicit | semantic | entity（co_activated 显式禁止）
	Relation   string `json:"relation,omitempty"`
	ValidFrom  string `json:"valid_from"`  // RFC3339
	ValidTo    string `json:"valid_to,omitempty"` // 缺省 = 存续
	RecordedAt string `json:"recorded_at"` // RFC3339
}

// FixtureCorpus 金标语料：docs + seeds（查询原文 → 种子文档 id）。
// seeds 与 EvalSet 的查询一一对应；缺锚点的查询在 Search 时显式报错，
// 让快照门禁把「语料与评测集漂移」作为失败而非静默 vacuous success。
// cards 为可选扩展（G-33 实体/边卡第三腿探针——非 strict Unmarshal 零破坏）。
// edges 为可选时态扩展（G-05-asof 四象限探针——未声明的语料双腿不接线，
// 既有 14 用例快照字节级不变）。
type FixtureCorpus struct {
	Docs  []FixtureDoc                 `json:"docs"`
	Seeds map[string]string            `json:"seeds"`
	Cards map[string][]FixtureCardDecl `json:"cards,omitempty"`
	Edges []FixtureEdgeDecl            `json:"edges,omitempty"`
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
	for q, decls := range c.Cards {
		if strings.TrimSpace(q) == "" {
			return errors.New("cards: query key is required")
		}
		for i, cd := range decls {
			if cd.Kind != bizknowledge.GraphCardKindEntity && cd.Kind != bizknowledge.GraphCardKindEdge {
				return fmt.Errorf("cards: query %q decl %d: kind %q must be entity/edge", q, i, cd.Kind)
			}
			if cd.Ref <= 0 {
				return fmt.Errorf("cards: query %q decl %d: ref must be positive", q, i)
			}
			if strings.TrimSpace(cd.Text) == "" {
				return fmt.Errorf("cards: query %q decl %d: text is required", q, i)
			}
			if _, ok := ids[cd.Host]; !ok {
				return fmt.Errorf("cards: query %q decl %d: unknown host doc %q", q, i, cd.Host)
			}
		}
	}
	edgeIDs := make(map[string]struct{}, len(c.Edges))
	for i, e := range c.Edges {
		if strings.TrimSpace(e.ID) == "" {
			return fmt.Errorf("edges: decl %d: id is required", i)
		}
		if _, dup := edgeIDs[e.ID]; dup {
			return fmt.Errorf("edges: decl %d: duplicate id %q", i, e.ID)
		}
		edgeIDs[e.ID] = struct{}{}
		if _, ok := ids[e.Doc]; !ok {
			return fmt.Errorf("edges: decl %q: unknown doc %q", e.ID, e.Doc)
		}
		if _, ok := ids[e.Target]; !ok {
			return fmt.Errorf("edges: decl %q: unknown target %q", e.ID, e.Target)
		}
		switch strings.TrimSpace(e.LinkType) {
		case bizknowledge.LinkTypeExplicit, bizknowledge.LinkTypeSemantic, bizknowledge.LinkTypeEntity:
		case bizknowledge.LinkTypeCoActivated:
			// 时态端口一刀切排除 co_activated（ADR-KN-20261020-J）——fixture 声明
			// 该类型等于永远观测不到的谎言，语法期而非运行期爆出。
			return fmt.Errorf("edges: decl %q: co_activated edges are not observable through as-of ports", e.ID)
		default:
			return fmt.Errorf("edges: decl %q: link_type %q must be explicit/semantic/entity", e.ID, e.LinkType)
		}
	}
	// 时间审计语法单源：与 buildFixtureSearcher 装配共用同一解析器。
	if _, err := parseFixtureEdges(c.Edges); err != nil {
		return err
	}
	return nil
}

// fixtureEdge 编译态时态边：RFC3339 声明已解析为 time.Time（validTo 零值 = 存续）。
type fixtureEdge struct {
	id, doc, target, linkType, relation string
	validFrom, validTo, recordedAt      time.Time
}

// parseFixtureEdges 校验并解析边时间审计三列（valid_to 可空，其余两列 RFC3339
// 必填）。validate 与装配共用，防止「加载能过、装配爆」双口径漂移。
func parseFixtureEdges(decls []FixtureEdgeDecl) ([]fixtureEdge, error) {
	out := make([]fixtureEdge, 0, len(decls))
	for _, d := range decls {
		vf, err := time.Parse(time.RFC3339, strings.TrimSpace(d.ValidFrom))
		if err != nil {
			return nil, fmt.Errorf("edges: decl %q: valid_from must be RFC3339, got %q", d.ID, d.ValidFrom)
		}
		var vt time.Time
		if raw := strings.TrimSpace(d.ValidTo); raw != "" {
			vt, err = time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, fmt.Errorf("edges: decl %q: valid_to must be RFC3339, got %q", d.ID, d.ValidTo)
			}
		}
		rc, err := time.Parse(time.RFC3339, strings.TrimSpace(d.RecordedAt))
		if err != nil {
			return nil, fmt.Errorf("edges: decl %q: recorded_at must be RFC3339, got %q", d.ID, d.RecordedAt)
		}
		out = append(out, fixtureEdge{
			id: d.ID, doc: d.Doc, target: d.Target,
			linkType: strings.TrimSpace(d.LinkType), relation: d.Relation,
			validFrom: vf, validTo: vt, recordedAt: rc,
		})
	}
	return out, nil
}

// visibleAsOf 复刻 data 层四象限谓词（knowledge_links_asof.go 的 Go 镜像，
// 与 SQL 文本同一规范：valid_to 排他、recorded_at 含端点、dual-OR 双分支）。
func (e fixtureEdge) visibleAsOf(span bizknowledge.TemporalSpan) bool {
	tOn, rOn := span.ValidOn(), span.RecordedOn()
	switch {
	case tOn && rOn:
		if e.recordedAt.After(span.RecordedAt) {
			return false
		}
		inside := !e.validFrom.After(span.ValidAt) && (e.validTo.IsZero() || e.validTo.After(span.ValidAt))
		knownOpen := !e.validTo.IsZero() && !e.validFrom.After(span.ValidAt) && e.validTo.After(span.RecordedAt)
		return inside || knownOpen
	case tOn:
		return !e.validFrom.After(span.ValidAt) && (e.validTo.IsZero() || e.validTo.After(span.ValidAt))
	case rOn:
		return !e.recordedAt.After(span.RecordedAt) && (e.validTo.IsZero() || e.validTo.After(span.RecordedAt))
	default:
		return e.validTo.IsZero()
	}
}

// fixtureEdgeSet 双腿共享的边存储（生产装配亦然：active/temporal 两端口指向
// 同一 repo）。
type fixtureEdgeSet struct {
	edges []fixtureEdge
}

func (f fixtureEdgeSet) touching(docIDs []string) []fixtureEdge {
	in := make(map[string]struct{}, len(docIDs))
	for _, id := range docIDs {
		in[id] = struct{}{}
	}
	var out []fixtureEdge
	for _, e := range f.edges {
		_, docIn := in[e.doc]
		_, tgtIn := in[e.target]
		if docIn || tgtIn {
			out = append(out, e)
		}
	}
	return out
}

func toFixtureActiveLinks(es []fixtureEdge) []bizknowledge.ActiveLink {
	out := make([]bizknowledge.ActiveLink, 0, len(es))
	for _, e := range es {
		out = append(out, bizknowledge.ActiveLink{
			DocID: e.doc, TargetDocID: e.target,
			LinkType: e.linkType, Relation: e.relation, WeightF: 1,
		})
	}
	return out
}

func (f fixtureEdgeSet) currentView(docIDs []string) []bizknowledge.ActiveLink {
	var kept []fixtureEdge
	for _, e := range f.touching(docIDs) {
		if e.validTo.IsZero() {
			kept = append(kept, e)
		}
	}
	return toFixtureActiveLinks(kept)
}

func (f fixtureEdgeSet) asOfView(docIDs []string, span bizknowledge.TemporalSpan) []bizknowledge.ActiveLink {
	if !span.Any() {
		return f.currentView(docIDs) // 零 span 字节级委托（与 data 层同约）
	}
	var kept []fixtureEdge
	for _, e := range f.touching(docIDs) {
		if e.visibleAsOf(span) {
			kept = append(kept, e)
		}
	}
	return toFixtureActiveLinks(kept)
}

// fixtureActiveLinkReader / fixtureTemporalLinkReader 复刻生产装配的双端口。
type fixtureActiveLinkReader struct{ edges fixtureEdgeSet }

func (r fixtureActiveLinkReader) ListActiveLinks(_ context.Context, _ string, docIDs []string) ([]bizknowledge.ActiveLink, error) {
	return r.edges.currentView(docIDs), nil
}

type fixtureTemporalLinkReader struct{ edges fixtureEdgeSet }

func (r fixtureTemporalLinkReader) ListActiveLinksAsOf(_ context.Context, _ string, docIDs []string, span bizknowledge.TemporalSpan) ([]bizknowledge.ActiveLink, error) {
	return r.edges.asOfView(docIDs, span), nil
}

var (
	_ bizknowledge.ActiveLinkReader         = fixtureActiveLinkReader{}
	_ bizknowledge.TemporalActiveLinkReader = fixtureTemporalLinkReader{}
)

// fixtureSearcher 以种子锚点为起点的确定性 Searcher。无词法打分——种子即
// 最坏假设下的混合检索命中，后续全部交给生产扩展/合并组件。cards 为 G-33
// 图谱卡片第三腿桩（nil=关腿，与装配层 nil 关腿语义同款）。
type fixtureSearcher struct {
	chunks   map[string][]biz.KnowledgeChunk // docID → chunks（chunk 序固定）
	links    map[string][]bizknowledge.Link  // docID → outbound explicit links
	seeds    map[string]string
	cards    knowledge.GraphCardSearcher
	expander *knowledge.GraphExpander
}

// fixtureCardSearcher 以语料声明卡片充当生产 GraphCardSearcher 桩（vec 忽略，
// 与 nil 关腿装配语义并列：桩必须在接口层面与生产检索器消费的端口同型）。
type fixtureCardSearcher struct {
	byQuery map[string][]biz.KnowledgeChunk
}

func (f fixtureCardSearcher) SearchGraphCardChunks(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	return f.byQuery[q.Query], nil
}

var _ knowledge.GraphCardSearcher = fixtureCardSearcher{}

// fixtureCardChunkID 复刻生产合成 ID 契约（ADR-KN-20261020-H：g33e/g33r 前缀
// 宿主投影候选一并走 side-channel metadata 标记——gen/消费两端格式钉死）。
func fixtureCardChunkID(kind string, ref int64, host string) string {
	if kind == bizknowledge.GraphCardKindEdge {
		return fmt.Sprintf("g33r/%d/%s", ref, host)
	}
	return fmt.Sprintf("g33e/%d/%s", ref, host)
}

// fixtureCardChunk 按生产候选构造规则造卡块：ID 合成、DocID/CollectionID 为
// 真实宿主、metadata g33 标记（键序与 json.Marshal map 字母序一致，钉字面量）。
func fixtureCardChunk(d FixtureCardDecl) biz.KnowledgeChunk {
	return biz.KnowledgeChunk{
		ID:           fixtureCardChunkID(d.Kind, d.Ref, d.Host),
		DocID:        d.Host,
		CollectionID: fixtureCollection,
		Content:      d.Text,
		Score:        0.9, // 与种子分同口径，保证快照确定
		EntityScore:  0.9,
		MetadataJSON: fmt.Sprintf(`{"g33":%q,"g33_ref":%d}`, d.Kind, d.Ref),
	}
}

// NewFixtureSearcher 把语料编译为 Searcher：成链 → 切分 → 提取 wiki 边 →
// 接生产 GraphExpander（无 active 边，维持 1 跳旧行为，与 gold_recall_test 同口径）。
// 语料声明了 cards 时自动装配卡片腿桩（声明即接线）。
func NewFixtureSearcher(c FixtureCorpus) (Searcher, error) {
	return buildFixtureSearcher(c, true)
}

// NewFixtureSearcherAblated 同语料但强制关卡片腿——消融对照（ADR-KN-20261020-I
// 测试策略：装配 nil 关腿复现基线，证明探针用例的命中确由第三腿贡献）。
func NewFixtureSearcherAblated(c FixtureCorpus) (Searcher, error) {
	return buildFixtureSearcher(c, false)
}

func buildFixtureSearcher(c FixtureCorpus, withCards bool) (Searcher, error) {
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
	// G-05-asof：声明式时态边（决策 c）——edges 非空即双腿接线（active 当前快照 +
	// temporal 四象限），随后整库扩散走 spreadExpand（含零 span 零改动委托语义）；
	// 未声明语料双腿不接线，既有 1 跳行为与快照字节级不变。
	if len(c.Edges) > 0 {
		parsed, err := parseFixtureEdges(c.Edges)
		if err != nil {
			return nil, fmt.Errorf("eval: parse fixture edges: %w", err)
		}
		set := fixtureEdgeSet{edges: parsed}
		s.expander.SetActiveLinks(fixtureActiveLinkReader{edges: set})
		s.expander.SetTemporalActiveLinks(fixtureTemporalLinkReader{edges: set})
	}
	if withCards && len(c.Cards) > 0 {
		byQuery := make(map[string][]biz.KnowledgeChunk, len(c.Cards))
		for q, decls := range c.Cards {
			for _, d := range decls {
				byQuery[q] = append(byQuery[q], fixtureCardChunk(d))
			}
		}
		s.cards = fixtureCardSearcher{byQuery: byQuery}
	}
	return s, nil
}

// Search 以锚点种子 ∪ 卡片腿候选 + 一跳扩展回答查询。未登记的查询报错
// （Runner 记入 CaseResult.Err 并继续），由快照测试汇总成失败。卡片腿失败
// 复刻生产语义（warn 降级为种子集），fixture 桩实际不失败。
func (s *fixtureSearcher) Search(ctx context.Context, query string, topK int) ([]Hit, error) {
	return s.searchOwned(ctx, query, topK, bizknowledge.TemporalSpan{})
}

// SearchAsOf 实现 eval.TemporalSearcher（G-05-asof）：同 Search 但携带时态窗口，
// 经 SearchQuery.Temporal 值拷贝进生产 GraphExpander 的时态腿。零 span 字节级委托。
func (s *fixtureSearcher) SearchAsOf(ctx context.Context, query string, topK int, span bizknowledge.TemporalSpan) ([]Hit, error) {
	return s.searchOwned(ctx, query, topK, span)
}

var _ TemporalSearcher = (*fixtureSearcher)(nil)

func (s *fixtureSearcher) searchOwned(ctx context.Context, query string, topK int, span bizknowledge.TemporalSpan) ([]Hit, error) {
	docID, ok := s.seeds[query]
	if !ok {
		return nil, fmt.Errorf("eval: fixture corpus has no seed anchor for query %q", query)
	}
	seeds := s.chunks[docID]
	if len(seeds) == 0 {
		return nil, fmt.Errorf("eval: fixture seed doc %q produced no chunks", docID)
	}
	q := biz.KnowledgeSearchQuery{
		CollectionID: fixtureCollection,
		Query:        query,
		TopK:         topK,
		Temporal:     span,
	}
	// 拷贝再并，避免 append 写回种子底层数组（多查询复跑同一 docID 时重复卡片）。
	cands := append([]biz.KnowledgeChunk{}, seeds...)
	if s.cards != nil {
		if cardChunks, err := s.cards.SearchGraphCardChunks(ctx, q, nil); err == nil {
			cands = append(cands, cardChunks...)
		}
	}
	out := s.expander.Expand(ctx, q, cands)
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
