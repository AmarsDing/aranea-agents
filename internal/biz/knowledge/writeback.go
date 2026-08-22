package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

var _ SessionWriteBack = (*Usecase)(nil)
var _ SessionWriteBackReview = (*Usecase)(nil)

// SP7 G2 写回飞轮：自动记忆过验证门的事实追加到团队库日记，带不可变 provenance。
// 不抽三元组、不改记忆内核；失败降级不阻断 AutoMemory 主流程。

const (
	// WriteBackCollectionName 专用团队收件箱；若工作区已有其它 team 库则复用先找到的。
	WriteBackCollectionName = "团队知识收件箱"
	// WriteBackInboxPrefix 写回日记流水的 rel_path 字面前缀（inbox/writeback-*.md）。
	// 日记只做 provenance：Agent 默认检索路径以此前缀排除流水，词条内容不受影响。
	WriteBackInboxPrefix   = "inbox/writeback-"
	writeBackMinConfidence = 0.85
	writeBackMinRunes      = 8
	writeBackMaxFacts      = 8
)

// WriteBackFact 一条待归档事实（由 L3 写入结果投影，不含推测）。
type WriteBackFact struct {
	FactID     string
	Statement  string
	FactKind   string
	Confidence float64
	SourceKind string
	// Tags 是记忆侧 LLM 已抽取的主题标签（memory_facts.tags_json 投影）：
	// 词条页定位主键——首选 tag 命中已有词条（basename/title/aliases）则 upsert，
	// 未命中新建 entries/<slug>.md；无 tags 回退当日日记。
	Tags []string
	// EntryTitle 写回执行期回填：事实落进的词条页显示名（日记 provenance 指针）；
	// 调用方传入时忽略，统一由写回路径赋值。
	EntryTitle string
}

// WriteBackInput 一次会话写回批次。
type WriteBackInput struct {
	Workspace string
	SessionID string
	AgentID   string
	UserID    string
	Facts     []WriteBackFact
}

// WriteBackResult 写回结果（Appended=0 表示门全挡或无团队库可写）。
type WriteBackResult struct {
	CollectionID string
	DocID        string
	Appended     int
	Created      bool
	// Landed fact_id → 落点词条页显示名；未落词条（无 tags 回退日记）的事实不在其中。
	Landed map[string]string
	// EntryDocs 本次实际改动的词条页文档（service 层据此重放 chunk/FTS）。
	// 2026-08-15 修复：词条页此前不在结果里，service 只重放日记 DocID，
	// 词条 chunks 永不重建——写入成功但检索不可见（entries/* 全部卡 pending）。
	EntryDocs []PromoteTouchedDoc
}

// EntryOf 返回指定 fact_id 落点的词条页显示名（knowledge_write 工具回执用）；
// 未落词条返回空。
func (r WriteBackResult) EntryOf(factID string) string {
	return r.Landed[factID]
}

// WriteBackReplayFunc 写回 chunk 重放钩子：touched 文档（日记 + 词条页）重建
// chunks/FTS（词法库 embedder=nil 纯分块）。失败必须只返回 error 由调用方
// Warn——写回本身已成功，重放属 best-effort 派生索引。
type WriteBackReplayFunc func(ctx context.Context, col Collection, touched []PromoteTouchedDoc) error

// SetWriteBackReplay 接线写回 chunk 重放钩子（生产 KnowledgeService 注入；
// 装配同 SetBlockIndexRepos 模式）。
func (u *Usecase) SetWriteBackReplay(fn WriteBackReplayFunc) {
	u.writeBackReplay = fn
}

// HasWriteBackReplay reports whether chunk replay is bound (Wave 2 bootstrap check).
func (u *Usecase) HasWriteBackReplay() bool {
	return u != nil && u.writeBackReplay != nil
}

// WriteBackGraphFunc 写回图谱钩子：touched 词条页触发 M2 实体共现 + typed 关系
// 抽取（均为 content_hash 幂等、best-effort）。团队库无 vault 同步循环，实体钩子
// 唯一载体（vault applier）永不触发——写回路径必须自带图谱收口，否则 entries/*
// 在图谱中恒为孤立节点。只传词条页：日记是 provenance（检索排除），抽边纯浪费。
type WriteBackGraphFunc func(ctx context.Context, col Collection, entryDocs []PromoteTouchedDoc) error

// SetWriteBackGraph 接线写回图谱钩子（生产 KnowledgeService 注入；同
// SetWriteBackReplay 模式——knowledge_write 工具直调 Usecase，挂 biz 层收口）。
func (u *Usecase) SetWriteBackGraph(fn WriteBackGraphFunc) {
	u.writeBackGraph = fn
}

// SessionWriteBack 会话事实写回团队知识库（SP7 G2）。
// Stability: evolving
type SessionWriteBack interface {
	WriteBackSessionFacts(ctx context.Context, in WriteBackInput) (WriteBackResult, error)
}

// SessionWriteBackReview 低置信事实待确认队列（US-44）。
// Stability: evolving
type SessionWriteBackReview interface {
	EnqueueWriteBackReview(ctx context.Context, in WriteBackInput) (WriteBackResult, error)
}

var writeBackKinds = map[string]struct{}{
	"preference":   {},
	"profile":      {},
	"goal":         {},
	"constraint":   {},
	"decision":     {},
	"relationship": {},
}

// FilterWriteBackFacts 验证门：白名单 kind + 高置信 + 最短陈述，截断 writeBackMaxFacts。
func FilterWriteBackFacts(facts []WriteBackFact) []WriteBackFact {
	out := make([]WriteBackFact, 0, len(facts))
	for _, f := range facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" || utf8.RuneCountInString(stmt) < writeBackMinRunes {
			continue
		}
		if f.Confidence < writeBackMinConfidence {
			continue
		}
		kind := strings.TrimSpace(f.FactKind)
		if _, ok := writeBackKinds[kind]; !ok {
			continue
		}
		f.Statement = stmt
		f.FactKind = kind
		out = append(out, f)
		if len(out) >= writeBackMaxFacts {
			break
		}
	}
	return out
}

// WriteBackRelPath 按 UTC 日切分的收件箱日记路径。
func WriteBackRelPath(now time.Time) string {
	return "inbox/writeback-" + now.UTC().Format("2006-01-02") + ".md"
}

// FormatWriteBackAppendix 渲染带 provenance 的 Markdown 追加块（纯函数）。
func FormatWriteBackAppendix(in WriteBackInput, facts []WriteBackFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range facts {
		kind := f.FactKind
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", kind, f.Statement)
		if f.FactID != "" {
			fmt.Fprintf(&b, "- fact_id: `%s`\n", f.FactID)
		}
		if in.SessionID != "" {
			fmt.Fprintf(&b, "- session_id: `%s`\n", in.SessionID)
		}
		if in.AgentID != "" {
			fmt.Fprintf(&b, "- agent_id: `%s`\n", in.AgentID)
		}
		if in.UserID != "" {
			fmt.Fprintf(&b, "- user_id: `%s`\n", in.UserID)
		}
		fmt.Fprintf(&b, "- confidence: %.2f\n", f.Confidence)
		fmt.Fprintf(&b, "- kind: %s\n", kind)
		if entry := strings.TrimSpace(f.EntryTitle); entry != "" {
			fmt.Fprintf(&b, "- entry: [[%s]]\n", entry)
		}
		src := strings.TrimSpace(f.SourceKind)
		if src == "" {
			src = "auto_memory"
		}
		fmt.Fprintf(&b, "- source: %s\n\n", src)
	}
	return b.String()
}

func writeBackAlreadyPresent(body, factID, statement string) bool {
	if factID != "" && strings.Contains(body, "fact_id: `"+factID+"`") {
		return true
	}
	if factID == "" && statement != "" && strings.Contains(body, statement) {
		return true
	}
	return false
}

func writeBackDocHeader(day time.Time) string {
	d := day.UTC().Format("2006-01-02")
	return "# 会话沉淀 " + d + "\n\n" +
		"由自动记忆验证门写入团队库。每条含不可变 provenance；未过门的推测不会出现在此。\n\n"
}

// WriteBackSessionFacts 词条优先写回：过门事实按 tags 匹配/新建词条页 upsert
// （同一 fact_id 再写入时更新词条旧段），当日日记保留为 provenance 流水。
// 无过门事实或无法解析团队库时返回零值结果（不报错）。
func (u *Usecase) WriteBackSessionFacts(ctx context.Context, in WriteBackInput) (WriteBackResult, error) {
	if err := u.requireRepo(); err != nil {
		return WriteBackResult{}, err
	}
	facts := FilterWriteBackFacts(in.Facts)
	if len(facts) == 0 {
		return WriteBackResult{}, nil
	}
	col, err := u.resolveWriteBackCollection(ctx, in.Workspace)
	if err != nil {
		return WriteBackResult{}, err
	}
	return u.writeBackFacts(ctx, col, in, facts)
}

// writeBackFacts 跳过验证门，词条 upsert + 日记 provenance 双写（确认过门走此路径）。
func (u *Usecase) writeBackFacts(ctx context.Context, col Collection, in WriteBackInput, facts []WriteBackFact) (WriteBackResult, error) {
	if len(facts) == 0 {
		return WriteBackResult{CollectionID: col.ID}, nil
	}
	landed, entryDocs := u.upsertFactsToEntries(ctx, col, in, facts)
	byFactID := make(map[string]string, len(landed))
	for i, title := range landed {
		facts[i].EntryTitle = title
		if facts[i].FactID != "" {
			byFactID[facts[i].FactID] = title
		}
	}
	now := time.Now()
	rel := WriteBackRelPath(now)
	res, err := u.upsertMarkdownDoc(ctx, col, rel, writeBackDocHeader(now), FormatWriteBackAppendix(in, facts))
	res.Landed = byFactID
	res.EntryDocs = entryDocs
	if err != nil {
		return res, err
	}
	// chunk 重放（2026-08-15 修复）：日记 + 全部词条页。钩子未接线时跳过
	// （ReembedDocuments 可手动自愈）；失败仅 Warn——写回已成功，重放可重试。
	if u.writeBackReplay != nil && (res.Appended > 0 || len(res.EntryDocs) > 0) {
		touched := make([]PromoteTouchedDoc, 0, len(res.EntryDocs)+1)
		if res.DocID != "" {
			touched = append(touched, PromoteTouchedDoc{DocID: res.DocID, Created: res.Created})
		}
		touched = append(touched, res.EntryDocs...)
		if len(touched) > 0 {
			if rerr := u.writeBackReplay(ctx, col, touched); rerr != nil {
				u.lg.Warn("写回飞轮 chunk 重放失败（写回已落库，ReembedDocuments 可自愈）",
					loggateway.StepID("knowledge.writeback"),
					loggateway.Str("collection_id", col.ID),
					loggateway.Err(rerr),
				)
			}
		}
	}
	// 图谱收口（2026-08-16）：词条页实体共现 + typed 关系抽取。必须在 chunk
	// 重放之后（抽取器读正文/状态依赖重放落库）；失败仅 Warn——幂等闸保证
	// 下次写回/热文档扫描自然重试。
	if u.writeBackGraph != nil && len(res.EntryDocs) > 0 {
		if gerr := u.writeBackGraph(ctx, col, res.EntryDocs); gerr != nil {
			u.lg.Warn("写回飞轮图谱抽取失败（写回已落库，幂等闸保证后续重试）",
				loggateway.StepID("knowledge.writeback"),
				loggateway.Str("collection_id", col.ID),
				loggateway.Err(gerr),
			)
		}
	}
	return res, nil
}

// upsertMarkdownDoc 创建或追加 Markdown 文档；按 fact_id/陈述去重。
func (u *Usecase) upsertMarkdownDoc(ctx context.Context, col Collection, rel, header, appendix string) (WriteBackResult, error) {
	if strings.TrimSpace(appendix) == "" {
		return WriteBackResult{CollectionID: col.ID}, nil
	}
	doc, err := u.documents.GetDocumentByRelPath(ctx, col.ID, rel)
	created := false
	if err != nil {
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return WriteBackResult{}, err
		}
		created = true
	}
	body := header
	if !created {
		body = doc.ContentText
	}
	toAppend := filterAppendixNotPresent(body, appendix)
	if toAppend == "" {
		if created {
			return WriteBackResult{CollectionID: col.ID}, nil
		}
		return WriteBackResult{CollectionID: col.ID, DocID: doc.ID}, nil
	}
	newContent := strings.TrimRight(body, "\n") + "\n\n" + strings.TrimSpace(toAppend) + "\n"
	newContent = u.MaybeAutolinkOutgoing(ctx, col.ID, doc.ID, rel, newContent)

	if created {
		doc, err = u.CreateDocument(ctx, Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			ContentText:  newContent,
			Organized:    true,
			Status:       "pending",
		})
		if err != nil {
			return WriteBackResult{}, err
		}
	} else {
		if err := u.documents.UpdateDocumentContent(ctx, doc.ID, newContent, true); err != nil {
			return WriteBackResult{}, err
		}
	}
	if err := u.RebuildBlockIndex(ctx, col.ID, doc.ID, newContent); err != nil {
		u.lg.Warn("写回飞轮块索引重建失败（正文已落，重建可自愈）",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	appended := strings.Count(toAppend, "## ")
	u.lg.Info("写回飞轮已归档会话事实",
		loggateway.StepID("knowledge.writeback"),
		loggateway.Str("collection_id", col.ID),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int("appended", appended),
	)
	return WriteBackResult{
		CollectionID: col.ID,
		DocID:        doc.ID,
		Appended:     appended,
		Created:      created,
	}, nil
}

func extractFirstFactID(appendix string) string {
	const p = "fact_id: `"
	i := strings.Index(appendix, p)
	if i < 0 {
		return ""
	}
	rest := appendix[i+len(p):]
	j := strings.IndexByte(rest, '`')
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func filterAppendixNotPresent(body, appendix string) string {
	if appendix == "" {
		return ""
	}
	blocks := splitMarkdownH2(appendix)
	var b strings.Builder
	for _, block := range blocks {
		id := extractFirstFactID(block)
		stmt := firstNonEmptyLine(block)
		if writeBackAlreadyPresent(body, id, stmt) {
			continue
		}
		b.WriteString(block)
		if !strings.HasSuffix(block, "\n") {
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func splitMarkdownH2(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n## ")
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i > 0 {
			p = "## " + p
		} else if !strings.HasPrefix(p, "## ") {
			p = "## " + p
		}
		out = append(out, p+"\n")
	}
	return out
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "- ") {
			continue
		}
		return t
	}
	return ""
}

// LookupWriteBackHome 解析工作区写回落点：同名「团队知识收件箱」优先，否则第一个 team 库。
// 只读、不懒创建——GET 入口与专家/pending 用此方法，避免扫一眼就造库。
func (u *Usecase) LookupWriteBackHome(ctx context.Context, workspace string) (Collection, bool, error) {
	cols, _, err := u.collections.ListCollections(ctx, workspace, 1000, 0)
	if err != nil {
		return Collection{}, false, err
	}
	var firstTeam Collection
	for _, c := range cols {
		if c.VaultBackend != VaultBackendTeam {
			continue
		}
		if c.Name == WriteBackCollectionName {
			return c, true, nil
		}
		if firstTeam.ID == "" {
			firstTeam = c
		}
	}
	if firstTeam.ID != "" {
		return firstTeam, true, nil
	}
	return Collection{}, false, nil
}

func (u *Usecase) resolveWriteBackCollection(ctx context.Context, workspace string) (Collection, error) {
	col, found, err := u.LookupWriteBackHome(ctx, workspace)
	if err != nil {
		return Collection{}, err
	}
	if found {
		return col, nil
	}
	return u.CreateVault(ctx, Collection{
		Name:         WriteBackCollectionName,
		Description:  "会话自动沉淀（验证门 + provenance）",
		VaultBackend: VaultBackendTeam,
		Workspace:    workspace,
	})
}
