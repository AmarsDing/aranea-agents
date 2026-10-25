package knowledge

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── G-34 MOC 摘要对账补齐（ADR-KN-20261021-C）───────────────────────────────
//
// ADR-A 落骨架时「## 摘要」只落占位文；本任务在每个 curate 轮内对账：扫描
// moc/ 前缀下占位仍在的 MOC 页 → 页内「## 簇成员」wikilink 反查成员文档
//（页即真相源，不另存 membership 投影）→ MOCSummarizer（LLM）出摘要 →
// ensureMOCSummarySection 幂等替换 + 块索引重建 + chunk 重放。
// 预算 4/库/轮（成功后扣，失败不占）；LLM/落页失败 Warn 降级、占位保留下轮
// 重试；DryRun / 非团队库 / 端口未接线一律静默跳过；人工已写摘要不覆写。
//
// 终审加固（2026-10-21 对抗评审 wf_0132085c-3bf 后落码）：
//   F1/F4 落笔一律以「新鲜正文」为基——占位门与 LLM 调用各前各后两次重读，
//       窗口内人工编辑绝不覆写（人工竞态让位，预算失败口径不占）。
//   F2    总预算耗尽整段丢弃成员同样置 Truncated（单节选零截断也不例外）。
//   F5    结构性不可逆失败（缺簇成员段/wikilink 空/hub 已删）→ 占位行内改写
//       为 mocSummaryBlockedPrefix 标记退出待补集（防扫描槽饥饿+死等）。
//   F6    成员读取仅 NotFound 跳过；瞬时错误整页留下轮（零 LLM 零写零计数）。
//   F7    LLM 连续失败 3 连熔断本库本轮（防故障期 32 页 × 60s 串行停滞）。

const (
	// mocSummaryBudgetPerRound 每库每轮 LLM 补齐上限（成功后扣减；失败不计）。
	mocSummaryBudgetPerRound = 4
	// mocSummaryScanLimit 每轮扫描候选页数上限（超出部分下轮承接）。
	mocSummaryScanLimit = 32
	// mocSummaryHubMaxRunes hub 词条正文节选 rune 上限（首链恒为 hub）。
	mocSummaryHubMaxRunes = 800
	// mocSummaryMemberMaxRunes 单个成员词条节选 rune 上限。
	mocSummaryMemberMaxRunes = 400
	// mocSummaryTotalMaxRunes hub+全成员节选合计 rune 总预算。
	mocSummaryTotalMaxRunes = 4000
	// mocSummaryMaxConsecutiveLLMFails F7：单库单轮 LLM 连续失败熔断线。
	mocSummaryMaxConsecutiveLLMFails = 3
)

// mocSummaryBlockedPrefix F5：结构性卡死退出标记前缀——占位段行内改写为该
// 标记（字样与占位文不同，SQL 待补过滤不再命中），页内人工可见；簇结构修复
// 后由人工作主撰写摘要，绝不自动恢复。
const mocSummaryBlockedPrefix = "（摘要自动补齐暂停："

// mocSummaryBlockedNote 退出标记正文（H2 段内容）。
func mocSummaryBlockedNote(reason string) string {
	return mocSummaryBlockedPrefix + reason + "〔修复簇结构后请人工撰写摘要〕）"
}

// mocFillOutcome 单页补齐结局（对账循环据此预算/断流记账）。
type mocFillOutcome int8

const (
	mocFillSkip    mocFillOutcome = iota // 不适用/人工作主：零账务零预算
	mocFillRetry                         // 瞬时失败：占位保留等下轮（不计断流）
	mocFillLLMFail                       // LLM 失败：占位保留 + 计断流器
	mocFillOK                            // 成功
)

// MOCPendingSummaryDoc 「## 摘要」段仍为占位文的 MOC 页（对账扫描行）。
type MOCPendingSummaryDoc struct {
	DocID   string
	RelPath string
	Body    string // 文档正文（content_text 当前快照）
}

// MOCSummaryExcerpt 一个词条的摘要输入节选（已按预算截断）。
type MOCSummaryExcerpt struct {
	RelPath string
	Body    string
}

// MOCSummaryRequest MOC 摘要生成入参：hub 居首、成员保序（页内簇成员段次序）。
type MOCSummaryRequest struct {
	CollectionID string
	DocID        string
	RelPath      string
	Title        string
	Hub          MOCSummaryExcerpt
	Members      []MOCSummaryExcerpt
	Truncated    bool // 任一节选被预算截断
}

// MOCSummaryResult MOC 摘要生成产物。
type MOCSummaryResult struct {
	Summary string
}

// MOCSummarizer MOC 摘要生成端口（ADR-C 窄口；生产 = internal/knowledge.mocSummarizer）。
// wire 装配注入；nil 时 runMOCSummaryTask 整体跳过（治理其余任务不受影响）。
// Stability:evolving
type MOCSummarizer interface {
	SummarizeMOC(ctx context.Context, req MOCSummaryRequest) (MOCSummaryResult, error)
}

// SetMOCSummarizer 接线 MOC 摘要端口（可选）。
func (u *Usecase) SetMOCSummarizer(s MOCSummarizer) {
	u.mocs = s
}

// mocMemberLinks 反解「## 簇成员」段的 wikilink 行（保序，行即真相源）；
// 返回去 .md 的 link 文本，非法行丢弃。首链恒为 hub（mocMemberSection 生成契约）。
func mocMemberLinks(section string) []string {
	var links []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [[") || !strings.HasSuffix(line, "]]") {
			continue
		}
		link := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "- [["), "]]"))
		if link != "" {
			links = append(links, link)
		}
	}
	return links
}

// mocTitleFromBody 页标题推导：frontmatter title 优先 → H1 → rel basename。
func mocTitleFromBody(body, relPath string) string {
	trimmed := strings.TrimPrefix(body, "\ufeff")
	if strings.HasPrefix(trimmed, "---\n") {
		rest := trimmed[len("---\n"):]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			for _, line := range strings.Split(rest[:end], "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "title:") {
					if t := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "title:")), `"'`); t != "" {
						return t
					}
				}
			}
		}
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	base := relPath[strings.LastIndex(relPath, "/")+1:]
	return strings.TrimSuffix(base, ".md")
}

// ensureMOCSummarySection 「## 摘要」段幂等补齐（ADR-C 专属领地）：
// 占位文 → 替换为摘要（changed=true）；段缺 → 段尾重挂；已是该摘要 → no-op；
// 人工撰写的其他内容 → 不覆写（changed=false）。空摘要不出手。
func ensureMOCSummarySection(body, summary string) (string, bool) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return body, false
	}
	sec, ok := extractH2Section(body, "## 摘要")
	if !ok {
		return strings.TrimRight(body, "\n") + "\n\n## 摘要\n\n" + summary + "\n", true
	}
	secBody := strings.TrimSpace(strings.TrimPrefix(sec, "## 摘要"))
	if secBody == summary || secBody != mocSummaryPlaceholder {
		// 已是目标摘要 → no-op；人工撰写（非占位文）→ 不覆写
		return body, false
	}
	return replaceH2Section(body, "## 摘要", "## 摘要\n\n"+summary)
}

// mocExcerpt 按 rune 上限截取正文；返回 (节选, 是否截断)。上限 ≤0 视为不限。
func mocExcerpt(s string, max int) (string, bool) {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s, false
	}
	return string(r[:max]), true
}

// runMOCSummaryTask MOC 摘要对账补齐（curate 轮内，ADR-C）。返回本轮补齐页数。
func (u *Usecase) runMOCSummaryTask(ctx context.Context, homeCol Collection, colID string, opts CurateOptions, rep *CurateReport) int {
	if opts.DryRun || u.mocs == nil || u.documents == nil {
		return 0
	}
	// 团队库闸（distill 先例）：local vault 永不巡检补摘要；集合解析失败即跳。
	if homeCol.ID == "" && u.collections != nil {
		col, err := u.collections.GetCollection(ctx, colID)
		if err != nil {
			u.lg.Warn("治理：MOC 摘要集合解析失败（跳过）", loggateway.StepID("knowledge.curate.moc_summary"),
				loggateway.Str("collection_id", colID), loggateway.Err(err))
			return 0
		}
		homeCol = col
	}
	if homeCol.VaultBackend != VaultBackendTeam {
		return 0
	}
	pendings, err := u.curate.ListPendingMOCSummaries(ctx, homeCol.ID, mocSummaryScanLimit)
	if err != nil {
		u.lg.Warn("治理：MOC 摘要待补页扫描失败", loggateway.StepID("knowledge.curate.moc_summary"), loggateway.Err(err))
		return 0
	}
	filled := 0
	consecFails := 0
	tripped := false
	for _, p := range pendings {
		if filled >= mocSummaryBudgetPerRound || tripped {
			break
		}
		switch u.fillMOCSummary(ctx, homeCol, p) {
		case mocFillOK:
			filled++
			consecFails = 0
		case mocFillLLMFail:
			consecFails++
			// F7：LLM 连续失败即熔断本库本轮——故障期不再逐页 60s 串停。
			if consecFails >= mocSummaryMaxConsecutiveLLMFails {
				tripped = true
				u.lg.Warn("治理：MOC 摘要 LLM 连续失败（本库本轮熔断）",
					loggateway.StepID("knowledge.curate.moc_summary"),
					loggateway.Int("consecutive_fails", consecFails),
					loggateway.Str("collection_id", homeCol.ID))
			}
		}
	}
	if filled > 0 {
		rep.Actions = append(rep.Actions, "moc_summary")
	}
	return filled
}

// fillMOCSummary 单页摘要补齐：新鲜正文占位门 → 簇成员反查装帧（预算扣减）→
// LLM → 落笔前再读新鲜正文重跑占位门 → 幂等落页 + 重建 + 重放。
// 结局语义对账：OK 计预算；LLMFail 计断流器；Retry 仅占位保留；Skip 零账务。
func (u *Usecase) fillMOCSummary(ctx context.Context, col Collection, pending MOCPendingSummaryDoc) mocFillOutcome {
	step := loggateway.StepID("knowledge.curate.moc_summary")
	// F1/F4：原文以扫描快照为源，扫描→落笔最长隔 60s（LLM 超时）——窗口内
	// 人工编辑（含人工摘要）会被旧快照+LLM 文案静默覆写。落笔一律以新鲜正文
	// 为基：此处一出装帧源，再出 LLM 后重读；页被删 → 让位。
	fresh, err := u.documents.GetDocumentByRelPath(ctx, col.ID, pending.RelPath)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return mocFillSkip
		}
		u.lg.Warn("治理：MOC 摘要页重读失败（下轮重试）", step,
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(err))
		return mocFillRetry
	}
	body := fresh.ContentText
	// 占位门：人工已定稿/摘要段被摘走 → 不调 LLM 不占预算。
	sec, ok := extractH2Section(body, "## 摘要")
	if !ok || !strings.Contains(sec, mocSummaryPlaceholder) {
		return mocFillSkip
	}
	memberSec, ok := extractH2Section(body, "## 簇成员")
	if !ok {
		return u.blockMOCSummary(ctx, col, pending, body, "缺簇成员段")
	}
	links := mocMemberLinks(memberSec)
	if len(links) == 0 {
		return u.blockMOCSummary(ctx, col, pending, body, "簇成员清单为空")
	}
	req := MOCSummaryRequest{
		CollectionID: col.ID,
		DocID:        pending.DocID,
		RelPath:      pending.RelPath,
		Title:        mocTitleFromBody(body, pending.RelPath),
	}
	// 首链恒为 hub（800 预算）；hub 瞬读错本轮跳过下轮重试、hub 已删行内退出；
	// 成员 NotFound 跳过不阻断，瞬读错整页留下轮（F6）。
	remaining := mocSummaryTotalMaxRunes
	hub, truncated, herr := u.mocMemberExcerpt(ctx, col.ID, links[0], mocSummaryHubMaxRunes, &remaining)
	if herr != nil {
		if apierror.IsCode(herr, apierror.CodeNotFound) {
			return u.blockMOCSummary(ctx, col, pending, body, "hub 词条缺失")
		}
		u.lg.Warn("治理：MOC 摘要补齐跳过（hub 读失败，下轮重试）", step,
			loggateway.Str("hub_rel_path", links[0]+".md"),
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(herr))
		return mocFillRetry
	}
	req.Hub = hub
	req.Truncated = truncated
	for _, link := range links[1:] {
		if remaining <= 0 {
			req.Truncated = true // F2：预算耗尽整段丢弃成员同样置截断标
			break
		}
		m, cut, cerr := u.mocMemberExcerpt(ctx, col.ID, link, mocSummaryMemberMaxRunes, &remaining)
		if cerr != nil {
			if apierror.IsCode(cerr, apierror.CodeNotFound) {
				u.lg.Warn("治理：MOC 摘要成员缺失（跳过该成员）", step,
					loggateway.Str("member_rel_path", link+".md"),
					loggateway.Str("rel_path", pending.RelPath))
				continue
			}
			u.lg.Warn("治理：MOC 摘要成员读失败（整页下轮重试）", step,
				loggateway.Str("member_rel_path", link+".md"),
				loggateway.Str("rel_path", pending.RelPath), loggateway.Err(cerr))
			return mocFillRetry
		}
		req.Members = append(req.Members, m)
		req.Truncated = req.Truncated || cut
	}
	res, err := u.mocs.SummarizeMOC(ctx, req)
	if err != nil {
		u.lg.Warn("治理：MOC 摘要生成失败（占位保留，下轮重试）", step,
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(err))
		return mocFillLLMFail
	}
	// F1/F4 闭幕：LLM 窗口后落笔前再读新鲜正文重跑占位门——窗口内人工已
	// 定稿 → 让位人工（LLM 开销已付按失败口径不占预算；人工作主不告警）。
	fresh2, err := u.documents.GetDocumentByRelPath(ctx, col.ID, pending.RelPath)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return mocFillSkip
		}
		u.lg.Warn("治理：MOC 摘要落笔前重读失败（下轮重试）", step,
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(err))
		return mocFillRetry
	}
	sec2, ok := extractH2Section(fresh2.ContentText, "## 摘要")
	if !ok || !strings.Contains(sec2, mocSummaryPlaceholder) {
		return mocFillSkip
	}
	next, changed := ensureMOCSummarySection(fresh2.ContentText, res.Summary)
	if !changed {
		return mocFillSkip // 空白产出：不覆写不计数
	}
	if err := u.documents.UpdateDocumentContent(ctx, pending.DocID, next, true); err != nil {
		u.lg.Warn("治理：MOC 摘要落页失败（下轮重试）", step,
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(err))
		return mocFillRetry
	}
	if rerr := u.RebuildBlockIndex(ctx, col.ID, pending.DocID, next); rerr != nil {
		u.lg.Warn("MOC 页块索引重建失败（摘要已落，索引自愈可补）", step,
			loggateway.Str("doc_id", pending.DocID), loggateway.Err(rerr))
	}
	u.replayMOCChunks(ctx, col, []PromoteTouchedDoc{{DocID: pending.DocID}})
	u.lg.Info("治理：MOC 摘要已补齐", step,
		loggateway.Str("rel_path", pending.RelPath),
		loggateway.Str("doc_id", pending.DocID),
		loggateway.Bool("truncated", req.Truncated),
	)
	return mocFillOK
}

// blockMOCSummary F5 结构性不可逆失败处理：占位段行内改写为
// mocSummaryBlockedPrefix 标记——SQL 待补过滤不再命中（退出扫描槽位，防
// 卡死页占位 LIMIT 32 窗口饥饿后续健康页），页内人工可见；绝不自动恢复
// （人工修复簇结构后由人工撰写摘要，equivalent 于人工定稿语义）。
func (u *Usecase) blockMOCSummary(ctx context.Context, col Collection, pending MOCPendingSummaryDoc, body, reason string) mocFillOutcome {
	step := loggateway.StepID("knowledge.curate.moc_summary")
	next, changed := ensureMOCSummarySection(body, mocSummaryBlockedNote(reason))
	if !changed {
		return mocFillSkip // 人工已先于本巡检定稿：让位
	}
	if err := u.documents.UpdateDocumentContent(ctx, pending.DocID, next, true); err != nil {
		u.lg.Warn("治理：MOC 摘要暂停标记落页失败（下轮重试）", step,
			loggateway.Str("rel_path", pending.RelPath), loggateway.Err(err))
		return mocFillRetry
	}
	if rerr := u.RebuildBlockIndex(ctx, col.ID, pending.DocID, next); rerr != nil {
		u.lg.Warn("MOC 页块索引重建失败（暂停标记已落，索引自愈可补）", step,
			loggateway.Str("doc_id", pending.DocID), loggateway.Err(rerr))
	}
	u.replayMOCChunks(ctx, col, []PromoteTouchedDoc{{DocID: pending.DocID}})
	u.lg.Warn("治理：MOC 摘要结构性失败（已行内退出待补集）", step,
		loggateway.Str("rel_path", pending.RelPath),
		loggateway.Str("reason", reason))
	return mocFillSkip
}

// mocMemberExcerpt 反查成员文档并按「单成员上限 ∩ 剩余总预算」截断；
// 返回 API 错误原链（调用方以 apierror.IsCode 判 NotFound 跳过 / 瞬错整页重试——F6）。
func (u *Usecase) mocMemberExcerpt(ctx context.Context, collectionID, link string, maxRunes int, remaining *int) (MOCSummaryExcerpt, bool, error) {
	rel := link + ".md"
	doc, err := u.documents.GetDocumentByRelPath(ctx, collectionID, rel)
	if err != nil {
		return MOCSummaryExcerpt{}, false, err
	}
	if maxRunes > *remaining {
		maxRunes = *remaining
	}
	body, cut := mocExcerpt(doc.ContentText, maxRunes)
	*remaining -= len([]rune(body))
	return MOCSummaryExcerpt{RelPath: rel, Body: body}, cut, nil
}
