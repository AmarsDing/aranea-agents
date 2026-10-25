package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── G-34 MOC 自动摘要词条（ADR-KN-20261021-A/B/C）─────────────────────────
//
// A：moc_emerge 提案 applied 时同步落 moc/<slug>.md 骨架页（零 LLM）。
//    载体 = 普通词条页（DocType=moc 显式落列、pending 入索引管线、Organized）。
//    MOC↔members 边由 `## 簇成员` wikilink 经块索引物化零成本获得；rel_path
//    冲突（人工页/他簇 MOC 同标题）→ slug+确定性短后缀另起一页。文档层
//    collection_id+rel_path get-or-create 为最终幂等担保。
// B：dedup_key = moc:<hub_doc_id>:<成员签名>——同簇沉默、簇演进可重提，
//    解除原三态（pending/applied/rejected）永久禁言死信。
// C：摘要段本轮落占位文；LLM 对账补齐属 ADR-C（巡检轮），占位锚常量共用。

// docTypeMOC MOC 页 DocType 列值（report/manual/note/faq 同槽）。
const docTypeMOC = "moc"

// mocPageDir MOC 页目录前缀（与 entries 同级）。
const mocPageDir = "moc"

// mocSummaryPlaceholder 摘要占位段锚文。ADR-C runMOCSummaryTask 以此侦测待补页。
const mocSummaryPlaceholder = "（摘要待治理巡检自动补齐）"

// mocMemberSigLen 成员签名十六进制截断长度（dedup_key 第三段宽度）。
const mocMemberSigLen = 10

// mocSkeletonParams 骨架渲染入参（纯值，脱 payload 的 any 罐）。
type mocSkeletonParams struct {
	Title      string   // 建议标题（suggested_title → 页名 basename）
	HubDocID   string   // hub 词条文档 ID（簇身份标记）
	HubRelPath string   // hub 词条 rel_path（成员清单第一项）
	Degree     int      // hub 度数
	Density    float64  // 簇边密度
	Members    []string // 成员 rel_path（不含 hub）
}

// mocMemberSignature 成员集合签名（ADR-B）：排序+去重后 sha1 截断 10 hex。
// 序无关且不污染入参切片；成员集合一变签名即变 → 簇演进可重提。
func mocMemberSignature(members []string) string {
	sorted := make([]string, 0, len(members))
	seen := map[string]struct{}{}
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		sorted = append(sorted, m)
	}
	sort.Strings(sorted)
	sum := sha1.Sum([]byte(strings.Join(sorted, "\x1f")))
	return hex.EncodeToString(sum[:])[:mocMemberSigLen]
}

// mocPageRelPath 标题 → 规范页路径；slug 坍缩为空时仰赖调用方先校验 title。
func mocPageRelPath(title string) string {
	slug := entrySlug(title)
	if slug == "" {
		return ""
	}
	return mocPageDir + "/" + slug + ".md"
}

// mocConflictRelPath rel_path 冲突确定性后缀页（ADR-A）。同簇二次 applied 命中同
// 一张后缀页走 Update；seed 取 hub_doc_id（缺省回退 hub_rel_path）。
func mocConflictRelPath(title, seed string) string {
	slug := entrySlug(title)
	if slug == "" {
		return ""
	}
	sum := sha1.Sum([]byte(strings.TrimSpace(seed)))
	return mocPageDir + "/" + slug + "-" + hex.EncodeToString(sum[:])[:6] + ".md"
}

// mocHubMarker 簇身份行前缀（页内 HTML 注释，人工不可见改动成本低）。
const mocHubMarkerPrefix = "<!-- moc:hub "

// extractMOCHubMarker 抽取页内簇身份标记；无标记返回空串。
func extractMOCHubMarker(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, mocHubMarkerPrefix) || !strings.HasSuffix(line, "-->") {
			continue
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, mocHubMarkerPrefix), "-->"))
	}
	return ""
}

// entriesMemberLink 成员清单行（wikilink 全路径形，与 entityWikiWikilink 同式）。
func entriesMemberLink(relPath string) string {
	return "- " + entityWikiWikilink(relPath)
}

// mocStatsSection 受管段「簇统计」（整体可替换的基本单元）。
func mocStatsSection(p mocSkeletonParams) string {
	return "## 簇统计\n\n" +
		"- 簇规模：" + strconv.Itoa(len(dedupMembers(p.Members))+1) + " 篇\n" +
		"- Hub 度数：" + strconv.Itoa(p.Degree) + "\n" +
		"- 边密度：" + strconv.FormatFloat(p.Density, 'f', 2, 64) + "\n"
}

// mocMemberSection 受管段「簇成员」：hub 恒居首，其余成员确定性排序（ADR-C 页即
// 真相源——对账补齐摘要以本段 wikilink 反查成员文档）。
func mocMemberSection(p mocSkeletonParams) string {
	var b strings.Builder
	b.WriteString("## 簇成员\n\n")
	b.WriteString(entriesMemberLink(p.HubRelPath))
	b.WriteString("\n")
	for _, m := range dedupMembers(p.Members) {
		if m == p.HubRelPath {
			continue
		}
		b.WriteString(entriesMemberLink(m))
		b.WriteString("\n")
	}
	return b.String()
}

// mocSummaryPlaceholderSection 占位摘要段（ADR-C ensureMOCSummarySection 替换目标）。
func mocSummaryPlaceholderSection() string {
	return "## 摘要\n\n" + mocSummaryPlaceholder + "\n"
}

// dedupMembers 成员去重+排序副本（null，不触碰入参）。
func dedupMembers(members []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(members))
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// h2SectionBounds 以精确小节标题（如 "## 簇统计"）定位整段字节界：
// [start,end) 含标题行至下一 "## " 行（不含）或 EOF。replaceH2BlockContaining
// 的 marker 契约是「块内行内子串」，以标题作 marker 会越界吞前节——受管段
// 必须走本函数。
func h2SectionBounds(body, heading string) (start, end int, ok bool) {
	start, end = -1, len(body)
	off := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if start < 0 {
			if trimmed == heading {
				start = off
			}
		} else if strings.HasPrefix(trimmed, "## ") {
			end = off
			break
		}
		off += len(line)
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, end, true
}

// replaceH2Section 替换以 heading 命名的 H2 小节整段为 newBlock；小节不存在
// 返回 false。输出与 buildMOCSkeletonBody 同规范化形态（段间恰一空行、EOF 单
// 换行），逐字节幂等是 applied 重放零改动的前提。
func replaceH2Section(body, heading, newBlock string) (string, bool) {
	start, end, ok := h2SectionBounds(body, heading)
	if !ok {
		return body, false
	}
	prefix := strings.TrimRight(body[:start], "\n")
	suffix := strings.TrimLeft(body[end:], "\n")
	var b strings.Builder
	if prefix != "" {
		b.WriteString(prefix)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(newBlock))
	if suffix != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimRight(suffix, "\n"))
	}
	b.WriteString("\n")
	return b.String(), true
}

// extractH2Section 取以 heading 命名的 H2 小节原文（ADR-C 占位侦测用）。
func extractH2Section(body, heading string) (string, bool) {
	start, end, ok := h2SectionBounds(body, heading)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(body[start:end]), true
}

// buildMOCSkeletonBody 生成 MOC 骨架页正文（ADR-A）。纯函数、输出确定性——
// 同参重放逐字节相等是 applied 幂等的前提。
func buildMOCSkeletonBody(p mocSkeletonParams) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlQuote(p.Title) + "\n")
	b.WriteString("type: " + docTypeMOC + "\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + p.Title + "\n\n")
	if hub := strings.TrimSpace(p.HubDocID); hub != "" {
		b.WriteString(mocHubMarkerPrefix + hub + " -->\n\n")
	}
	b.WriteString("> 本页由知识治理自动维护：「簇统计」「簇成员」随提案应用更新，「摘要」由治理巡检自动补齐。人工可直接编辑其余部分。\n\n")
	b.WriteString(mocSummaryPlaceholderSection())
	b.WriteString("\n")
	b.WriteString(mocStatsSection(p))
	b.WriteString("\n")
	b.WriteString(mocMemberSection(p))
	return b.String()
}

// ensureMOCSkeletonBody 幂等保证 MOC 页体：空 → 全骨架；已有 → 受管段（簇统计/
// 簇成员）整体替换，缺失则段尾重挂；「摘要」与人工追加段不触碰（ADR-C 领地）。
// 返回 (next, changed)；changed=false 时 next==body。
func ensureMOCSkeletonBody(body string, p mocSkeletonParams) (string, bool) {
	if strings.TrimSpace(body) == "" {
		return buildMOCSkeletonBody(p), true
	}
	next := body
	changed := false
	for _, s := range []struct{ marker, block string }{
		{"## 簇统计", mocStatsSection(p)},
		{"## 簇成员", mocMemberSection(p)},
	} {
		if replaced, ok := replaceH2Section(next, s.marker, s.block); ok {
			next = replaced
			continue
		}
		next = strings.TrimRight(next, "\n") + "\n\n" + strings.TrimRight(s.block, "\n") + "\n"
	}
	if next != body {
		changed = true
	}
	return next, changed
}

// mocParamsFromPayload 解 applied 提案载荷（JSON 往返后 members 为 []any、数值为
// float64）。缺 title/hub_rel_path 的历史提案返回 ok=false → disposal 降级 no-op
// （与 orphan/conflict 先例同语义）。
func mocParamsFromPayload(payload map[string]any) (mocSkeletonParams, bool) {
	if payload == nil {
		return mocSkeletonParams{}, false
	}
	title, _ := payload["suggested_title"].(string)
	hubRel, _ := payload["hub_rel_path"].(string)
	title, hubRel = strings.TrimSpace(title), strings.TrimSpace(hubRel)
	if title == "" || hubRel == "" {
		return mocSkeletonParams{}, false
	}
	p := mocSkeletonParams{Title: title, HubRelPath: hubRel}
	p.HubDocID, _ = payload["hub_doc_id"].(string)
	switch v := payload["members"].(type) {
	case []string:
		p.Members = v
	case []any:
		for _, m := range v {
			if s, ok := m.(string); ok {
				p.Members = append(p.Members, s)
			}
		}
	}
	if d, ok := payload["degree"].(float64); ok {
		p.Degree = int(d)
	}
	if d, ok := payload["density"].(float64); ok {
		p.Density = d
	}
	return p, true
}

// applyMOCGovernanceDisposal 执行 moc_emerge 提案 applied 的同步建/更（ADR-A）。
// 幂等担保链：dedup 签名粗筛 → 文档层 rel_path get-or-create 终裁 → 页内受管段
// 幂等替换。团队库以外的集合显式拒绝（禁止伪 applied）；提案停留 pending 可重审。
func (u *Usecase) applyMOCGovernanceDisposal(ctx context.Context, view GovernanceProposalView) error {
	// 载荷先裁：历史提案缺关键字段降级 no-op（与既有 kind 先例一致）——
	// 该判定先行的另一收益：repo 未接线时 legacy 提案仍可静默落 applied。
	p, ok := mocParamsFromPayload(view.Payload)
	if !ok {
		return nil
	}
	if u.documents == nil || u.collections == nil {
		return apierror.Unavailable(apierror.DomainKnowledge, "knowledge document repo not wired")
	}
	col, err := u.collections.GetCollection(ctx, view.CollectionID)
	if err != nil {
		return err
	}
	if col.VaultBackend != VaultBackendTeam {
		return apierror.BadRequest(apierror.DomainKnowledge, "moc disposal only applies to team vaults")
	}
	doc, rel, err := u.resolveMOCPaging(ctx, col.ID, p)
	if err != nil {
		return err
	}
	body := ""
	if rel == "" {
		return nil // slug 坍缩（标题全符号）：无法成页，no-op
	}
	created := doc.ID == ""
	if !created {
		body = doc.ContentText
	}
	next, changed := ensureMOCSkeletonBody(body, p)
	if !changed {
		return nil // 已是最新骨架：幂等成功（零写零重放）
	}
	if created {
		doc, err = u.CreateDocument(ctx, Document{
			CollectionID: col.ID,
			RelPath:      rel,
			Source:       rel,
			MimeType:     "text/markdown",
			DocType:      docTypeMOC,
			ContentText:  next,
			Organized:    true,
			Status:       "pending",
		})
		if err != nil {
			return err
		}
	} else if err = u.documents.UpdateDocumentContent(ctx, doc.ID, next, true); err != nil {
		return err
	} else {
		doc.ContentText = next
	}
	if rerr := u.RebuildBlockIndex(ctx, col.ID, doc.ID, next); rerr != nil {
		u.lg.Warn("MOC 页块索引重建失败（正文已落，索引自愈可补）",
			loggateway.StepID("knowledge.curate.moc"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(rerr),
		)
	}
	u.replayMOCChunks(ctx, col, []PromoteTouchedDoc{{DocID: doc.ID, Created: created}})
	u.lg.Info("治理：moc_emerge 提案 applied，MOC 页已落",
		loggateway.StepID("knowledge.curate.moc"),
		loggateway.Str("rel_path", rel),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int64("proposal_id", view.ID),
	)
	return nil
}

// resolveMOCPaging get-or-create 选址：首选 moc/<slug>.md；被人工页或他簇 MOC
// 占用时确定性后缀化另起。返回现存同簇页（doc.ID 非空走 Update）或空 doc 走建页。
func (u *Usecase) resolveMOCPaging(ctx context.Context, collectionID string, p mocSkeletonParams) (Document, string, error) {
	primary := mocPageRelPath(p.Title)
	if primary == "" {
		return Document{}, "", nil
	}
	doc, ours, err := u.lookupMOCPage(ctx, collectionID, primary, p.HubDocID)
	if err != nil {
		return Document{}, "", err
	}
	if doc == nil || ours {
		if doc == nil {
			return Document{}, primary, nil
		}
		return *doc, primary, nil
	}
	// 冲突：另起确定性后缀页。二次碰撞（后缀页也存且仍非本簇）显式报错可审。
	seed := p.HubDocID
	if seed == "" {
		seed = p.HubRelPath
	}
	alt := mocConflictRelPath(p.Title, seed)
	doc, ours, err = u.lookupMOCPage(ctx, collectionID, alt, p.HubDocID)
	if err != nil {
		return Document{}, "", err
	}
	if doc == nil {
		return Document{}, alt, nil
	}
	if ours {
		return *doc, alt, nil
	}
	return Document{}, "", apierror.BadRequest(apierror.DomainKnowledge,
		fmt.Sprintf("moc page %s collided twice; rename manually", alt))
}

// lookupMOCPage 按 rel_path 取页并判归属：同簇 = DocType=moc 且 hub 标记相等
// （hub_doc_id 缺失的提案无法自证归属，一律视为他页走后缀）。
func (u *Usecase) lookupMOCPage(ctx context.Context, collectionID, rel, hubDocID string) (doc *Document, ours bool, err error) {
	found, err := u.documents.GetDocumentByRelPath(ctx, collectionID, rel)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if strings.TrimSpace(found.DocType) != docTypeMOC {
		return &found, false, nil
	}
	if hubDocID == "" {
		return &found, false, nil
	}
	if extractMOCHubMarker(found.ContentText) != hubDocID {
		return &found, false, nil
	}
	return &found, true, nil
}

// replayMOCChunks MOC 页 chunk 重放（实体词条先例：Warn 降级不阻断 disposal）。
func (u *Usecase) replayMOCChunks(ctx context.Context, col Collection, touched []PromoteTouchedDoc) {
	if u == nil || u.writeBackReplay == nil || len(touched) == 0 {
		return
	}
	deferred := false
	for _, doc := range touched {
		deferred = deferKnowledgeReplay(ctx, doc) || deferred
	}
	if deferred {
		return
	}
	if err := u.writeBackReplay(ctx, col, touched); err != nil {
		u.lg.Warn("MOC 页 chunk 重放失败（正文已落，索引自愈可补）",
			loggateway.StepID("knowledge.curate.moc"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Err(err),
		)
	}
}
