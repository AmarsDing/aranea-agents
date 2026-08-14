// Package knowledge — Vault 摘要卡生成（P2-2）：LLM 预生成摘要写入 frontmatter。
// 降级原则：LLM 不可用/超时/输出不可解析 → 返回 nil error 不阻塞索引（NFR-11）。
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
	"aranea-agents/pkg/strutil"
)

const (
	// vaultSummaryTimeout 单次摘要 LLM 调用超时。
	vaultSummaryTimeout = 30 * time.Second
	// vaultSummaryRetryAfter 同内容失败后的最小重试间隔（防无 LLM 时每轮 sync 高频重试）。
	vaultSummaryRetryAfter = 5 * time.Minute
	// vaultSummaryMaxRunes 摘要字符上限（设计 V2：摘要卡 ≤200 token，字符截断近似控制）。
	vaultSummaryMaxRunes = 300
	// vaultSummaryInputRunes 送入 LLM 的正文窗口上限。
	vaultSummaryInputRunes = 8000
)

const vaultSummarySystemPrompt = `你是一名知识库文档摘要助手。用户会给你一篇 Markdown 文档正文，请输出 JSON：
{"summary": "150 字以内的内容摘要", "tags": ["3~6 个主题标签"], "type": "文档类型单词（如 report/note/guide/reference/log/other）"}
要求：
1. summary 客观概括文档主题与要点，不夸大、不遗漏关键事实；
2. tags 取文档核心主题词；
3. 只输出 JSON 对象，不要解释、不要代码块包裹。`

// VaultSummaryGenerator 为 vault 文档生成摘要卡（summary/tags/type）并写回 frontmatter。
//
// 收敛语义：
//   - stale 判定仅基于 Body（SummaryStale），KB 写回 frontmatter 自身不触发重生成；
//   - 写回采用「重读最新 + 受管字段合并 + CAS」：并发外部编辑的正文不被回滚；
//   - 若摘要基于旧正文（并发编辑），SummaryHash 与当前正文不匹配 → 下轮 sync 自动重生成（自愈）。
type VaultSummaryGenerator struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	filer   *bizknowledge.VaultFiler
	lg      loggateway.Logger

	mu          sync.Mutex
	lastAttempt map[string]time.Time // relPath+bodyHash → 上次尝试时间（节流）
	retryAfter  time.Duration
}

// NewVaultSummaryGenerator 构造。lg/filer 为 nil 时使用默认值。
func NewVaultSummaryGenerator(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, filer *bizknowledge.VaultFiler, lg loggateway.Logger) *VaultSummaryGenerator {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if filer == nil {
		filer = bizknowledge.NewVaultFiler(nil)
	}
	return &VaultSummaryGenerator{
		llm:         llm,
		sys:         sys,
		catalog:     catalog,
		filer:       filer,
		lg:          lg.With(loggateway.Domain("knowledge")),
		lastAttempt: make(map[string]time.Time),
		retryAfter:  vaultSummaryRetryAfter,
	}
}

// GenerateAndApply 为单个文档生成摘要卡并写回 frontmatter；返回 applied=true 表示已更新。
// 任何 LLM 路径失败均降级为 (false, nil)；仅文件读写错误返回 error。
func (g *VaultSummaryGenerator) GenerateAndApply(ctx context.Context, root, relPath string) (bool, error) {
	if g == nil || g.llm == nil {
		return false, nil
	}
	doc, _, err := g.filer.ReadDocWithHash(root, relPath)
	if err != nil {
		return false, err
	}
	if !bizknowledge.SummaryStale(doc.Body, doc.Frontmatter.SummaryHash) {
		return false, nil
	}
	body := strings.TrimSpace(doc.Body)
	if body == "" {
		return false, nil
	}
	if !g.markAttempt(relPath, body) {
		return false, nil
	}
	provider, model, err := ResolveLLM(ctx, g.sys, g.catalog, "vault summary", g.lg)
	if err != nil {
		return false, nil // 降级：无可用 LLM 不阻塞
	}
	card := g.callLLM(ctx, provider, model, body)
	if card == nil {
		return false, nil // 失败已降级（日志已记）
	}
	card.baseHash = bizknowledge.HashContent(doc.Body)
	return g.apply(root, relPath, card)
}

// markAttempt 节流：同 relPath+bodyHash 在 retryAfter 窗口内只允许一次尝试。
// body 变更即新 key（内容变化后立即重试）。
func (g *VaultSummaryGenerator) markAttempt(relPath, body string) bool {
	key := relPath + "\x00" + bizknowledge.HashContent(body)
	g.mu.Lock()
	defer g.mu.Unlock()
	if t, ok := g.lastAttempt[key]; ok && time.Since(t) < g.retryAfter {
		return false
	}
	g.lastAttempt[key] = time.Now()
	return true
}

// summaryCard LLM 产出（baseHash 由生成方补记，非 LLM 输出）。
type summaryCard struct {
	summary  string
	tags     []string
	docType  string
	baseHash string
}

func (g *VaultSummaryGenerator) callLLM(ctx context.Context, provider, model, body string) *summaryCard {
	callCtx, cancel := context.WithTimeout(ctx, vaultSummaryTimeout)
	defer cancel()
	resp, _, err := g.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   vaultSummarySystemPrompt,
		User:     truncateRunes(body, vaultSummaryInputRunes),
	})
	if err != nil {
		g.lg.Warn("vault summary LLM failed",
			loggateway.StepID("knowledge.vault_summary.fail"),
			loggateway.Err(err))
		return nil
	}
	card, err := parseSummaryCard(resp)
	if err != nil {
		g.lg.Warn("vault summary parse failed",
			loggateway.StepID("knowledge.vault_summary.parse"),
			loggateway.Err(err))
		return nil
	}
	return card
}

// apply 合并写回：重读磁盘最新版本，仅覆盖受管摘要字段（并发外部编辑的正文不回滚）。
func (g *VaultSummaryGenerator) apply(root, relPath string, card *summaryCard) (bool, error) {
	cur, curHash, err := g.filer.ReadDocWithHash(root, relPath)
	if err != nil {
		return false, err
	}
	cur.Frontmatter.Summary = card.summary
	cur.Frontmatter.Tags = card.tags
	cur.Frontmatter.Type = card.docType
	cur.Frontmatter.SummaryHash = card.baseHash
	conflict, err := g.filer.WriteDocCAS(root, relPath, cur, curHash)
	if err != nil {
		return false, err
	}
	if conflict {
		g.lg.Warn("vault summary applied with conflict, both copies kept",
			loggateway.StepID("knowledge.vault_summary.conflict"),
			loggateway.Str("rel_path", relPath))
	}
	g.lg.Debug("vault summary applied",
		loggateway.Str("rel_path", relPath),
		loggateway.Int("tags", len(card.tags)))
	return true, nil
}

// parseSummaryCard 解析 LLM 输出（容错截取首个 JSON 对象），并做长度/数量校验。
func parseSummaryCard(resp string) (*summaryCard, error) {
	raw := strings.TrimSpace(stripCodeFence(resp))
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, apierror.Internal(apierror.DomainKnowledge, "vault summary: no JSON object in LLM output")
	}
	var parsed struct {
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
		Type    string   `json:"type"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil, apierror.Internal(apierror.DomainKnowledge, "vault summary: invalid JSON").WithCause(err)
	}
	summary := truncateRunes(strings.TrimSpace(parsed.Summary), vaultSummaryMaxRunes)
	if summary == "" {
		return nil, apierror.Internal(apierror.DomainKnowledge, "vault summary: empty summary")
	}
	return &summaryCard{
		summary: summary,
		tags:    sanitizeSummaryTags(parsed.Tags),
		docType: truncateRunes(strings.TrimSpace(parsed.Type), 30),
	}, nil
}

// sanitizeSummaryTags 清洗标签：去空白、单标签 ≤30 rune、总数 ≤8。
func sanitizeSummaryTags(tags []string) []string {
	out := make([]string, 0, 8)
	for _, t := range tags {
		t = truncateRunes(strings.TrimSpace(t), 30)
		if t == "" {
			continue
		}
		out = append(out, t)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// truncateRunes 按 rune 截断（避免截断多字节字符）。P1-3：委托 strutil。
func truncateRunes(s string, max int) string {
	return strutil.TruncateRunes(s, max)
}
