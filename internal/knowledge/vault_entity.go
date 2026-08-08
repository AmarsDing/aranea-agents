// Package knowledge — Vault 实体共现关联（P2-4 entity 轨）：LLM 抽取文档实体，
// 基于实体共享建 entity 轨关联。降级原则同摘要卡：LLM 不可用/超时/输出不可解析
// → 返回 (false, nil) 不阻塞索引（NFR-11）。
package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	// vaultEntityTimeout 单次实体抽取 LLM 调用超时。
	vaultEntityTimeout = 30 * time.Second
	// vaultEntityRetryAfter 同内容失败后的最小重试间隔（防无 LLM 时每轮 sync 高频重试）。
	vaultEntityRetryAfter = 5 * time.Minute
	// vaultEntityInputRunes 送入 LLM 的正文窗口上限。
	vaultEntityInputRunes = 8000
	// vaultEntityMaxEntities 单文档保留实体数上限。
	vaultEntityMaxEntities = 20
	// vaultEntityMaxNameRunes 实体名长度上限。
	vaultEntityMaxNameRunes = 40
	// vaultEntityMaxTypeRunes 实体类型长度上限。
	vaultEntityMaxTypeRunes = 30
	// vaultEntityDefaultMaxDocFreq R-3 频次过滤默认上限：出现在更多文档中的实体
	// 无区分度，视为噪声不建链。
	vaultEntityDefaultMaxDocFreq = 50
)

const vaultEntitySystemPrompt = `你是一名知识库实体抽取助手。用户会给你一篇 Markdown 文档正文，请抽取关键实体并输出 JSON：
{"entities":[{"name":"实体名","type":"类型（person/org/concept/tech/topic/indicator/other）","mentions":出现次数}]}
要求：
1. 只抽取有区分度的实体：专有名词、概念、技术、指标、主题词；
2. 不抽取「文档」「正文」「笔记」等泛化词；
3. 实体保留原文形态，总数不超过 20 个；
4. 只输出 JSON 对象，不要解释、不要代码块包裹。`

// entityStopwords 内置停用实体（R-3）：泛化词无关联价值；ASCII 匹配大小写不敏感。
var entityStopwords = map[string]bool{
	"文档": true, "文件": true, "正文": true, "内容": true, "笔记": true,
	"章节": true, "段落": true, "图片": true, "表格": true, "示例": true,
	"the": true, "this": true, "that": true, "document": true,
	"file": true, "note": true, "content": true, "section": true,
}

// VaultEntityExtractor 抽取文档实体并基于共现建 entity 轨关联（P2-4）。
//
// 幂等：按 docID+contentHash 记录已抽取状态，同内容不重复调 LLM；
// 失败节流：同内容失败后在 retryAfter 窗口内不重试；
// 顺序契约：实体先落库再查共现（频次统计含当前文档，达上限即噪声 R-3）。
type VaultEntityExtractor struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	uc      *bizknowledge.Usecase
	lg      loggateway.Logger

	mu          sync.Mutex
	extracted   map[string]string    // docID → 已抽取 contentHash（幂等短路）
	lastAttempt map[string]time.Time // docID+\x00+contentHash → 上次尝试（失败节流）
	retryAfter  time.Duration
	maxDocFreq  int // R-3：实体文档频次上限（含当前文档），超出视为噪声不建链
}

// NewVaultEntityExtractor 构造。lg 为 nil 时使用 Noop。
func NewVaultEntityExtractor(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, uc *bizknowledge.Usecase, lg loggateway.Logger) *VaultEntityExtractor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VaultEntityExtractor{
		llm:         llm,
		sys:         sys,
		catalog:     catalog,
		uc:          uc,
		lg:          lg.With(loggateway.Domain("knowledge")),
		extracted:   make(map[string]string),
		lastAttempt: make(map[string]time.Time),
		retryAfter:  vaultEntityRetryAfter,
		maxDocFreq:  vaultEntityDefaultMaxDocFreq,
	}
}

// ExtractAndLink 抽取实体 → 落库 → 共现建链。applied=true 表示完成一次有效抽取
// （实体被全部过滤为空也是有效结果）。LLM 路径任何失败均降级 (false, nil)；
// 仅 repo 读写错误返回 error（供调用方记录，下轮 hash 未变前由幂等短路防重刷）。
func (e *VaultEntityExtractor) ExtractAndLink(ctx context.Context, collectionID, docID string) (bool, error) {
	if e == nil || e.llm == nil || e.uc == nil {
		return false, nil
	}
	doc, err := e.uc.GetDocument(ctx, docID)
	if err != nil {
		return false, err
	}
	content := strings.TrimSpace(doc.ContentText)
	if content == "" {
		return false, nil
	}
	if e.alreadyExtracted(docID, doc.ContentHash) {
		return false, nil
	}
	if !e.markAttempt(docID, doc.ContentHash) {
		return false, nil
	}
	provider, model, err := ResolveLLM(ctx, e.sys, e.catalog, "vault entity", e.lg)
	if err != nil {
		return false, nil // 降级：无可用 LLM 不阻塞
	}
	entities := e.callLLM(ctx, provider, model, content)
	if entities == nil {
		return false, nil // 失败已降级（日志已记）
	}
	entityIDs, err := e.uc.ReplaceDocEntities(ctx, collectionID, docID, entities)
	if err != nil {
		return false, err
	}
	links, err := e.buildLinks(ctx, collectionID, docID, entityIDs)
	if err != nil {
		return false, err
	}
	if err := e.uc.ReplaceEntityLinks(ctx, collectionID, docID, links); err != nil {
		return false, err
	}
	e.markExtracted(docID, doc.ContentHash)
	e.lg.Debug("vault entities extracted",
		loggateway.Str("collection_id", collectionID),
		loggateway.Str("doc_id", docID),
		loggateway.Int("entities", len(entities)),
		loggateway.Int("links", len(links)))
	return true, nil
}

// buildLinks 基于实体共现建链（Context 为共享实体展示名逗号分隔，供 UI 标注来源）。
// G5-F：共现查询按 ReplaceDocEntities 解析返回的 entity_id 关联（归一化/别名已生效）。
func (e *VaultEntityExtractor) buildLinks(ctx context.Context, collectionID, docID string, entityIDs []int64) ([]bizknowledge.Link, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	coocs, err := e.uc.FindEntityCooccurrences(ctx, collectionID, entityIDs, docID, e.maxDocFreq)
	if err != nil {
		return nil, err
	}
	links := make([]bizknowledge.Link, 0, len(coocs))
	for _, c := range coocs {
		if c.DocID == docID || len(c.SharedEntities) == 0 {
			continue // 防御自链/空共现
		}
		links = append(links, bizknowledge.Link{
			CollectionID: collectionID,
			DocID:        docID,
			TargetDocID:  c.DocID,
			LinkType:     bizknowledge.LinkTypeEntity,
			Context:      strings.Join(c.SharedEntities, ","),
		})
	}
	return links, nil
}

// alreadyExtracted 幂等短路：同 docID+contentHash 已成功抽取过。
func (e *VaultEntityExtractor) alreadyExtracted(docID, contentHash string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	h, ok := e.extracted[docID]
	return ok && h == contentHash
}

func (e *VaultEntityExtractor) markExtracted(docID, contentHash string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.extracted[docID] = contentHash
}

// markAttempt 失败节流：同 docID+contentHash 在 retryAfter 窗口内只允许一次尝试。
func (e *VaultEntityExtractor) markAttempt(docID, contentHash string) bool {
	key := docID + "\x00" + contentHash
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.lastAttempt[key]; ok && time.Since(t) < e.retryAfter {
		return false
	}
	e.lastAttempt[key] = time.Now()
	return true
}

// callLLM 调用 LLM 并解析实体；任何失败返回 nil（日志已记，调用方降级）。
func (e *VaultEntityExtractor) callLLM(ctx context.Context, provider, model, content string) []bizknowledge.DocEntity {
	callCtx, cancel := context.WithTimeout(ctx, vaultEntityTimeout)
	defer cancel()
	resp, _, err := e.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   vaultEntitySystemPrompt,
		User:     truncateRunes(content, vaultEntityInputRunes),
	})
	if err != nil {
		e.lg.Warn("vault entity LLM failed",
			loggateway.StepID("knowledge.vault_entity.fail"),
			loggateway.Err(err))
		return nil
	}
	entities, err := parseDocEntities(resp)
	if err != nil {
		e.lg.Warn("vault entity parse failed",
			loggateway.StepID("knowledge.vault_entity.parse"),
			loggateway.Err(err))
		return nil
	}
	return entities
}

// parseDocEntities 解析 LLM 输出（容错截取首个 JSON 对象），并做停用词/长度/数量清洗。
// 返回空切片（非 nil）表示「抽取成功但无有效实体」——调用方据此清空旧实体。
func parseDocEntities(resp string) ([]bizknowledge.DocEntity, error) {
	raw := strings.TrimSpace(stripCodeFence(resp))
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, apierror.Internal(apierror.DomainKnowledge, "vault entity: no JSON object in LLM output")
	}
	var parsed struct {
		Entities []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Mentions int    `json:"mentions"`
		} `json:"entities"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil, apierror.Internal(apierror.DomainKnowledge, "vault entity: invalid JSON").WithCause(err)
	}
	out := make([]bizknowledge.DocEntity, 0, vaultEntityMaxEntities)
	seen := make(map[string]bool, len(parsed.Entities))
	for _, en := range parsed.Entities {
		name := truncateRunes(strings.TrimSpace(en.Name), vaultEntityMaxNameRunes)
		// R-3 噪声过滤：过短（单字无区分度）/ 停用词 / 重复名。
		if len([]rune(name)) < 2 || entityStopwords[strings.ToLower(name)] || seen[name] {
			continue
		}
		seen[name] = true
		mentions := en.Mentions
		if mentions < 1 {
			mentions = 1
		}
		out = append(out, bizknowledge.DocEntity{
			Name:       name,
			EntityType: truncateRunes(strings.TrimSpace(en.Type), vaultEntityMaxTypeRunes),
			Mentions:   mentions,
		})
		if len(out) >= vaultEntityMaxEntities {
			break
		}
	}
	return out, nil
}
