// Package knowledge — 自治理知识图谱 M2.2：两步 LLM 关系抽取。
//
// 管线（每文档，幂等）：
//
//	词条 body
//	 → [Step1 实体抽取] LLM 抽实体清单（名词短语）
//	 → [Step2 三元组抽取] 基于实体清单抽 (主语, 谓词, 宾语, confidence)
//	 → [归一化] 谓词归一到核心闭集（CoreRelations）；词表外谓词落 vocab candidate 层
//	 → [宾语解析] 宾语实体 → 同库文档（basename/title/aliases 多键，歧义跳过）
//	 → 写 knowledge_links (link_type=semantic, relation=谓词, confidence)
//
// 成本闸门：只对高价值词条抽取（worker 经 HotDocumentLister 选_doc_），
// 且按 content_hash 幂等（正文未变不重抽）。confidence < 0.7 的边写入即关闭
// （valid_to=now，仅留痕不进主图谱）。KGGen 式嵌入聚类归并推迟到 M4 治理周期；
// M2 归一化依赖 vault 既有 basename/title/aliases 解析键（与 autolink/mention 同语义）。
package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

const (
	// relationExtractTimeout 单次 LLM 调用超时（Step1/Step2 各自计时）。
	// deepseek 高峰期单请求 >60s 常见（2026-08-18 评测批量超时），放宽到 180s。
	relationExtractTimeout = 180 * time.Second
	// relationMaxEntities Step1 实体清单上限（控 Step2 prompt 规模）。
	relationMaxEntities = 20
	// relationMaxTriples Step2 三元组上限（控单文档边数）。
	relationMaxTriples = 16
	// relationBodyMaxRunes 抽取输入正文截断（控 token 成本）。
	relationBodyMaxRunes = 6000
	// relationMinConfidence 主图谱置信度门槛；低于此值边写入即关闭（仅留痕）。
	relationMinConfidence = 0.7
	// relationContextMaxRunes 边证据（context 列）截断。
	relationContextMaxRunes = 240
)

// RelationDocReader 抽取输入文档读取窄接口（生产实现：bizknowledge.Usecase）。
type RelationDocReader interface {
	GetDocument(ctx context.Context, id string) (bizknowledge.Document, error)
}

// RelationObjectResolver 宾语实体 → 同库候选文档窄接口
// （生产实现：data.knowledgeBlockRepo，与 autolink/mention 共用解析键）。
type RelationObjectResolver interface {
	ListResolveCandidates(ctx context.Context, collectionIDs []string) ([]bizknowledge.ResolveDocCandidate, error)
}

// RelationExtractStats 单文档抽取统计（worker 对账/日志用）。
type RelationExtractStats struct {
	Entities   int    // Step1 抽取实体数（截断后）
	Triples    int    // Step2 抽取三元组数（截断后）
	Links      int    // 落库 semantic 出链数（开+闭）
	OpenLinks  int    // 其中进主图谱（confidence >= 门槛）的边数
	Candidates int    // 词表外谓词数（落 vocab candidate 层）
	Skipped    bool   // true = 未执行抽取（原因见 SkipReason）
	SkipReason string // unchanged / empty_body / no_resolver
}

// RelationExtractor 两步 LLM 关系抽取器。全部外部依赖经窄接口注入；
// LLM 缺失/调用失败上抛 error（worker 记录，state 不推进，下周期重试）。
type RelationExtractor struct {
	llm      biz.LLMCaller
	sys      RefineLLMSettingsGetter
	catalog  LLMCatalogLister
	docs     RelationDocReader
	links    bizknowledge.SemanticLinkRepo
	vocab    bizknowledge.RelationVocabRepo
	state    bizknowledge.RelationStateRepo
	resolver RelationObjectResolver
	lg       loggateway.Logger
}

// NewRelationExtractor 构造抽取器；lg 为 nil 时降级 Noop。
func NewRelationExtractor(
	llm biz.LLMCaller,
	sys RefineLLMSettingsGetter,
	catalog LLMCatalogLister,
	docs RelationDocReader,
	links bizknowledge.SemanticLinkRepo,
	vocab bizknowledge.RelationVocabRepo,
	state bizknowledge.RelationStateRepo,
	resolver RelationObjectResolver,
	lg loggateway.Logger,
) *RelationExtractor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RelationExtractor{
		llm: llm, sys: sys, catalog: catalog, docs: docs,
		links: links, vocab: vocab, state: state, resolver: resolver, lg: lg,
	}
}

// ExtractDoc 对单文档执行两步关系抽取（幂等：content_hash 一致且已抽过则跳过）。
// 派生索引纪律：semantic 出链随文档内容整体重建（ReplaceSemanticLinks 删旧插新）。
func (e *RelationExtractor) ExtractDoc(ctx context.Context, docID string) (RelationExtractStats, error) {
	var stats RelationExtractStats
	if e == nil || e.llm == nil || e.docs == nil || e.links == nil || e.state == nil {
		return stats, fmt.Errorf("relation extractor not fully wired (llm/docs/links/state required)")
	}
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return stats, fmt.Errorf("doc id required")
	}
	if e.resolver == nil {
		// 宾语解析键缺失 = 装配缺陷；跳过且不推进 state，接线修复后自然重抽。
		stats.Skipped, stats.SkipReason = true, "no_resolver"
		return stats, nil
	}

	doc, err := e.docs.GetDocument(ctx, docID)
	if err != nil {
		return stats, fmt.Errorf("get document %s: %w", docID, err)
	}
	body := strings.TrimSpace(doc.ContentText)
	if body == "" {
		stats.Skipped, stats.SkipReason = true, "empty_body"
		return stats, nil
	}
	contentHash := strings.TrimSpace(doc.ContentHash)
	if contentHash == "" {
		sum := sha1.Sum([]byte(body))
		contentHash = hex.EncodeToString(sum[:])
	}

	// 幂等闸门：正文未变且已抽过关系 → 跳过（控 LLM 成本）。
	st, found, err := e.state.GetRelationState(ctx, docID)
	if err != nil {
		return stats, fmt.Errorf("get relation state %s: %w", docID, err)
	}
	if found && st.ContentHash == contentHash && !st.RelationsExtractedAt.IsZero() {
		stats.Skipped, stats.SkipReason = true, "unchanged"
		return stats, nil
	}

	provider, model, err := ResolveLLM(ctx, e.sys, e.catalog, "relation extract", e.lg)
	if err != nil {
		return stats, err
	}
	body = truncateRunes(body, relationBodyMaxRunes)
	title := docDisplayName(doc.RelPath, doc.Source)

	// Step1：实体抽取。
	entities, err := llmExtractEntities(ctx, e.llm, provider, model, title, body)
	if err != nil {
		return stats, fmt.Errorf("extract entities %s: %w", docID, err)
	}
	stats.Entities = len(entities)

	// Step2：三元组抽取（无实体时短路，省下一次 LLM 调用）。
	var triples []extractTriple
	if len(entities) > 0 {
		triples, err = e.extractTriples(ctx, provider, model, title, body, entities)
		if err != nil {
			return stats, fmt.Errorf("extract triples %s: %w", docID, err)
		}
	}
	stats.Triples = len(triples)

	// 谓词归一化 + 词表外落 candidate；宾语实体 → 同库文档。
	keyIdx, err := e.buildDocKeyIndex(ctx, doc.CollectionID)
	if err != nil {
		return stats, fmt.Errorf("resolve candidates %s: %w", doc.CollectionID, err)
	}
	links := make([]bizknowledge.SemanticLink, 0, len(triples))
	for _, t := range triples {
		// 关系必须能回指正文证据；LLM 自报 evidence 不在源文中时拒绝发布，
		// 防止高 confidence 幻觉边进入扩散激活主图。
		if !relationEvidenceSupported(body, t.Evidence) {
			continue
		}
		relation := normalizePredicate(t.Predicate)
		if relation == "" {
			continue
		}
		if !isCoreRelation(relation) {
			stats.Candidates++
			if e.vocab != nil {
				if uerr := e.vocab.UpsertCandidate(ctx, relation, "llm"); uerr != nil {
					e.lg.Warn("谓词候选词表登记失败",
						loggateway.StepID("knowledge.relation_extract.vocab"),
						loggateway.Str("relation", relation),
						loggateway.Err(uerr))
				}
			}
		}
		targetDocID, ok := keyIdx.resolve(t.Object)
		if !ok || targetDocID == docID {
			continue // 未知/歧义宾语、自环：跳过（留实体轨兜底）
		}
		closed := t.Confidence < relationMinConfidence
		links = append(links, bizknowledge.SemanticLink{
			TargetDocID: targetDocID,
			Relation:    relation,
			Confidence:  t.Confidence,
			Context:     relationEdgeContext(t.Subject, t.Evidence),
			Closed:      closed,
		})
		if !closed {
			stats.OpenLinks++
		}
	}
	stats.Links = len(links)

	if err := e.links.ReplaceSemanticLinks(ctx, doc.CollectionID, docID, links); err != nil {
		return stats, fmt.Errorf("replace semantic links %s: %w", docID, err)
	}
	if err := e.state.UpsertRelationState(ctx, bizknowledge.RelationState{
		DocID:                docID,
		CollectionID:         doc.CollectionID,
		ContentHash:          contentHash,
		RelationsExtractedAt: time.Now(),
	}); err != nil {
		return stats, fmt.Errorf("upsert relation state %s: %w", docID, err)
	}
	return stats, nil
}

// ── Step1 / Step2 LLM 调用 ────────────────────────────────────────────────

type extractEntity struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type extractTriple struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// llmExtractEntities Step1 实体抽取（M2.1 entity 轨与 M2.2 关系抽取共用）。
func llmExtractEntities(ctx context.Context, llm biz.LLMCaller, provider, model, title, body string) ([]extractEntity, error) {
	callCtx, cancel := context.WithTimeout(ctx, relationExtractTimeout)
	defer cancel()
	resp, _, err := llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是知识图谱实体抽取助手。从给定文档正文中抽取关键实体（名词短语：概念、技术、产品、组件、人物、组织、协议等）。
要求：
1. 以 JSON 数组输出，每个元素 {"name": "实体名", "type": "concept/tech/product/component/person/org/protocol/other"}；
2. 实体名保留正文中的原始写法，不要翻译或改写；
3. 最多 20 个，按对文档主题的重要性排序；
4. 只输出 JSON 数组，不要任何解释或代码块包裹。`,
		User: "文档标题：" + title + "\n\n文档正文：\n" + body,
	})
	if err != nil {
		return nil, err
	}
	var entities []extractEntity
	if err := unmarshalLLMJSONArray(resp, &entities); err != nil {
		// 容错：LLM 偶发输出纯字符串数组。
		var names []string
		if serr := unmarshalLLMJSONArray(resp, &names); serr != nil {
			return nil, fmt.Errorf("parse entities: %w", err)
		}
		for _, n := range names {
			entities = append(entities, extractEntity{Name: n})
		}
	}
	out := entities[:0]
	for _, en := range entities {
		en.Name = strings.TrimSpace(en.Name)
		if en.Name == "" {
			continue
		}
		if bizknowledge.IsReservedEntryKey(en.Name) || bizknowledge.IsNoiseEntryKey(en.Name) {
			continue // 保留键/噪声键不成实体（entry_key_guard：堵伪实体边源头）
		}
		out = append(out, en)
		if len(out) >= relationMaxEntities {
			break
		}
	}
	return out, nil
}

func (e *RelationExtractor) extractTriples(ctx context.Context, provider, model, title, body string, entities []extractEntity) ([]extractTriple, error) {
	names := make([]string, 0, len(entities))
	for _, en := range entities {
		names = append(names, en.Name)
	}
	entityList, _ := json.Marshal(names)

	callCtx, cancel := context.WithTimeout(ctx, relationExtractTimeout)
	defer cancel()
	resp, _, err := e.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是知识图谱关系抽取助手。给定文档正文与实体清单，抽取实体间的关系三元组。
谓词优先使用核心闭集：` + strings.Join(bizknowledge.CoreRelations, ", ") + `。
核心闭集确实无法表达的关系，允许使用简洁的新谓词（英文 kebab-case）。
以 JSON 数组输出，每个元素：
{"subject": "主语实体", "predicate": "谓词", "object": "宾语实体", "confidence": 0.0-1.0, "evidence": "来源句片段"}
要求：
1. subject/object 必须原样取自实体清单；
2. confidence 反映正文断言的明确程度（明示 0.8-1.0，推断 0.5-0.8，猜测 <0.5）；
3. 最多 16 条，按置信度降序；
4. 只输出 JSON 数组，不要任何解释或代码块包裹。`,
		User: "文档标题：" + title + "\n\n实体清单：\n" + string(entityList) + "\n\n文档正文：\n" + body,
	})
	if err != nil {
		return nil, err
	}
	var triples []extractTriple
	if err := unmarshalLLMJSONArray(resp, &triples); err != nil {
		return nil, fmt.Errorf("parse triples: %w", err)
	}
	out := triples[:0]
	for _, t := range triples {
		t.Subject = strings.TrimSpace(t.Subject)
		t.Object = strings.TrimSpace(t.Object)
		if t.Subject != "" && t.Object != "" && strings.TrimSpace(t.Predicate) != "" {
			out = append(out, t)
		}
		if len(out) >= relationMaxTriples {
			break
		}
	}
	return out, nil
}

// unmarshalLLMJSONArray 解析 LLM 返回的 JSON 数组：去代码围栏；
// 直接解析失败时退化为首 [ 至末 ] 子串抢救（LLM 偶发前后缀噪声）。
func unmarshalLLMJSONArray(raw string, v any) error {
	s := stripCodeFence(strings.TrimSpace(raw))
	if err := json.Unmarshal([]byte(s), v); err == nil {
		return nil
	}
	lo := strings.Index(s, "[")
	hi := strings.LastIndex(s, "]")
	if lo >= 0 && hi > lo {
		return json.Unmarshal([]byte(s[lo:hi+1]), v)
	}
	return fmt.Errorf("no JSON array in response")
}

// ── 谓词归一化 ────────────────────────────────────────────────────────────

// normalizePredicate 谓词归一化：小写、去空白、空格/下划线归一为连字符。
func normalizePredicate(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	p = strings.ReplaceAll(p, "_", "-")
	p = strings.Join(strings.Fields(p), "-")
	return p
}

func isCoreRelation(relation string) bool {
	for _, c := range bizknowledge.CoreRelations {
		if relation == c {
			return true
		}
	}
	return false
}

// relationEdgeContext 边证据：主语实体 + 来源句片段（截断，入 links.context）。
func relationEdgeContext(subject, evidence string) string {
	s := strings.TrimSpace(subject)
	if ev := strings.TrimSpace(evidence); ev != "" {
		if s != "" {
			s += " — "
		}
		s += ev
	}
	return truncateRunes(s, relationContextMaxRunes)
}

func relationEvidenceSupported(body, evidence string) bool {
	evidence = strings.TrimSpace(evidence)
	if utf8.RuneCountInString(evidence) < 2 {
		return false
	}
	normalize := func(s string) string {
		return strings.Join(strings.Fields(strings.ToLower(s)), " ")
	}
	return strings.Contains(normalize(body), normalize(evidence))
}

// ── 宾语实体 → 文档解析 ───────────────────────────────────────────────────

// docKeyIndex 同库文档的多键解析索引（basename/title/aliases → docID）。
// 与 autolink/mention 同语义：多键命中、歧义键跳过（不产边）。
type docKeyIndex struct {
	keys      map[string]string // normKey → docID
	ambiguous map[string]bool   // 多文档同键
}

// buildDocKeyIndex 拉取同库候选文档建键索引（每文档抽取一次，N(候选) 级别可接受）。
func (e *RelationExtractor) buildDocKeyIndex(ctx context.Context, collectionID string) (*docKeyIndex, error) {
	cands, err := e.resolver.ListResolveCandidates(ctx, []string{collectionID})
	if err != nil {
		return nil, err
	}
	idx := &docKeyIndex{keys: make(map[string]string, len(cands)*2), ambiguous: make(map[string]bool)}
	for _, c := range cands {
		idx.add(docDisplayName(c.RelPath, ""), c.DocID)
		idx.add(c.Title, c.DocID)
		for _, a := range c.Aliases {
			idx.add(a, c.DocID)
		}
	}
	return idx, nil
}

func (idx *docKeyIndex) add(name, docID string) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 2 || docID == "" {
		return // 单字符键匹配噪声过大（与 mentionNeedles 同纪律）
	}
	k := strings.ToLower(name)
	if prev, ok := idx.keys[k]; ok && prev != docID {
		idx.ambiguous[k] = true
		return
	}
	idx.keys[k] = docID
}

// resolve 宾语实体名 → 目标文档 ID；未命中/歧义返回 ok=false。
func (idx *docKeyIndex) resolve(name string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(name))
	if k == "" || idx.ambiguous[k] {
		return "", false
	}
	id, ok := idx.keys[k]
	return id, ok
}

// docDisplayName 文档显示名：rel_path/source 取 basename 去扩展名。
func docDisplayName(relPath, source string) string {
	name := relPath
	if name == "" {
		name = source
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

// truncateRunes 按 rune 截断（追加省略号标记截断点）。
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}
